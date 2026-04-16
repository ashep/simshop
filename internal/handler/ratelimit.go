package handler

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]time.Time
	rpm     int
}

// RateLimitMiddleware returns a middleware that limits each client IP to rpm
// requests per minute. Excess requests receive HTTP 429 with a Retry-After
// header and a JSON error body.
func RateLimitMiddleware(rpm int) func(http.HandlerFunc) http.HandlerFunc {
	rl := &rateLimiter{
		entries: make(map[string]time.Time),
		rpm:     rpm,
	}
	go rl.sweep()
	return rl.middleware()
}

func (rl *rateLimiter) middleware() func(http.HandlerFunc) http.HandlerFunc {
	window := time.Minute / time.Duration(rl.rpm)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := rateLimitClientIP(r)
			if ip == "" {
				next(w, r)
				return
			}

			rl.mu.Lock()
			last, seen := rl.entries[ip]
			now := time.Now()
			if seen && now.Sub(last) < window {
				retryAfter := int(math.Ceil(window.Seconds() - now.Sub(last).Seconds()))
				rl.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprint(w, `{"error": "rate limit exceeded"}`)
				return
			}
			rl.entries[ip] = now
			rl.mu.Unlock()

			next(w, r)
		}
	}
}

// sweep removes entries older than one minute to prevent unbounded map growth.
// It runs in a background goroutine and never exits.
func (rl *rateLimiter) sweep() {
	window := time.Minute / time.Duration(rl.rpm)
	ticker := time.NewTicker(window)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-window)
		rl.mu.Lock()
		for ip, last := range rl.entries {
			if last.Before(cutoff) {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitClientIP extracts the client IP from the request, preferring the
// first entry in X-Forwarded-For (set by Cloudflare) over RemoteAddr.
func rateLimitClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	return ""
}
