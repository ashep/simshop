package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func buildTestResponder(t *testing.T) *openapi.Responder {
	t.Helper()
	oas, err := openapi.New(api.Spec)
	require.NoError(t, err)
	return oas.Responder()
}

func newTestHandler() *Handler {
	return &Handler{geo: &geoDetectorStub{}, l: zerolog.Nop()}
}

type geoDetectorStub struct{ country string }

func (s *geoDetectorStub) Detect(_ *http.Request) string { return s.country }

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
