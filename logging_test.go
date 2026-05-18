package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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

// levelCaptureHandler captures the level of the last slog record it receives.
type levelCaptureHandler struct{ level *slog.Level }

func (h *levelCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *levelCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.level = r.Level
	return nil
}
func (h *levelCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler  { return h }
func (h *levelCaptureHandler) WithGroup(_ string) slog.Handler        { return h }
