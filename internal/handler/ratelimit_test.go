package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitMiddleware(main *testing.T) {
	main.Run("FirstRequest_Passes", func(t *testing.T) {
		mw := RateLimitMiddleware(1)
		req := httptest.NewRequest(http.MethodPost, "/orders", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	main.Run("SecondRequestWithin60s_Returns429", func(t *testing.T) {
		mw := RateLimitMiddleware(1)
		req := httptest.NewRequest(http.MethodPost, "/orders", nil)
		req.RemoteAddr = "1.2.3.4:5678"

		// First request: allowed
		w1 := httptest.NewRecorder()
		mw(nopHandler)(w1, req)

		// Second request immediately after: rate limited
		w2 := httptest.NewRecorder()
		mw(nopHandler)(w2, req)

		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w2.Code)
		}
		if ct := w2.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if body := w2.Body.String(); body != `{"error": "rate limit exceeded"}` {
			t.Errorf("unexpected body: %s", body)
		}
		if ra := w2.Header().Get("Retry-After"); ra == "" {
			t.Error("expected Retry-After header to be set")
		}
	})

	main.Run("DifferentIPs_IndependentLimits", func(t *testing.T) {
		mw := RateLimitMiddleware(1)

		// IP A: first request
		reqA := httptest.NewRequest(http.MethodPost, "/orders", nil)
		reqA.RemoteAddr = "1.1.1.1:1111"
		wA1 := httptest.NewRecorder()
		mw(nopHandler)(wA1, reqA)

		// IP B: first request — must not be rate-limited by IP A's state
		reqB := httptest.NewRequest(http.MethodPost, "/orders", nil)
		reqB.RemoteAddr = "2.2.2.2:2222"
		wB := httptest.NewRecorder()
		mw(nopHandler)(wB, reqB)

		if wB.Code != http.StatusOK {
			t.Errorf("expected status %d for IP B, got %d", http.StatusOK, wB.Code)
		}
	})

	main.Run("RequestAfterWindowExpiry_Passes", func(t *testing.T) {
		rl := &rateLimiter{
			rpm:     1,
			entries: make(map[string]time.Time),
		}
		mw := rl.middleware()

		req := httptest.NewRequest(http.MethodPost, "/orders", nil)
		req.RemoteAddr = "3.3.3.3:9999"

		// Seed a stale entry (61 seconds ago)
		rl.mu.Lock()
		rl.entries["3.3.3.3"] = time.Now().Add(-61 * time.Second)
		rl.mu.Unlock()

		w := httptest.NewRecorder()
		mw(nopHandler)(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d after window expiry, got %d", http.StatusOK, w.Code)
		}
	})

	main.Run("XForwardedFor_UsedAsClientIP", func(t *testing.T) {
		mw := RateLimitMiddleware(1)

		req := httptest.NewRequest(http.MethodPost, "/orders", nil)
		req.Header.Set("X-Forwarded-For", "9.9.9.9")
		req.RemoteAddr = "127.0.0.1:1234"

		// First request: allowed
		w1 := httptest.NewRecorder()
		mw(nopHandler)(w1, req)

		// Same XFF IP again: rate limited
		w2 := httptest.NewRecorder()
		mw(nopHandler)(w2, req)

		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("expected status %d for XFF IP, got %d", http.StatusTooManyRequests, w2.Code)
		}
	})
}
