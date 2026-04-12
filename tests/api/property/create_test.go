//go:build functest

package property_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProperty(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)
	nonAdmin := sd.CreateUser(main)

	doRequest := func(t *testing.T, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL("/properties"),
			bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	validBody := `{"titles":{"EN":"Color","UK":"Колір"}}`

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.NotNil(t, body["id"])
	})

	main.Run("Forbidden_NonAdmin", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody, nonAdmin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		body := `{"titles":{"zz":"Unknown"}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code"}`, string(respBody))
	})
}
