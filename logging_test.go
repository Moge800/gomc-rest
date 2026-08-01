package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecoverPanicReturns500(t *testing.T) {
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })

	handler := recoverPanic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestLogRequestsStatusCode(t *testing.T) {
	// Redirect slog to discard so test output stays clean.
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })

	cases := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{
			name:       "explicit 200",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "implicit 200",
			handler:    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "explicit 404",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "explicit 503",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			logRequests(tc.handler).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestLogRequestsLevel(t *testing.T) {
	cases := []struct {
		status    int
		wantLevel slog.Level
	}{
		{http.StatusOK, slog.LevelInfo},
		{http.StatusNotFound, slog.LevelWarn},
		{http.StatusServiceUnavailable, slog.LevelError},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var got slog.Level
			h := &levelCaptureHandler{level: &got}
			orig := slog.Default()
			slog.SetDefault(slog.New(h))
			t.Cleanup(func() { slog.SetDefault(orig) })

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			logRequests(handler).ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.wantLevel {
				t.Errorf("status %d → level %v, want %v", tc.status, got, tc.wantLevel)
			}
		})
	}
}

func TestLogRequestsPLCLatency(t *testing.T) {
	var capturedAttrs []slog.Attr
	h := &attrCaptureHandler{attrs: &capturedAttrs}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(orig) })

	// handler that writes PLC latency into context (simulates a PLC endpoint)
	withPLC := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writePLCLatency(r.Context(), 15*time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	// handler that does NOT write PLC latency (simulates /health, /metrics)
	withoutPLC := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("plc_latency_ms present when PLC op performed", func(t *testing.T) {
		capturedAttrs = nil
		req := httptest.NewRequest(http.MethodGet, "/read", nil)
		logRequests(withPLC).ServeHTTP(httptest.NewRecorder(), req)

		for _, a := range capturedAttrs {
			if a.Key == "plc_latency_ms" {
				if a.Value.Any().(float64) <= 0 {
					t.Errorf("plc_latency_ms = %v, want > 0", a.Value)
				}
				return
			}
		}
		t.Error("plc_latency_ms not found in log attrs")
	})

	t.Run("plc_latency_ms absent when no PLC op", func(t *testing.T) {
		capturedAttrs = nil
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		logRequests(withoutPLC).ServeHTTP(httptest.NewRecorder(), req)

		for _, a := range capturedAttrs {
			if a.Key == "plc_latency_ms" {
				t.Errorf("plc_latency_ms should be absent for non-PLC endpoints, got %v", a.Value)
			}
		}
	})
}

func TestLogRequestsDurationMs(t *testing.T) {
	var capturedAttrs []slog.Attr
	h := &attrCaptureHandler{attrs: &capturedAttrs}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(orig) })

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	logRequests(handler).ServeHTTP(httptest.NewRecorder(), req)

	var foundDurationMs bool
	for _, a := range capturedAttrs {
		if a.Key == "duration_ms" {
			foundDurationMs = true
			if _, ok := a.Value.Any().(float64); !ok {
				t.Errorf("duration_ms value type = %T, want float64", a.Value.Any())
			}
		}
		if a.Key == "duration" {
			t.Error("old key 'duration' found; expected 'duration_ms'")
		}
	}
	if !foundDurationMs {
		t.Error("duration_ms not found in log attrs")
	}
}

// TestRunIDOnStateEventsOnly pins run_id to state events. Request logs are the
// bulk of the file and are contiguous in time, so they must stay narrow.
func TestRunIDOnStateEventsOnly(t *testing.T) {
	var capturedAttrs []slog.Attr
	h := &attrCaptureHandler{attrs: &capturedAttrs}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(orig) })

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	logRequests(handler).ServeHTTP(httptest.NewRecorder(), req)
	for _, a := range capturedAttrs {
		if a.Key == "run_id" {
			t.Errorf("request log carries run_id; it belongs on state events only")
		}
	}

	capturedAttrs = nil
	logState("PLC connected", "host", "192.168.0.10")
	var found bool
	for _, a := range capturedAttrs {
		if a.Key == "run_id" {
			found = true
			if got := a.Value.String(); got != runID {
				t.Errorf("run_id = %q, want %q", got, runID)
			}
		}
	}
	if !found {
		t.Error("state event is missing run_id")
	}
}

func TestStateEventHandlerWritesOnlyStateInfoBelowThreshold(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(newStateEventHandler(base))

	logger.Info("request", "status", http.StatusOK)
	if buf.Len() != 0 {
		t.Fatalf("ordinary Info log was written below Warn threshold: %s", buf.String())
	}

	logStateAt(logger, slog.LevelInfo, "PLC reconnected", "host", "192.168.0.10")
	got := buf.String()
	if !strings.Contains(got, "msg=\"PLC reconnected\"") {
		t.Errorf("state event missing from file log: %s", got)
	}
	if !strings.Contains(got, "kind=state") || !strings.Contains(got, "run_id="+runID) {
		t.Errorf("state event identifiers missing: %s", got)
	}

	buf.Reset()
	logger.Warn("PLC reconnect failed")
	if !strings.Contains(buf.String(), "msg=\"PLC reconnect failed\"") {
		t.Errorf("Warn log was filtered out: %s", buf.String())
	}
}

func TestStateEventHandlerWithErrorThreshold(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	logger := slog.New(newStateEventHandler(base))

	logger.Info("ordinary Info")
	logger.Warn("ordinary Warn")
	logStateAt(logger, slog.LevelInfo, "PLC reconnected")
	logStateAt(logger, slog.LevelWarn, "PLC connection lost")

	got := buf.String()
	for _, want := range []string{"PLC reconnected", "PLC connection lost"} {
		if !strings.Contains(got, want) {
			t.Errorf("state event %q missing from Error-threshold log: %s", want, got)
		}
	}
	for _, unwanted := range []string{"ordinary Info", "ordinary Warn"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("non-state event %q bypassed Error threshold: %s", unwanted, got)
		}
	}
}

// levelCaptureHandler captures the level of the last slog record it receives.
type levelCaptureHandler struct{ level *slog.Level }

func (h *levelCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *levelCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.level = r.Level
	return nil
}
func (h *levelCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *levelCaptureHandler) WithGroup(_ string) slog.Handler      { return h }

// attrCaptureHandler collects all slog attributes from the last record.
type attrCaptureHandler struct{ attrs *[]slog.Attr }

func (h *attrCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *attrCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.attrs = nil
	r.Attrs(func(a slog.Attr) bool {
		*h.attrs = append(*h.attrs, a)
		return true
	})
	return nil
}
func (h *attrCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *attrCaptureHandler) WithGroup(_ string) slog.Handler      { return h }
