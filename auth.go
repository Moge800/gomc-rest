package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authExemptPaths are reachable without a token even when one is configured,
// so liveness probes keep working for monitoring tools.
var authExemptPaths = map[string]bool{
	"/health": true,
}

// requireToken wraps h with static bearer-token authentication. When token is
// empty (GOMCR_TOKEN unset) it is a no-op, preserving the closed-network
// default of no authentication.
//
// Note: the token travels in cleartext over HTTP. This is intentional for the
// air-gapped FA networks gomc-rest targets; put a TLS-terminating reverse proxy
// in front if transport encryption is required.
func requireToken(h http.Handler, token string) http.Handler {
	if token == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authExemptPaths[r.URL.Path] {
			h.ServeHTTP(w, r)
			return
		}
		if !tokenMatches(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// tokenMatches reports whether the Authorization header carries the expected
// bearer token. The comparison is constant-time to avoid leaking the token via
// response timing.
func tokenMatches(header, token string) bool {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	got := header[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
