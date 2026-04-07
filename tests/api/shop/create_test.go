//go:build functest

package shop_test

import (
	"bytes"
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
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost:9000/shops",
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

		resp := doRequest(t, `{"id":"testshop","names":{"en":"Test Shop"}}`)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	main.Run("DuplicateID", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, `{"id":"dupshop","names":{"en":"Dup Shop"}}`)
		resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		resp = doRequest(t, `{"id":"dupshop","names":{"en":"Dup Shop"}}`)
		resp.Body.Close()
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})
}
