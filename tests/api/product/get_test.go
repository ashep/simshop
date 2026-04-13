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

const (
	testProductID   = "018f4e3a-0000-7000-8000-000000000099"
	testProductYAML = `data:
  EN:
    title: Widget
    description: A fine widget
`
)

func makeDataDir(t *testing.T, products map[string]string) string {
	t.Helper()
	dataDir := t.TempDir()
	prodsDir := filepath.Join(dataDir, "products")
	require.NoError(t, os.MkdirAll(prodsDir, 0755))
	for id, yaml := range products {
		require.NoError(t, os.WriteFile(filepath.Join(prodsDir, id+".yaml"), []byte(yaml), 0644))
	}
	return dataDir
}

func TestListProducts(main *testing.T) {
	dataDir := makeDataDir(main, map[string]string{testProductID: testProductYAML})
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
		assert.Equal(t, testProductID, body[0]["id"])
		assert.Contains(t, body[0], "data")
	})

	main.Run("EmptyWhenNoProducts", func(t *testing.T) {
		t.Parallel()

		emptyDir := t.TempDir()
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
}
