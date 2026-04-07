package openapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ashep/simshop/internal/openapi"
	"github.com/stretchr/testify/assert"
)

func buildMiddleware(t *testing.T) func(http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return buildOpenAPI(t).Middleware()
}

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func TestMiddleware(main *testing.T) {
	main.Run("ValidRequest", func(t *testing.T) {
		mw := buildMiddleware(t)
		body := `{"name":"Widget","id":"550e8400-e29b-41d4-a716-446655440000"}`
		req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	main.Run("ValidRequestNoID", func(t *testing.T) {
		mw := buildMiddleware(t)
		body := `{"name":"Widget"}`
		req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	main.Run("MissingRequiredField", func(t *testing.T) {
		mw := buildMiddleware(t)
		body := `{}`
		req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), `"error"`)
	})

	main.Run("InvalidUUID", func(t *testing.T) {
		mw := buildMiddleware(t)
		body := `{"name":"Widget","id":"not-a-uuid"}`
		req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), `"error"`)
	})

	main.Run("UnknownRoute_PassesThrough", func(t *testing.T) {
		mw := buildMiddleware(t)
		req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
		rr := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	main.Run("HandlerCanReadBody", func(t *testing.T) {
		mw := buildMiddleware(t)
		body := `{"name":"Widget"}`
		req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		var readBody []byte
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			readBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		})
		mw(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, body, string(readBody))
	})

	main.Run("InvalidSpec_ReturnsError", func(t *testing.T) {
		specFS := fstest.MapFS{"root.yaml": {Data: []byte("not: valid: openapi")}}
		_, err := openapi.New(specFS)
		assert.Error(t, err)
	})
}
