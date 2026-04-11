//go:build functest

package shop_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ashep/simshop/internal/shop"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetShop(main *testing.T) {
	main.Parallel()

	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	doRequest := func(t *testing.T, id string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/shops/"+id), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	doAdminRequest := func(t *testing.T, id string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/shops/"+id), nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", admin.APIKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "nonexistent")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop not found"}`, string(body))
	})

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "getshop1", admin.ID, map[string]string{"en": "Get Shop", "uk": "Отримати Магазин"})

		resp := doRequest(t, "getshop1")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var sh shop.Shop
		require.NoError(t, json.Unmarshal(body, &sh))

		assert.Equal(t, "getshop1", sh.ID)
		assert.Equal(t, "Get Shop", sh.Names["en"])
		assert.Equal(t, "Отримати Магазин", sh.Names["uk"])
	})

	main.Run("AdminGetsExtraFields", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "getshop2", admin.ID, map[string]string{"en": "Admin Shop"})

		resp := doAdminRequest(t, "getshop2")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(body, &result))

		assert.Equal(t, "getshop2", result["id"])
		assert.Contains(t, result, "owner_id")
		assert.Contains(t, result, "created_at")
		assert.Contains(t, result, "updated_at")
		assert.NotNil(t, result["created_at"], "created_at should be present and non-null")
		assert.NotNil(t, result["updated_at"], "updated_at should be present and non-null")

		names, ok := result["names"].(map[string]any)
		assert.True(t, ok, "names should be a map")
		assert.Equal(t, "Admin Shop", names["en"])
	})

	main.Run("PublicGetsMissingExtraFields", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "getshop3", admin.ID, map[string]string{"en": "Public Shop"})

		resp := doRequest(t, "getshop3")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(body, &result))

		assert.NotContains(t, result, "owner_id")
		assert.NotContains(t, result, "created_at")
		assert.NotContains(t, result, "updated_at")
	})
}
