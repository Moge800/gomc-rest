package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Getenv, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}

	// setup logger
	logOut := io.Writer(os.Stderr)
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("open log file: %v", err)
		}
		defer f.Close()
		logOut = io.MultiWriter(os.Stderr, f)
	}
	log.SetOutput(logOut)
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, nil)))

	plc := newConfiguredPLCClient(cfg)
	plcQueue := newPLCQueue(plc, cfg.QueueSize)
	plcQueue.InitialConnect()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth(plcQueue))
	mux.HandleFunc("/read", handleRead(plcQueue))
	mux.HandleFunc("/write", handleWrite(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/run", handleRemoteRun(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/stop", handleRemoteStop(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/pause", handleRemotePause(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/latch-clear", handleRemoteLatchClear(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/reset", handleRemoteReset(plcQueue, cfg.ReadOnly))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("listening",
			"addr", cfg.Listen,
			"plc", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			"frame", cfg.Frame,
			"transport", cfg.Transport,
			"mode", cfg.ModeString,
			"readonly", cfg.ReadOnly,
			"queue_size", cfg.QueueSize,
			"timeout", cfg.Timeout,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	if err := plcQueue.Shutdown(ctx); err != nil {
		slog.Error("queue shutdown error", "error", err)
	}
	plcQueue.Close()
}
