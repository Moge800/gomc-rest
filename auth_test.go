package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenMatches(t *testing.T) {
	cases := []struct {
		name   string
		header string
		token  string
		want   bool
	}{
		{"exact match", "Bearer secret", "secret", true},
		{"case-insensitive scheme", "bearer secret", "secret", true},
		{"wrong token", "Bearer nope", "secret", false},
		{"missing scheme", "secret", "secret", false},
		{"empty header", "", "secret", false},
		{"scheme only", "Bearer ", "secret", false},
		{"prefix of token", "Bearer secre", "secret", false},
		{"trailing space differs", "Bearer secret ", "secret", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenMatches(tc.header, tc.token); got != tc.want {
				t.Errorf("tokenMatches(%q, %q) = %v, want %v", tc.header, tc.token, got, tc.want)
			}
		})
	}
}

func TestRequireToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no token configured is a no-op", func(t *testing.T) {
		h := requireToken(ok, "")
		req := httptest.NewRequest(http.MethodGet, "/read", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("valid token passes", func(t *testing.T) {
		h := requireToken(ok, "secret")
		req := httptest.NewRequest(http.MethodGet, "/read", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("missing token rejected with 401", func(t *testing.T) {
		h := requireToken(ok, "secret")
		req := httptest.NewRequest(http.MethodGet, "/read", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Errorf("WWW-Authenticate = %q, want %q", got, "Bearer")
		}
	})

	t.Run("wrong token rejected with 401", func(t *testing.T) {
		h := requireToken(ok, "secret")
		req := httptest.NewRequest(http.MethodGet, "/read", nil)
		req.Header.Set("Authorization", "Bearer nope")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("health is exempt even without token", func(t *testing.T) {
		h := requireToken(ok, "secret")
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 (health should bypass auth)", rec.Code)
		}
	})
}
