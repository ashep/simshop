//go:build functest

package product_test

import (
	"encoding/json"
	"io"
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

// makeDataDir creates a data directory with a products.yaml listing and per-language
// markdown files for each product.
func makeDataDir(t *testing.T, productsYAML string, markdownFiles map[string]map[string]string) string {
	t.Helper()
	dataDir := t.TempDir()

	if productsYAML != "" {
		productsDir := filepath.Join(dataDir, "products")
		require.NoError(t, os.MkdirAll(productsDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(productsDir, "products.yaml"), []byte(productsYAML), 0644))
	}

	for id, langs := range markdownFiles {
		productDir := filepath.Join(dataDir, "products", id)
		require.NoError(t, os.MkdirAll(productDir, 0755))
		for lang, content := range langs {
			require.NoError(t, os.WriteFile(filepath.Join(productDir, lang+".md"), []byte(content), 0644))
		}
	}

	return dataDir
}

func TestListProducts(main *testing.T) {
	dataDir := makeDataDir(main, testProductsYAML, map[string]map[string]string{
		"cronus": {"en": "# Cronus", "uk": "# Кронос"},
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

	main.Run("GetReturnsContent", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/cronus/en"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "# Cronus", string(body))
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
