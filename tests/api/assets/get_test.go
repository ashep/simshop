//go:build functest

package assets_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
      currency:
        en: UAH
      phone_code: "+380"
`

func makeDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "shop.yaml"), []byte(testShopYAML), 0644))
	assetsDir := filepath.Join(dataDir, "assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetsDir, "css"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "logo.svg"), []byte("SVGDATA"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "css", "style.css"), []byte("CSSDATA"), 0644))
	return dataDir
}

func TestServeAsset(main *testing.T) {
	dataDir := makeDataDir(main)
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ReturnsTopLevelFile", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/assets/logo.svg"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "SVGDATA", string(body))
	})

	main.Run("ReturnsNestedFile", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/assets/css/style.css"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "CSSDATA", string(body))
	})

	main.Run("NotFoundWhenFileMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/assets/missing.png"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
