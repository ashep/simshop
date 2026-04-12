//go:build functest

package property_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListProperties(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())

	doRequest := func(t *testing.T) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/properties"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("EmptyList", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.NotNil(t, body)
	})

	main.Run("WithProperties", func(t *testing.T) {
		t.Parallel()

		p1 := sd.CreateProperty(t, map[string]string{"EN": "Color", "UK": "Колір"})
		p2 := sd.CreateProperty(t, map[string]string{"EN": "Size"})

		resp := doRequest(t)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

		ids := make([]string, 0, len(body))
		for _, item := range body {
			ids = append(ids, item["id"].(string))
		}
		assert.Contains(t, ids, p1.ID)
		assert.Contains(t, ids, p2.ID)
	})
}
