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

func TestServeAsset(main *testing.T) {
	dataDir := main.TempDir()
	assetsDir := filepath.Join(dataDir, "assets")
	require.NoError(main, os.MkdirAll(filepath.Join(assetsDir, "css"), 0755))
	require.NoError(main, os.WriteFile(filepath.Join(assetsDir, "logo.svg"), []byte("SVGDATA"), 0644))
	require.NoError(main, os.WriteFile(filepath.Join(assetsDir, "css", "style.css"), []byte("CSSDATA"), 0644))

	newH := func() *Handler {
		return &Handler{dataDir: dataDir, l: zerolog.Nop()}
	}

	doRequest := func(t *testing.T, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/assets/"+path, nil)
		r.SetPathValue("path", path)
		w := httptest.NewRecorder()
		newH().ServeAsset(w, r)
		return w
	}

	main.Run("ReturnsTopLevelFile", func(t *testing.T) {
		w := doRequest(t, "logo.svg")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "SVGDATA", w.Body.String())
	})

	main.Run("ReturnsNestedFile", func(t *testing.T) {
		w := doRequest(t, "css/style.css")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "CSSDATA", w.Body.String())
	})

	main.Run("NotFoundWhenFileMissing", func(t *testing.T) {
		w := doRequest(t, "missing.png")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnDirectory", func(t *testing.T) {
		w := doRequest(t, "css")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnPathTraversal", func(t *testing.T) {
		// URL stays clean; the injected path value attempts to escape the assets dir.
		r := httptest.NewRequest(http.MethodGet, "/assets/logo.svg", nil)
		r.SetPathValue("path", "../shop.yaml")
		w := httptest.NewRecorder()
		newH().ServeAsset(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
