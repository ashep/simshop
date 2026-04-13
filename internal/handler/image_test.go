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

func TestServeImage(main *testing.T) {
	dataDir := main.TempDir()
	imgDir := filepath.Join(dataDir, "products", "prod-abc", "images")
	require.NoError(main, os.MkdirAll(imgDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(imgDir, "photo.jpg"), []byte("JPEGDATA"), 0644))

	newH := func() *Handler {
		return &Handler{dataDir: dataDir, l: zerolog.Nop()}
	}

	doRequest := func(t *testing.T, productID, fileName string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/images/"+productID+"/"+fileName, nil)
		r.SetPathValue("product_id", productID)
		r.SetPathValue("file_name", fileName)
		w := httptest.NewRecorder()
		newH().ServeImage(w, r)
		return w
	}

	main.Run("ReturnsFile", func(t *testing.T) {
		w := doRequest(t, "prod-abc", "photo.jpg")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "JPEGDATA", w.Body.String())
	})

	main.Run("NotFoundWhenProductMissing", func(t *testing.T) {
		w := doRequest(t, "no-such-product", "photo.jpg")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundWhenFileMissing", func(t *testing.T) {
		w := doRequest(t, "prod-abc", "missing.jpg")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnProductIDPathTraversal", func(t *testing.T) {
		// Simulate injected path value — URL itself is clean, but path value contains traversal.
		r := httptest.NewRequest(http.MethodGet, "/images/prod-abc/photo.jpg", nil)
		r.SetPathValue("product_id", "../etc")
		r.SetPathValue("file_name", "photo.jpg")
		w := httptest.NewRecorder()
		newH().ServeImage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnFileNamePathTraversal", func(t *testing.T) {
		// Simulate injected path value — URL itself is clean, but path value contains traversal.
		r := httptest.NewRequest(http.MethodGet, "/images/prod-abc/photo.jpg", nil)
		r.SetPathValue("product_id", "prod-abc")
		r.SetPathValue("file_name", "../product.yaml")
		w := httptest.NewRecorder()
		newH().ServeImage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
