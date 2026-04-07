package contenttype_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/contenttype"
	"github.com/stretchr/testify/assert"
)

func nextHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	h := contenttype.Middleware()(nextHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"unsupported media type"}`, rr.Body.String())
}

func TestMiddleware_WrongContentType(t *testing.T) {
	h := contenttype.Middleware()(nextHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"unsupported media type"}`, rr.Body.String())
}

func TestMiddleware_UnparseableContentType(t *testing.T) {
	h := contenttype.Middleware()(nextHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", ";;;")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"unsupported media type"}`, rr.Body.String())
}

func TestMiddleware_ApplicationJSON(t *testing.T) {
	h := contenttype.Middleware()(nextHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestMiddleware_ApplicationJSONWithParams(t *testing.T) {
	h := contenttype.Middleware()(nextHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}
