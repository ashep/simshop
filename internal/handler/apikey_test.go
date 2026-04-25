package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIKeyMiddleware(main *testing.T) {
	const goodKey = "s3cret"

	makeHandler := func(called *bool) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			*called = true
			w.WriteHeader(http.StatusOK)
		}
	}

	doRequest := func(t *testing.T, header string) (*httptest.ResponseRecorder, bool) {
		t.Helper()
		called := false
		mw := APIKeyMiddleware(goodKey)
		h := mw(makeHandler(&called))
		r := httptest.NewRequest(http.MethodGet, "/orders", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w, called
	}

	main.Run("MissingHeaderReturns401", func(t *testing.T) {
		w, called := doRequest(t, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "missing or invalid authorization header")
		assert.False(t, called)
	})

	main.Run("WrongSchemeReturns401", func(t *testing.T) {
		w, called := doRequest(t, "Basic dXNlcjpwYXNz")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "missing or invalid authorization header")
		assert.False(t, called)
	})

	main.Run("BearerWithoutValueReturns401", func(t *testing.T) {
		w, called := doRequest(t, "Bearer ")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "invalid api key")
		assert.False(t, called)
	})

	main.Run("WrongKeyReturns401", func(t *testing.T) {
		w, called := doRequest(t, "Bearer wrong")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "invalid api key")
		assert.False(t, called)
	})

	main.Run("CorrectKeyCallsNext", func(t *testing.T) {
		w, called := doRequest(t, "Bearer "+goodKey)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, called)
	})
}
