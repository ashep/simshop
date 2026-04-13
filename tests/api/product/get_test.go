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

const (
	testProductID   = "018f4e3a-0000-7000-8000-000000000099"
	testProductYAML = `data:
  EN:
    title: Widget
    description: A fine widget
prices:
  DEFAULT: 500
  US: 800
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

func TestGetProduct(main *testing.T) {
	dataDir := makeDataDir(main, map[string]string{testProductID: testProductYAML})
	app := testapp.New(main, dataDir)
	app.Start()

	doRequest := func(t *testing.T, id string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/"+id), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, testProductID)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, testProductID, body["id"])
		assert.Contains(t, body, "data")
		assert.NotNil(t, body["data"])
		assert.NotContains(t, body, "created_at")
		assert.NotContains(t, body, "updated_at")

		data := body["data"].(map[string]any)
		en := data["EN"].(map[string]any)
		assert.Equal(t, "Widget", en["title"])
		assert.Equal(t, "A fine widget", en["description"])
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-0000-0000-000000000000")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(body))
	})
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

func TestGetProductPrice(main *testing.T) {
	dataDir := makeDataDir(main, map[string]string{testProductID: testProductYAML})
	app := testapp.New(main, dataDir)
	app.Start()

	doRequest := func(t *testing.T, id, country string) *http.Response {
		t.Helper()
		url := app.URL("/products/" + id + "/prices")
		if country != "" {
			url += "?country=" + country
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("ExactCountry", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, testProductID, "US")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "US", body["country_id"])
		assert.Equal(t, float64(800), body["value"])
	})

	main.Run("DefaultFallback", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, testProductID, "UA")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		// handler echoes the requested country, even when price came from DEFAULT
		assert.Equal(t, "UA", body["country_id"])
		assert.Equal(t, float64(500), body["value"])
	})

	main.Run("NoCountryParam_UsesDefault", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, testProductID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "DEFAULT", body["country_id"])
		assert.Equal(t, float64(500), body["value"])
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-0000-0000-000000000000", "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
