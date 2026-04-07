package auth

import (
	"context"
	"net/http"
)

type userCtxKey struct{}

type authSvc interface {
	GetUserByAPIKey(context.Context, string) (*User, error)
}

// Middleware returns an HTTP middleware that enforces API key authentication
// via the X-API-Key header. Requests without a valid key are rejected with
// 403 Forbidden. The comparison is timing-safe.
func Middleware(svc authSvc) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				w.WriteHeader(http.StatusForbidden)
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

func ContextWithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

func GetUserFromContext(ctx context.Context) *User {
	v := ctx.Value(userCtxKey{})
	u, ok := v.(*User)
	if !ok {
		return nil
	}
	return u
}
