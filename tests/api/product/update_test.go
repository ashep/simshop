//go:build functest

package product_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateProduct(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "uprodshop", shopOwner.ID, map[string]string{"EN": "Update Shop"}, nil)

	doRequest := func(t *testing.T, id string, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, app.URL("/products/"+id),
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

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 1000}, map[string]product.DataItem{
			"EN": {Title: "Old Title", Description: "Old Desc"},
			"UK": {Title: "Старий заголовок", Description: "Старий опис"},
		})

		body := `{"data":{"EN":{"title":"New Title","description":"New Desc"}}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		content := sd.GetProductData(t, p.ID)
		assert.Equal(t, product.DataItem{Title: "New Title", Description: "New Desc"}, content["EN"])
		assert.Len(t, content, 1) // UK row was deleted (full replace)
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 1000}, map[string]product.DataItem{
			"EN": {Title: "Old Title", Description: "Old Desc"},
		})

		body := `{"data":{"EN":{"title":"Owner Update","description":"Owner Desc"}}}`
		resp := doRequest(t, p.ID, body, shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		content := sd.GetProductData(t, p.ID)
		assert.Equal(t, product.DataItem{Title: "Owner Update", Description: "Owner Desc"}, content["EN"])
	})

	main.Run("Forbidden_NonOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 1000}, map[string]product.DataItem{
			"EN": {Title: "Title", Description: "Desc"},
		})

		otherUser := sd.CreateUser(t)
		body := `{"data":{"EN":{"title":"Hack","description":"Hack"}}}`
		resp := doRequest(t, p.ID, body, otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		body := `{"data":{"EN":{"title":"Hack","description":"Hack"}}}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		body := `{"data":{"EN":{"title":"Title","description":"Desc"}}}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(respBody))
	})

	main.Run("MissingEnTitle", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 1000}, map[string]product.DataItem{
			"EN": {Title: "Title", Description: "Desc"},
		})

		// OpenAPI spec enforces required: [EN] — sending without EN key returns 400
		body := `{"data":{"UK":{"title":"Заголовок","description":"Опис"}}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 1000}, map[string]product.DataItem{
			"EN": {Title: "Title", Description: "Desc"},
		})

		// "zz" is not in the languages table
		body := `{"data":{"EN":{"title":"Title","description":"Desc"},"zz":{"title":"Zz","description":"Zz desc"}}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code: zz"}`, string(respBody))
	})
}
