package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogRequestsStatusCode(t *testing.T) {
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
