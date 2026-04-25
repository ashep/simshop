//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const testAPIKey = "test-api-key"

func TestListOrders(main *testing.T) {
	dataDir := makeDataDir(main)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.RateLimit = -1
		cfg.Server.APIKey = testAPIKey
	})
	a.Start()

	getOrders := func(t *testing.T, authHeader string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/orders"), nil)
		require.NoError(t, err)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	postOrder := func(t *testing.T, body []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	main.Run("MissingAuthHeaderReturns401", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		resp := getOrders(t, "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "missing or invalid authorization header")
	})

	main.Run("WrongKeyReturns401", func(t *testing.T) {
		resp := getOrders(t, "Bearer wrong")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "invalid api key")
	})

	main.Run("EmptyDBReturnsEmptyArray", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		resp := getOrders(t, "Bearer "+testAPIKey)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(body))
	})

	main.Run("PopulatedDBReturnsRecords", func(t *testing.T) {
		truncateOrders(t, a.DSN())

		// Seed two orders via the POST endpoint to exercise the writer side too.
		// The second one carries an attribute so we can assert the attrs payload.
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"first_name": "Alice",
			"last_name":  "Smith",
			"phone":      "+1",
			"email":      "alice@example.com",
			"country":    "ua",
			"city":       "Kyiv",
			"address":    "Addr 1",
		}))
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"attributes": map[string]string{"display_color": "red"},
			"first_name": "Bob",
			"last_name":  "Brown",
			"phone":      "+2",
			"email":      "bob@example.com",
			"country":    "ua",
			"city":       "Kyiv",
			"address":    "Addr 2",
		}))

		resp := getOrders(t, "Bearer "+testAPIKey)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got, 2)

		// Newest first: Bob (the second insert) should be index 0.
		assert.Equal(t, "Bob", got[0]["first_name"])
		assert.Equal(t, "Alice", got[1]["first_name"])

		// Bob has the display_color=red attr and price 5999.
		assert.Equal(t, float64(5999), got[0]["price"])
		bobAttrs, ok := got[0]["attrs"].([]any)
		require.True(t, ok)
		require.Len(t, bobAttrs, 1)
		assert.Equal(t, "Display color", bobAttrs[0].(map[string]any)["name"])
		assert.Equal(t, "Red", bobAttrs[0].(map[string]any)["value"])
		assert.Equal(t, float64(1000), bobAttrs[0].(map[string]any)["price"])

		// Both have one history entry of status "new" from the writer's initial insert.
		bobHistory, ok := got[0]["history"].([]any)
		require.True(t, ok)
		require.Len(t, bobHistory, 1)
		assert.Equal(t, "new", bobHistory[0].(map[string]any)["status"])

		// Alice has no attrs and one history entry.
		aliceAttrs, ok := got[1]["attrs"].([]any)
		require.True(t, ok)
		assert.Empty(t, aliceAttrs)
		aliceHistory, ok := got[1]["history"].([]any)
		require.True(t, ok)
		require.Len(t, aliceHistory, 1)
	})
}

// TestListOrders_NoAPIKeyConfigured starts a separate testapp with no API key
// and verifies the route is not registered (405).
func TestListOrders_NoAPIKeyConfigured(t *testing.T) {
	dataDir := makeDataDir(t)
	a := testapp.New(t, dataDir, func(cfg *app.Config) {
		cfg.RateLimit = -1
		cfg.Server.APIKey = "" // explicit
	})
	a.Start()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/orders"), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	// POST /orders is registered unconditionally. With no API key, GET /orders is not
	// registered, so the stdlib mux returns 405 Method Not Allowed (path exists for POST).
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
