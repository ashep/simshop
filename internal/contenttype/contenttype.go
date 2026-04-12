package contenttype

import (
	"mime"
	"net/http"
)

// Middleware returns an HTTP middleware that enforces the given media type
// on incoming requests. Requests with a missing, unparseable, or non-matching
// Content-Type are rejected with 415 Unsupported Media Type and a JSON error body.
func Middleware(mediaType string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != mediaType {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_, _ = w.Write([]byte(`{"error":"unsupported media type"}`))
				return
			}
			next(w, r)
		}
	}
}
