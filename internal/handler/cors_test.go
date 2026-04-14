package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func nopHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestCORSMiddleware(main *testing.T) {
	main.Run("NoAllowedOrigins_NoHeaderSet", func(t *testing.T) {
		mw := CORSMiddleware(nil)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no ACAO header, got %q", got)
		}
	})

	main.Run("OriginMatches_HeaderSet", func(t *testing.T) {
		mw := CORSMiddleware([]string{"http://localhost:63342"})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:63342" {
			t.Errorf("expected ACAO header %q, got %q", "http://localhost:63342", got)
		}
	})

	main.Run("OriginDoesNotMatch_NoHeaderSet", func(t *testing.T) {
		mw := CORSMiddleware([]string{"http://example.com"})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no ACAO header, got %q", got)
		}
	})

	main.Run("Wildcard_AllOriginsAllowed", func(t *testing.T) {
		mw := CORSMiddleware([]string{"*"})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected ACAO header %q, got %q", "*", got)
		}
	})

	main.Run("Preflight_Returns204WithHeaders", func(t *testing.T) {
		mw := CORSMiddleware([]string{"http://localhost:63342"})
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:63342" {
			t.Errorf("expected ACAO header %q, got %q", "http://localhost:63342", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
			t.Error("expected Access-Control-Allow-Methods header to be set")
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
			t.Error("expected Access-Control-Allow-Headers header to be set")
		}
	})

	main.Run("Preflight_OriginNotAllowed_PassesThrough", func(t *testing.T) {
		mw := CORSMiddleware([]string{"http://example.com"})
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected handler status %d, got %d", http.StatusOK, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no ACAO header, got %q", got)
		}
	})
}
