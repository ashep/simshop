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

func TestListShop(main *testing.T) {
	main.Parallel()

	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	doRequest := func(t *testing.T) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/shops"), nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", admin.APIKey)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "listshop1", admin.ID, map[string]string{"en": "List Shop One"})
		sd.CreateShop(t, "listshop2", admin.ID, map[string]string{"en": "List Shop Two", "uk": "Перелік Два"})

		resp := doRequest(t)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var shops []shop.Shop
		require.NoError(t, json.Unmarshal(body, &shops))

		byID := make(map[string]shop.Shop, len(shops))
		for _, s := range shops {
			byID[s.ID] = s
		}

		if s, ok := byID["listshop1"]; assert.True(t, ok, "listshop1 not in response") {
			assert.Equal(t, "List Shop One", s.Names["en"])
		}
		if s, ok := byID["listshop2"]; assert.True(t, ok, "listshop2 not in response") {
			assert.Equal(t, "List Shop Two", s.Names["en"])
			assert.Equal(t, "Перелік Два", s.Names["uk"])
		}
	})
}
