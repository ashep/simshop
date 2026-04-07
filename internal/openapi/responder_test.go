package openapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/openapi"
	"github.com/stretchr/testify/assert"
)

func buildResponder(t *testing.T) *openapi.Responder {
	t.Helper()
	return buildOpenAPI(t).Responder()
}

func TestResponder(main *testing.T) {
	main.Run("ValidResponse", func(t *testing.T) {
		resp := buildResponder(t)
		req := httptest.NewRequest(http.MethodPost, "/product", nil)
		rr := httptest.NewRecorder()

		err := resp.Write(rr, req, http.StatusCreated, map[string]string{
			"id":   "550e8400-e29b-41d4-a716-446655440000",
			"name": "Widget",
		})

		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rr.Code)
		assert.JSONEq(t, `{"id":"550e8400-e29b-41d4-a716-446655440000","name":"Widget"}`, rr.Body.String())
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	})

	main.Run("MissingRequiredField", func(t *testing.T) {
		resp := buildResponder(t)
		req := httptest.NewRequest(http.MethodPost, "/product", nil)
		rr := httptest.NewRecorder()

		err := resp.Write(rr, req, http.StatusCreated, map[string]string{"name": "Widget"})

		assert.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	main.Run("UndeclaredStatusCode", func(t *testing.T) {
		resp := buildResponder(t)
		req := httptest.NewRequest(http.MethodPost, "/product", nil)
		rr := httptest.NewRecorder()

		err := resp.Write(rr, req, http.StatusAccepted, map[string]string{
			"id":   "550e8400-e29b-41d4-a716-446655440000",
			"name": "Widget",
		})

		assert.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	main.Run("RouteNotInSpec", func(t *testing.T) {
		resp := buildResponder(t)
		req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
		rr := httptest.NewRecorder()

		err := resp.Write(rr, req, http.StatusOK, nil)

		assert.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Empty(t, rr.Body.String())
	})

	main.Run("NilBodyOnKnownRoute", func(t *testing.T) {
		resp := buildResponder(t)
		req := httptest.NewRequest(http.MethodPost, "/product", nil)
		rr := httptest.NewRecorder()

		err := resp.Write(rr, req, http.StatusOK, nil)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Empty(t, rr.Body.String())
	})
}
