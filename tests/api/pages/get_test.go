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

func makeDataDir(t *testing.T, pages map[string]map[string]string) string {
	t.Helper()
	dataDir := t.TempDir()
	for id, langs := range pages {
		pageDir := filepath.Join(dataDir, "pages", id)
		require.NoError(t, os.MkdirAll(pageDir, 0755))
		for lang, content := range langs {
			require.NoError(t, os.WriteFile(filepath.Join(pageDir, lang+".md"), []byte(content), 0644))
		}
	}
	return dataDir
}

func TestGetPages(main *testing.T) {
	dataDir := makeDataDir(main, map[string]map[string]string{
		"about": {"en": "# About", "uk": "# Про нас"},
	})
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ListReturnsPageIDs", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/pages"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body []string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, []string{"about"}, body)
	})

	main.Run("ListEmptyWhenNoPagesDir", func(t *testing.T) {
		emptyDir := t.TempDir()
		emptyApp := testapp.New(t, emptyDir)
		emptyApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, emptyApp.URL("/pages"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body []string
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
