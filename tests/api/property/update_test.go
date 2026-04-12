//go:build functest

package property_test

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

func TestUpdateProperty(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)
	nonAdmin := sd.CreateUser(main)

	doRequest := func(t *testing.T, id string, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, app.URL("/properties/"+id),
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

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		prop := sd.CreateProperty(t, map[string]string{"EN": "Color", "UK": "Колір"})

		resp := doRequest(t, prop.ID, `{"titles":{"EN":"Colour","DE":"Farbe"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		updated := sd.GetProperty(t, prop.ID)
		assert.Equal(t, map[string]string{"EN": "Colour", "DE": "Farbe"}, updated.Titles)
	})

	main.Run("Forbidden_NonAdmin", func(t *testing.T) {
		t.Parallel()

		prop := sd.CreateProperty(t, map[string]string{"EN": "Size"})

		resp := doRequest(t, prop.ID, `{"titles":{"EN":"Updated"}}`, nonAdmin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		prop := sd.CreateProperty(t, map[string]string{"EN": "Weight"})

		resp := doRequest(t, prop.ID, `{"titles":{"EN":"Updated"}}`, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-0000-0000-000000000000", `{"titles":{"EN":"Updated"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"property not found"}`, string(body))
	})

	main.Run("MissingEnTitle", func(t *testing.T) {
		t.Parallel()

		prop := sd.CreateProperty(t, map[string]string{"EN": "Style"})

		resp := doRequest(t, prop.ID, `{"titles":{"UK":"Стиль"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		prop := sd.CreateProperty(t, map[string]string{"EN": "Material"})

		resp := doRequest(t, prop.ID, `{"titles":{"EN":"Valid","zz":"Unknown"}}`, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code"}`, string(body))
	})
}
