//go:build functest

package image_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProductID = "018f4e3a-0000-7000-8000-000000000099"

const productYAML = `
name:
  en: Test Product
description:
  en: A test product
prices:
  default:
    currency: USD
    value: 10
images:
  - preview: photo.jpg
    full: photo.jpg
`

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
	prodDir := filepath.Join(dataDir, "products", testProductID)
	imgDir := filepath.Join(prodDir, "images")
	require.NoError(t, os.MkdirAll(imgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(prodDir, "product.yaml"), []byte(productYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(imgDir, "photo.jpg"), []byte("JPEGDATA"), 0644))
	return dataDir
}

func TestServeImage(main *testing.T) {
	dataDir := makeDataDir(main)
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ReturnsFile", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/images/"+testProductID+"/photo.jpg"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	main.Run("NotFoundWhenFileMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/images/"+testProductID+"/missing.jpg"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("NotFoundWhenProductMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/images/no-such-product/photo.jpg"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
