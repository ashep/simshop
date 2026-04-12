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

func TestListShopProducts(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "listprodshop", shopOwner.ID, map[string]string{
		"EN": "List Products Shop",
	}, nil)

	p := sd.CreateProduct(main, sh.ID, map[string]int{"DEFAULT": 300}, map[string]product.DataItem{
		"EN": {Title: "Gadget", Description: "A useful gadget"},
	})

	doRequest := func(t *testing.T, shopID string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/shops/"+shopID+"/products"), nil)
		require.NoError(t, err)
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "nonexistentshop", admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop not found"}`, string(body))
	})

	main.Run("Success_Anonymous", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, sh.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, p.ID, body[0]["id"])
		assert.Contains(t, body[0], "data")
		assert.NotContains(t, body[0], "created_at")
		assert.NotContains(t, body[0], "updated_at")
	})

	main.Run("Success_ValidKey_NotOwner", func(t *testing.T) {
		t.Parallel()

		otherUser := sd.CreateUser(t)
		resp := doRequest(t, sh.ID, otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.NotContains(t, body[0], "created_at")
		assert.NotContains(t, body[0], "updated_at")
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, sh.ID, shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.Contains(t, body[0], "created_at")
		assert.NotNil(t, body[0]["created_at"])
		assert.Contains(t, body[0], "updated_at")
		assert.NotNil(t, body[0]["updated_at"])
	})

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, sh.ID, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.Contains(t, body[0], "created_at")
		assert.NotNil(t, body[0]["created_at"])
		assert.Contains(t, body[0], "updated_at")
		assert.NotNil(t, body[0]["updated_at"])
	})

	main.Run("Success_EmptyShop", func(t *testing.T) {
		t.Parallel()

		emptyOwner := sd.CreateUser(t)
		emptySh := sd.CreateShop(t, "emptyshop4lst", emptyOwner.ID, map[string]string{"EN": "Empty Shop"}, nil)

		resp := doRequest(t, emptySh.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(body))
	})
}
