package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// APIKeyMiddleware returns a middleware that requires "Authorization: Bearer <key>"
// on every request. The candidate key is compared in constant time against the
// configured key. On any failure (header missing, wrong scheme, wrong key) the
// middleware writes 401 with a JSON error body and does not call next.
//
// Passing an empty key into this constructor is a programmer error — callers
// must register the route only when the key is configured.
func APIKeyMiddleware(key string) func(http.HandlerFunc) http.HandlerFunc {
	expected := []byte(key)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, bearerPrefix) {
				writeUnauthorized(w, "missing or invalid authorization header")
				return
			}
			candidate := []byte(auth[len(bearerPrefix):])
			if subtle.ConstantTimeCompare(candidate, expected) != 1 {
				writeUnauthorized(w, "invalid api key")
				return
			}
			next(w, r)
		}
	}
}

func writeUnauthorized(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error": "` + reason + `"}`))
}
