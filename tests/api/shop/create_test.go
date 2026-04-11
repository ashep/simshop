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

func TestCreateShop(main *testing.T) {
	main.Parallel()

	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	doRequest := func(t *testing.T, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL("/shops"),
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

		body := `{"id":"testshop","names":{"en":"Test Shop"},"owner_id":"` + admin.ID + `"}`
		resp := doRequest(t, body)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	main.Run("WithDescriptions", func(t *testing.T) {
		t.Parallel()

		body := `{"id":"descshopapi","names":{"en":"Desc Shop"},"descriptions":{"en":"A shop description"},"owner_id":"` + admin.ID + `"}`
		resp := doRequest(t, body)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		got := sd.GetShop(t, "descshopapi")
		assert.Equal(t, "A shop description", got.Descriptions["en"])
	})

	main.Run("DuplicateID", func(t *testing.T) {
		t.Parallel()

		body := `{"id":"dupshop","names":{"en":"Dup Shop"},"owner_id":"` + admin.ID + `"}`

		resp := doRequest(t, body)
		resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		resp = doRequest(t, body)
		resp.Body.Close()
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})

	main.Run("InvalidOwnerID", func(t *testing.T) {
		t.Parallel()

		body := `{"id":"ownershop","names":{"en":"Owner Shop"},"owner_id":"00000000-0000-0000-0000-000000000000"}`
		resp := doRequest(t, body)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid owner id"}`, string(respBody))
	})
}
