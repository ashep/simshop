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

func TestGetProductFiles(main *testing.T) {
	const fileProductID = "018f4e3a-0000-7000-8000-000000000098"

	// JPEG header bytes — enough for http.DetectContentType to return "image/jpeg"
	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	productYAML := `data:
  EN:
    title: Photo Widget
    description: Has a photo
files:
  - photo.jpg
`
	dataDir := makeDataDir(main, map[string]string{fileProductID: productYAML})
	publicDir := main.TempDir()

	fileDir := filepath.Join(publicDir, fileProductID)
	require.NoError(main, os.MkdirAll(fileDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(fileDir, "photo.jpg"), jpegBytes, 0644))

	app := testapp.NewWithPublicDir(main, dataDir, publicDir)
	app.Start()

	doRequest := func(t *testing.T, id string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/products/"+id+"/files"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-0000-0000-000000000000")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(body))
	})

	main.Run("WithFile", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, fileProductID)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var items []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
		require.Len(t, items, 1)
		assert.Equal(t, "photo.jpg", items[0]["name"])
		assert.Equal(t, "image/jpeg", items[0]["mime_type"])
		assert.Equal(t, float64(len(jpegBytes)), items[0]["size_bytes"])
		assert.Equal(t, "/files/"+fileProductID+"/photo.jpg", items[0]["path"])
		assert.NotContains(t, items[0], "id")
		assert.NotContains(t, items[0], "created_at")
		assert.NotContains(t, items[0], "updated_at")
	})
}
