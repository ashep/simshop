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
  - id: widget
    title:
      en: Widget
      uk: Віджет
    description:
      en: A test product
      uk: Тестовий продукт
`

const testProductYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
  ua:
    currency: UAH
    value: 1999.99
`

const testProductWithAttrPricesYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
  ua:
    currency: UAH
    value: 1999.99
attrs:
  display_color:
    en:
      title: Display color
      values:
        red:
          title: Red
        green:
          title: Green
    uk:
      title: Колір дисплея
      values:
        red:
          title: Червоний
        green:
          title: Зелений
attr_prices:
  display_color:
    red:
      default: 10
      ua: 5
    green:
      default: 8
      ua: 3
`

const testProductWithAttrImagesYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attr_images:
  display_color:
    red: red-thumb.jpg
    green: green-thumb.jpg
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
		"widget": testProductYAML,
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
		assert.Equal(t, "widget", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Widget", title["en"])
		assert.Equal(t, "Віджет", title["uk"])
		description, ok := body[0]["description"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "A test product", description["en"])
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
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/widget/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "widget", body["id"])
		assert.Equal(t, "Widget", body["name"])
		assert.Equal(t, "A test product", body["description"])
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "USD", price["currency"])
		assert.Equal(t, 49.99, price["value"])
	})

	main.Run("GetReturnsCorrectLanguage", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/widget/uk"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "Віджет", body["name"])
		assert.Equal(t, "Тестовий продукт", body["description"])
	})

	main.Run("GetReturnsCountryPrice", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/widget/en"), nil)
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
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/widget/en"), nil)
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
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/widget/fr"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("GetReturnsAttrPricesResolvedByCountry", func(t *testing.T) {
		apDir := makeDataDir(t, "", map[string]string{"widget": testProductWithAttrPricesYAML})
		apApp := testapp.New(t, apDir)
		apApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, apApp.URL("/products/widget/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "UA")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		attrPrices, ok := body["attr_prices"].(map[string]any)
		require.True(t, ok, "attr_prices should be present")
		displayColor, ok := attrPrices["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 5.0, displayColor["red"])
		assert.Equal(t, 3.0, displayColor["green"])
	})

	main.Run("GetReturnsAttrImages", func(t *testing.T) {
		aiDataDir := t.TempDir()
		productDir := filepath.Join(aiDataDir, "products", "widget")
		imagesDir := filepath.Join(productDir, "images")
		require.NoError(t, os.MkdirAll(imagesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(productDir, "product.yaml"),
			[]byte(testProductWithAttrImagesYAML), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "red-thumb.jpg"), []byte("fake"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "green-thumb.jpg"), []byte("fake"), 0644))

		aiApp := testapp.New(t, aiDataDir)
		aiApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			aiApp.URL("/products/widget/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		attrImages, ok := body["attr_images"].(map[string]any)
		require.True(t, ok, "attr_images should be present")
		displayColor, ok := attrImages["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "/images/widget/red-thumb.jpg", displayColor["red"])
		assert.Equal(t, "/images/widget/green-thumb.jpg", displayColor["green"])
	})

	main.Run("GetReturnsAttrPricesWithDefaultFallback", func(t *testing.T) {
		apDir := makeDataDir(t, "", map[string]string{"widget": testProductWithAttrPricesYAML})
		apApp := testapp.New(t, apDir)
		apApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, apApp.URL("/products/widget/en"), nil)
		require.NoError(t, err)
		req.Header.Set("CF-IPCountry", "ZZ")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		attrPrices, ok := body["attr_prices"].(map[string]any)
		require.True(t, ok, "attr_prices should be present")
		displayColor, ok := attrPrices["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 10.0, displayColor["red"])
		assert.Equal(t, 8.0, displayColor["green"])
	})
}
