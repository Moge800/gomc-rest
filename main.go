package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"

	charmlog "github.com/charmbracelet/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed openapi.yaml
var openAPISpec []byte

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Getenv, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}

	// setup logger
	charmLogger := charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		Level:           charmlog.Level(cfg.LogLevel),
		ReportTimestamp: true,
	})
	logOut := io.Writer(os.Stderr)
	var handler slog.Handler = charmLogger
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("open log file %q: %v", cfg.LogFile, err)
		}
		defer f.Close()
		logOut = io.MultiWriter(os.Stderr, f)
		fileHandler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: cfg.LogFileLevel})
		handler = &teeHandler{handlers: []slog.Handler{charmLogger, fileHandler}}
	}
	log.SetOutput(logOut)
	slog.SetDefault(slog.New(handler))

	plc := newConfiguredPLCClient(cfg)
	plcQueue := newPLCQueue(plc, cfg.QueueSize)
	plcQueue.InitialConnect()

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", handleOpenAPI(openAPISpec))
	mux.HandleFunc("/version", handleVersion())
	mux.HandleFunc("/info", handleInfo(cfg))
	mux.HandleFunc("/metrics", handleMetrics(plcQueue))
	mux.HandleFunc("/health", handleHealth(plcQueue))
	mux.HandleFunc("/read", handleRead(plcQueue))
	mux.HandleFunc("/write", handleWrite(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/random-read", handleRandomRead(plcQueue))
	mux.HandleFunc("/random-write", handleRandomWrite(plcQueue, cfg.ReadOnly))
	mux.HandleFunc("/remote/run", handleRemoteRun(plcQueue, cfg.ReadOnly, cfg.EnableRemote))
	mux.HandleFunc("/remote/stop", handleRemoteStop(plcQueue, cfg.ReadOnly, cfg.EnableRemote))
	mux.HandleFunc("/remote/pause", handleRemotePause(plcQueue, cfg.ReadOnly, cfg.EnableRemote))
	mux.HandleFunc("/remote/latch-clear", handleRemoteLatchClear(plcQueue, cfg.ReadOnly, cfg.EnableRemote))
	mux.HandleFunc("/remote/reset", handleRemoteReset(plcQueue, cfg.ReadOnly, cfg.EnableRemote))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           logRequests(recoverPanic(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("listening",
			"version", version,
			"addr", cfg.Listen,
			"plc", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			"frame", cfg.Frame,
			"transport", cfg.Transport,
			"mode", cfg.ModeString,
			"readonly", cfg.ReadOnly,
			"enable_remote", cfg.EnableRemote,
			"log_level", cfg.LogLevel,
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
