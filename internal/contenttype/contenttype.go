package contenttype

import (
	"mime"
	"net/http"
)

// Middleware returns an HTTP middleware that enforces application/json
// Content-Type on incoming requests. Requests with a missing, unparseable,
// or non-application/json Content-Type are rejected with 415 Unsupported
// Media Type and a JSON error body.
func Middleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_, _ = w.Write([]byte(`{"error":"unsupported media type"}`))
				return
			}
			next(w, r)
		}
	}
}
