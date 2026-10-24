package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPromoCode(main *testing.T) {
	resp := buildTestResponder(main)
	dataDir := main.TempDir()
	productDir := filepath.Join(dataDir, "products", "widget")
	require.NoError(main, os.MkdirAll(productDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(productDir, "product.yaml"),
		[]byte(testProductYAML+"promo_codes:\n  SPRING15: 15\n"), 0644))

	doRequest := func(t *testing.T, id, code string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/promo-codes/x", nil)
		r.SetPathValue("id", id)
		r.SetPathValue("code", code)
		w := httptest.NewRecorder()
		h.CheckPromoCode(w, r)
		return w
	}

	main.Run("ReturnsCanonicalCodeAndPercent", func(t *testing.T) {
		w := doRequest(t, "widget", "spring15")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.JSONEq(t, `{"code":"SPRING15","discount_percent":15}`, w.Body.String())
	})

	main.Run("UnknownCodeReturns404", func(t *testing.T) {
		w := doRequest(t, "widget", "NOPE")
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"promo code not found"}`, w.Body.String())
	})

	main.Run("UnknownProductReturns404", func(t *testing.T) {
		w := doRequest(t, "ghost", "SPRING15")
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"promo code not found"}`, w.Body.String())
	})

	main.Run("PathTraversalIDReturns404", func(t *testing.T) {
		w := doRequest(t, "..", "SPRING15")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("CodeWithSpaceReturns404", func(t *testing.T) {
		w := doRequest(t, "widget", "a b")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
