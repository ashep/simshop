package auth

import (
	"context"
	"net/http"
)

// OptionalMiddleware is like Middleware but does not reject unauthenticated
// requests. If no X-API-Key header is present the request continues without
// a user in context. If a key is present but invalid, 403 is returned.
func OptionalMiddleware(svc authSvc) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			u, err := svc.GetUserByAPIKey(r.Context(), key)
			if err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey{}, u)))
		}
	}
}
