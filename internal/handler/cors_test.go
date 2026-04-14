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
	main.Run("AnyOrigin_WildcardHeaderSet", func(t *testing.T) {
		mw := CORSMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected ACAO header %q, got %q", "*", got)
		}
	})

	main.Run("NoOriginHeader_WildcardHeaderSet", func(t *testing.T) {
		mw := CORSMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected ACAO header %q, got %q", "*", got)
		}
	})

	main.Run("Preflight_Returns204WithHeaders", func(t *testing.T) {
		mw := CORSMiddleware()
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "http://localhost:63342")
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()

		mw(nopHandler)(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected ACAO header %q, got %q", "*", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
			t.Error("expected Access-Control-Allow-Methods header to be set")
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
			t.Error("expected Access-Control-Allow-Headers header to be set")
		}
	})
}
