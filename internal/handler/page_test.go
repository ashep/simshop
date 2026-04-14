package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPages(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, dataDir string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages", nil)
		w := httptest.NewRecorder()
		h.ListPages(w, r)
		return w
	}

	main.Run("EmptyWhenNoPagesDir", func(t *testing.T) {
		dataDir := t.TempDir()
		w := doRequest(t, dataDir)
		assert.Equal(t, http.StatusOK, w.Code)
		var body []string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("ReturnsSubdirNamesSkipsFiles", func(t *testing.T) {
		dataDir := t.TempDir()
		pagesDir := filepath.Join(dataDir, "pages")
		require.NoError(t, os.MkdirAll(filepath.Join(pagesDir, "about"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pagesDir, "readme.txt"), []byte("x"), 0644))

		w := doRequest(t, dataDir)
		assert.Equal(t, http.StatusOK, w.Code)
		var body []string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, []string{"about"}, body)
	})
}

func TestServePage(main *testing.T) {
	dataDir := main.TempDir()
	pageDir := filepath.Join(dataDir, "pages", "about")
	require.NoError(main, os.MkdirAll(pageDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(pageDir, "en.md"), []byte("# About"), 0644))

	doRequest := func(t *testing.T, id, lang string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/"+id+"/"+lang, nil)
		r.SetPathValue("id", id)
		r.SetPathValue("lang", lang)
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		return w
	}

	main.Run("ReturnsContent", func(t *testing.T) {
		w := doRequest(t, "about", "en")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "# About", w.Body.String())
	})

	main.Run("NotFoundWhenIDMissing", func(t *testing.T) {
		w := doRequest(t, "no-such-page", "en")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundWhenLangMissing", func(t *testing.T) {
		w := doRequest(t, "about", "uk")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnIDPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/about/en", nil)
		r.SetPathValue("id", "../about") // would resolve to "about" if traversal were allowed
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnLangPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/about/en", nil)
		r.SetPathValue("id", "about")
		r.SetPathValue("lang", "../en") // would resolve to "en" if traversal were allowed
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
