//go:build functest

package product_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProduct(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "getprodshop", shopOwner.ID, map[string]string{
		"EN": "Get Product Shop",
	}, nil)

	p := sd.CreateProduct(main, sh.ID, map[string]int{"DEFAULT": 500}, map[string]product.ContentItem{
		"EN": {Title: "Widget", Description: "A fine widget"},
	})

	doRequest := func(t *testing.T, id string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/products/"+id), nil)
		require.NoError(t, err)
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success_Anonymous", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, p.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.NotNil(t, body["id"])
		assert.Contains(t, body, "content")
		assert.NotNil(t, body["content"])
		assert.NotContains(t, body, "created_at")
		assert.NotContains(t, body, "updated_at")
	})

	main.Run("Success_ValidKey_NotOwner", func(t *testing.T) {
		t.Parallel()

		otherUser := sd.CreateUser(t)
		resp := doRequest(t, p.ID, otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.Contains(t, body, "content")
		assert.NotContains(t, body, "created_at")
		assert.NotContains(t, body, "updated_at")
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, p.ID, shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.Contains(t, body, "content")
		assert.Contains(t, body, "created_at")
		assert.NotNil(t, body["created_at"])
		assert.Contains(t, body, "updated_at")
		assert.NotNil(t, body["updated_at"])
	})

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, p.ID, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.Contains(t, body, "content")
		assert.Contains(t, body, "created_at")
		assert.NotNil(t, body["created_at"])
		assert.Contains(t, body, "updated_at")
		assert.NotNil(t, body["updated_at"])
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-0000-0000-000000000000", admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(respBody))
	})
}
