//go:build functest

package product_test

import (
	"bytes"
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

func TestSetProductPrices(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "setpriceshop", shopOwner.ID, map[string]string{"EN": "Set Price Shop"}, nil)

	doRequest := func(t *testing.T, id string, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPut,
			app.URL("/products/"+id+"/prices"),
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

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		body := `{"prices":{"DEFAULT":1000,"US":999}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		prices := sd.GetProductPrices(t, p.ID)
		assert.Equal(t, map[string]int{"DEFAULT": 1000, "US": 999}, prices)
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		body := `{"prices":{"DEFAULT":800}}`
		resp := doRequest(t, p.ID, body, shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		prices := sd.GetProductPrices(t, p.ID)
		assert.Equal(t, map[string]int{"DEFAULT": 800}, prices)
	})

	main.Run("Success_ClearsAll", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500, "US": 999}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		body := `{"prices":{}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		prices := sd.GetProductPrices(t, p.ID)
		assert.Empty(t, prices)
	})

	main.Run("Forbidden_NonOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		otherUser := sd.CreateUser(t)
		body := `{"prices":{"DEFAULT":100}}`
		resp := doRequest(t, p.ID, body, otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		body := `{"prices":{"DEFAULT":100}}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		body := `{"prices":{"DEFAULT":100}}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(respBody))
	})

	main.Run("InvalidCountry", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		// "ZZ" is not in the countries table
		body := `{"prices":{"DEFAULT":1000,"ZZ":999}}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid country code: ZZ"}`, string(respBody))
	})
}

func TestGetProductPrice(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "getpriceshop", shopOwner.ID, map[string]string{"EN": "Get Price Shop"}, nil)

	doRequest := func(t *testing.T, id string, country string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/products/"+id+"/prices?country="+country), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success_ExactCountry", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500, "US": 999}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		resp := doRequest(t, p.ID, "US")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "US", body["country_id"])
		assert.Equal(t, float64(999), body["value"])
	})

	main.Run("Success_FallbackToDefault", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"DEFAULT": 500, "US": 999}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		resp := doRequest(t, p.ID, "DE")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "DE", body["country_id"])
		assert.Equal(t, float64(500), body["value"])
	})

	main.Run("Success_NoPricesAtAll", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		resp := doRequest(t, p.ID, "US")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "US", body["country_id"])
		assert.Equal(t, float64(0), body["value"])
	})

	main.Run("Success_NoDefaultDefined", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, map[string]int{"US": 999}, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		resp := doRequest(t, p.ID, "DE")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "DE", body["country_id"])
		assert.Equal(t, float64(0), body["value"])
	})

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", "US")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(respBody))
	})
}
