//go:build functest

package pages_test

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

// makeDataDir creates a data directory with a pages.yaml listing the given pages
// and per-language markdown files for each page.
func makeDataDir(t *testing.T, pagesYAML string, markdownFiles map[string]map[string]string) string {
	t.Helper()
	dataDir := t.TempDir()

	if pagesYAML != "" {
		pagesDir := filepath.Join(dataDir, "pages")
		require.NoError(t, os.MkdirAll(pagesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pagesDir, "pages.yaml"), []byte(pagesYAML), 0644))
	}

	for id, langs := range markdownFiles {
		pageDir := filepath.Join(dataDir, "pages", id)
		require.NoError(t, os.MkdirAll(pageDir, 0755))
		for lang, content := range langs {
			require.NoError(t, os.WriteFile(filepath.Join(pageDir, lang+".md"), []byte(content), 0644))
		}
	}

	return dataDir
}

const testPagesYAML = `
pages:
  - id: about
    title:
      en: About
      uk: Про нас
`

func TestGetPages(main *testing.T) {
	dataDir := makeDataDir(main, testPagesYAML, map[string]map[string]string{
		"about": {"en": "# About", "uk": "# Про нас"},
	})
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ListReturnsPagesWithTitles", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/pages"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, "about", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "About", title["en"])
		assert.Equal(t, "Про нас", title["uk"])
	})

	main.Run("ListEmptyWhenNoPagesYAML", func(t *testing.T) {
		emptyDir := makeDataDir(t, "", nil)
		emptyApp := testapp.New(t, emptyDir)
		emptyApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, emptyApp.URL("/pages"), nil)
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
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/pages/about/en"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "# About", string(body))
	})

	main.Run("GetNotFoundWhenIDMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/pages/no-such-page/en"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("GetNotFoundWhenLangMissing", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/pages/about/fr"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
