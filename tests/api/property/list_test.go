//go:build functest

package property_test

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

func TestListProperties(main *testing.T) {
	const propID1 = "018f4e3a-0000-7000-8000-000000000001"
	const propID2 = "018f4e3a-0000-7000-8000-000000000002"

	propertiesYAML := `- id: "` + propID1 + `"
  titles:
    EN: Color
    UK: Колір
- id: "` + propID2 + `"
  titles:
    EN: Size
`

	dataDir := main.TempDir()
	require.NoError(main, os.WriteFile(
		filepath.Join(dataDir, "properties.yaml"),
		[]byte(propertiesYAML), 0644))

	app := testapp.New(main, dataDir)
	app.Start()

	doRequest := func(t *testing.T) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/properties"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("ReturnsList", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 2)

		ids := []string{body[0]["id"].(string), body[1]["id"].(string)}
		assert.Contains(t, ids, propID1)
		assert.Contains(t, ids, propID2)

		// Find the Color property and assert its titles map
		for _, item := range body {
			if item["id"] == propID1 {
				titles := item["titles"].(map[string]any)
				assert.Equal(t, "Color", titles["EN"])
				assert.Equal(t, "Колір", titles["UK"])
			}
		}
	})

	main.Run("EmptyWhenNoProperties", func(t *testing.T) {
		t.Parallel()

		emptyDir := t.TempDir()
		emptyApp := testapp.New(t, emptyDir)
		emptyApp.Start()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, emptyApp.URL("/properties"), nil)
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
