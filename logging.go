package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

// teeHandler fans out log records to multiple handlers.
type teeHandler struct {
	handlers []slog.Handler
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range t.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &teeHandler{handlers: hs}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &teeHandler{handlers: hs}
}

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
// 2xx → Info, 4xx → Warn, 5xx → Error.
func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		} else if rec.status >= 400 {
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "request",
			"remote", r.RemoteAddr,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	})
}
