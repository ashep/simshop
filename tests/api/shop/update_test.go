//go:build functest

package shop_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutShop(main *testing.T) {
	main.Parallel()

	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	doRequest := func(t *testing.T, id string, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPut,
			app.URL("/shops/"+id),
			bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", apiKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("AdminSuccess", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "putshop1", admin.ID, map[string]string{"EN": "Original"}, nil)

		resp := doRequest(t, "putshop1", `{"titles":{"EN":"Updated","UK":"Оновлено"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := sd.GetShop(t, "putshop1")
		assert.Equal(t, map[string]string{"EN": "Updated", "UK": "Оновлено"}, got.Titles)
	})

	main.Run("ReplacesExistingTitles", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "putshop2", admin.ID, map[string]string{"EN": "English", "UK": "Українська"}, nil)

		// Send only EN — UK title must be gone after PUT
		resp := doRequest(t, "putshop2", `{"titles":{"EN":"Only English"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := sd.GetShop(t, "putshop2")
		assert.Equal(t, map[string]string{"EN": "Only English"}, got.Titles)
		assert.Empty(t, got.Descriptions)
	})

	main.Run("OwnerSuccess", func(t *testing.T) {
		t.Parallel()

		owner := sd.CreateUser(t)
		sd.CreateShop(t, "putshop3", owner.ID, map[string]string{"EN": "Owner Shop"}, nil)

		resp := doRequest(t, "putshop3", `{"titles":{"EN":"Owner Updated"}}`, owner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := sd.GetShop(t, "putshop3")
		assert.Equal(t, "Owner Updated", got.Titles["EN"])
	})

	main.Run("WithDescriptions", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "putshop6", admin.ID, map[string]string{"EN": "Shop"}, nil)

		resp := doRequest(t, "putshop6", `{"titles":{"EN":"Shop"},"descriptions":{"EN":"Updated description"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := sd.GetShop(t, "putshop6")
		assert.Equal(t, "Updated description", got.Descriptions["EN"])
	})

	main.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()

		owner := sd.CreateUser(t)
		other := sd.CreateUser(t)
		sd.CreateShop(t, "putshop4", owner.ID, map[string]string{"EN": "Shop"}, nil)

		resp := doRequest(t, "putshop4", `{"titles":{"EN":"Hacked"}}`, other.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "nonexistent", `{"titles":{"EN":"Test"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop not found"}`, string(body))
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "putshop5", admin.ID, map[string]string{"EN": "Lang Test"}, nil)

		resp := doRequest(t, "putshop5", `{"titles":{"xx":"Unknown"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code: xx"}`, string(body))
	})
}
