//go:build functest

package product_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProductsYAML = `
products:
  - id: cronus
    title:
      en: Cronus
      uk: Cronus
    description:
      en: A wooden desktop clock
      uk: Настільний годинник у деревʼяному корпусі
`

const testCronusProductYAML = `
name:
  en: Cronus
  uk: Кронос
description:
  en: A wooden desktop clock
  uk: Настільний годинник у деревʼяному корпусі
price:
  default:
    currency: USD
    value: 49.99
  ua:
    currency: UAH
    value: 1999.99
`

// makeDataDir creates a data directory with a products.yaml listing and per-product
// product.yaml detail files.
func makeDataDir(t *testing.T, productsYAML string, productYAMLs map[string]string) string {
	t.Helper()
	dataDir := t.TempDir()

	if productsYAML != "" {
		productsDir := filepath.Join(dataDir, "products")
		require.NoError(t, os.MkdirAll(productsDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(productsDir, "products.yaml"), []byte(productsYAML), 0644))
	}

	for id, yaml := range productYAMLs {
		productDir := filepath.Join(dataDir, "products", id)
		require.NoError(t, os.MkdirAll(productDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(yaml), 0644))
	}

	return dataDir
}

func TestListProducts(main *testing.T) {
	dataDir := makeDataDir(main, testProductsYAML, map[string]string{
		"cronus": testCronusProductYAML,
	})
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ReturnsList", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, "cronus", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Cronus", title["en"])
		assert.Equal(t, "Cronus", title["uk"])
		description, ok := body[0]["description"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "A wooden desktop clock", description["en"])
	})

	main.Run("EmptyListWhenNoProductsYAML", func(t *testing.T) {
		emptyDir := makeDataDir(t, "", nil)
		emptyApp := testapp.New(t, emptyDir)
		emptyApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, emptyApp.URL("/products"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body []any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("GetReturnsProductDetail", func(t *testing.T) {
		t.Parallel()
		// CF-IPCountry: XX has no price entry → falls back to default. Avoids live ipinfo.io call.
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "cronus", body["id"])
		assert.Equal(t, "Cronus", body["name"])
		assert.Equal(t, "A wooden desktop clock", body["description"])
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "USD", price["currency"])
		assert.Equal(t, 49.99, price["value"])
	})

	main.Run("GetReturnsCorrectLanguage", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/uk"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "Кронос", body["name"])
		assert.Equal(t, "Настільний годинник у деревʼяному корпусі", body["description"])
	})

	main.Run("GetReturnsCountryPrice", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "UA")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "UAH", price["currency"])
		assert.Equal(t, 1999.99, price["value"])
	})

	main.Run("GetReturnsDefaultPriceForUnknownCountry", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "ZZ")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "USD", price["currency"])
		assert.Equal(t, 49.99, price["value"])
	})

	main.Run("GetNotFoundWhenIDMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/no-such-product/en"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("GetNotFoundWhenLangMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/fr"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
