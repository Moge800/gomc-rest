package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

// logPLCOp logs a single PLC operation result via slog.
func logPLCOp(addr string, d time.Duration, err error) {
	result := "ok"
	if err != nil {
		var busy *busyErr
		var connErr *connErrWrap
		var plcErr *mc.MCProtocolError
		switch {
		case errors.As(err, &busy):
			result = "busy"
		case errors.Is(err, errQueueClosed):
			result = "queue_closed"
		case errors.Is(err, context.Canceled):
			result = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			result = "timeout"
		case errors.As(err, &connErr):
			result = "connection_error"
		case errors.As(err, &plcErr):
			result = "plc_error"
		default:
			result = "error"
		}
	}
	slog.Debug("plc_op",
		"addr", addr,
		"latency_ms", d.Milliseconds(),
		"result", result,
	)
}

// statusRecorder captures the HTTP status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// recoverPanic catches handler panics, logs them, and returns 500.
// Place inside logRequests so the request is logged with status 500.
func recoverPanic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// logRequests wraps h and logs each request via slog.
// When logSuccess is false, 2xx responses are not logged.
func logRequests(h http.Handler, logSuccess bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		if !logSuccess && rec.status < 400 {
			return
		}
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	})
}
