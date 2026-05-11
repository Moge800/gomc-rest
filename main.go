package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
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
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Printf("listening on %s  plc=%s:%d frame=%s transport=%s mode=%s readonly=%t queue_size=%d timeout=%s\n", cfg.Listen, cfg.Host, cfg.Port, cfg.Frame, cfg.Transport, cfg.ModeString, cfg.ReadOnly, cfg.QueueSize, cfg.Timeout)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-quit
	fmt.Println("\nshutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	if err := plcQueue.Shutdown(ctx); err != nil {
		log.Printf("queue shutdown error: %v", err)
	}
	plcQueue.Close()
}
