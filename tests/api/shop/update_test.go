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

func TestUpdateShop(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	doRequest := func(t *testing.T, id string, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch,
			"http://localhost:9000/shops/"+id,
			bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", admin.APIKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "apishop1", admin.ID, map[string]string{"en": "Original"})

		resp := doRequest(t, "apishop1", `{"names":{"en":"Updated","uk":"Оновлено"}}`)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := sd.GetShop(t, "apishop1")
		assert.Equal(t, "Updated", got.Names["en"])
		assert.Equal(t, "Оновлено", got.Names["uk"])
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "nonexistent", `{"names":{"en":"Test"}}`)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop not found"}`, string(body))
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "apishop2", admin.ID, map[string]string{"en": "Lang Test"})

		resp := doRequest(t, "apishop2", `{"names":{"xx":"Unknown"}}`)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code"}`, string(body))
	})
}
