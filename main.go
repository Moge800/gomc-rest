package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("invalid %s %q: must be a boolean (true/false or 1/0)", key, v)
	}
	return b
}

func main() {
	host := flag.String("host", getenv("PLC_HOST", "192.168.0.1"), "PLC host")
	portStr := flag.String("port", getenv("PLC_PORT", "5007"), "PLC port")
	modeStr := flag.String("mode", getenv("PLC_MODE", "binary"), "PLC mode (binary|ascii)")
	listen := flag.String("listen", getenv("LISTEN_ADDR", ":8080"), "HTTP listen address")
	readonly := flag.Bool("readonly", getenvBool("READONLY", false), "disable write and remote-control endpoints")
	flag.Parse()

	port, err := strconv.Atoi(*portStr)
	if err != nil || port < 1 || port > 65535 {
		log.Fatalf("invalid port: %s", *portStr)
	}

	var mode mc.Mode
	switch *modeStr {
	case "binary":
		mode = mc.ModeBinary
	case "ascii":
		mode = mc.ModeASCII
	default:
		log.Fatalf("invalid mode %q: must be binary or ascii", *modeStr)
	}

	plc := newPLCClient(*host, port, mode)
	plc.initialConnect()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth(plc))
	mux.HandleFunc("/read", handleRead(plc))
	mux.HandleFunc("/write", handleWrite(plc, *readonly))
	mux.HandleFunc("/remote/run", handleRemoteRun(plc, *readonly))
	mux.HandleFunc("/remote/stop", handleRemoteStop(plc, *readonly))
	mux.HandleFunc("/remote/pause", handleRemotePause(plc, *readonly))
	mux.HandleFunc("/remote/latch-clear", handleRemoteLatchClear(plc, *readonly))
	mux.HandleFunc("/remote/reset", handleRemoteReset(plc, *readonly))

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Printf("listening on %s  plc=%s:%d mode=%s readonly=%t\n", *listen, *host, port, *modeStr, *readonly)
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

	plc.mu.Lock()
	if plc.conn != nil {
		plc.conn.Close()
	}
	plc.mu.Unlock()
}
