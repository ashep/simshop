package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/shop"
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

type shopServiceStub struct {
	shop *shop.Shop
	err  error
}

func (s *shopServiceStub) Get(_ context.Context) (*shop.Shop, error) {
	return s.shop, s.err
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

	main.Run("BadGateway", func(t *testing.T) {
		h := newTestHandler()
		w := httptest.NewRecorder()

		h.writeError(w, &BadGatewayError{Reason: "upstream service failed"})

		if w.Code != http.StatusBadGateway {
			t.Errorf("expected status %d, got %d", http.StatusBadGateway, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if body := w.Body.String(); body != `{"error": "upstream service failed"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("Unauthorized", func(t *testing.T) {
		h := newTestHandler()
		w := httptest.NewRecorder()

		h.writeError(w, &UnauthorizedError{Reason: "invalid api key"})

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if body := w.Body.String(); body != `{"error": "invalid api key"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})
}
