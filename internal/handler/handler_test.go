package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func newTestHandler() *Handler {
	return &Handler{l: zerolog.Nop()}
}

func TestWriteError(main *testing.T) {
	main.Run("BadRequest", func(t *testing.T) {
		h := newTestHandler()
		w := httptest.NewRecorder()

		h.writeError(w, &BadRequestError{Reason: "something is wrong"})

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if body := w.Body.String(); body != `{"error": "something is wrong"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("NotFound", func(t *testing.T) {
		h := newTestHandler()
		w := httptest.NewRecorder()

		h.writeError(w, &NotFoundError{Reason: "product not found"})

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if body := w.Body.String(); body != `{"error": "product not found"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})
}
