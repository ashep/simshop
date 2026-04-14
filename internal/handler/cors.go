package handler

import "net/http"

// CORSMiddleware returns a middleware that sets CORS headers for requests whose
// Origin matches one of the allowedOrigins. If allowedOrigins contains "*",
// all origins are allowed and the wildcard is echoed back. OPTIONS preflight
// requests are responded to immediately with 204 No Content when the origin
// is allowed.
func CORSMiddleware(allowedOrigins []string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := matchOrigin(origin, allowedOrigins)

			if allowed != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowed)

				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			next(w, r)
		}
	}
}

func matchOrigin(origin string, allowed []string) string {
	for _, a := range allowed {
		if a == "*" {
			return "*"
		}
		if a == origin {
			return origin
		}
	}
	return ""
}
