//go:build functest

package product_test

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

func TestCreateProduct(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "prodshop", shopOwner.ID, map[string]string{
		"EN": "Product Shop",
		"UK": "Магазин продуктів",
	}, nil)

	limitShop := sd.CreateShop(main, "limitshop", shopOwner.ID, map[string]string{
		"EN": "Limit Shop",
	}, nil)
	sd.SetShopMaxProducts(main, limitShop.ID, 0)

	doRequest := func(t *testing.T, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL("/products"),
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

	validBody := func() string {
		return `{"shop_id":"` + sh.ID + `","data":{"EN":{"title":"Widget","description":"A fine widget"},"UK":{"title":"Віджет","description":"Гарний віджет"}}}`
	}

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody(), admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.NotNil(t, body["id"])
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody(), shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body, "id")
		assert.NotNil(t, body["id"])
	})

	main.Run("Forbidden_NotOwner", func(t *testing.T) {
		t.Parallel()

		otherUser := sd.CreateUser(t)
		resp := doRequest(t, validBody(), otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, validBody(), "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		body := `{"shop_id":"no-such-shop","data":{"EN":{"title":"Widget","description":"A fine widget"},"UK":{"title":"Віджет","description":"Гарний віджет"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop not found"}`, string(respBody))
	})

	main.Run("MissingContent", func(t *testing.T) {
		t.Parallel()

		// Shop has "EN" and "UK"; request is missing "UK"
		body := `{"shop_id":"` + sh.ID + `","data":{"EN":{"title":"Widget","description":"A fine widget"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"content missing for language: UK"}`, string(respBody))
	})

	main.Run("ShopProductLimitReached", func(t *testing.T) {
		t.Parallel()

		body := `{"shop_id":"` + limitShop.ID + `","data":{"EN":{"title":"Widget","description":"A fine widget"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"shop product limit reached"}`, string(respBody))
	})

	main.Run("WhitespaceOnlyTitle", func(t *testing.T) {
		t.Parallel()

		body := `{"shop_id":"` + sh.ID + `","data":{"EN":{"title":" ","description":"A fine widget"},"UK":{"title":"Віджет","description":"Гарний віджет"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"title is required for the language EN"}`, string(respBody))
	})

	main.Run("WhitespaceOnlyDescription", func(t *testing.T) {
		t.Parallel()

		body := `{"shop_id":"` + sh.ID + `","data":{"EN":{"title":"Widget","description":" "},"UK":{"title":"Віджет","description":"Гарний віджет"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"description is required for the language EN"}`, string(respBody))
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		// "zz" is not in the languages table
		body := `{"shop_id":"` + sh.ID + `","data":{"EN":{"title":"Widget","description":"A fine widget"},"UK":{"title":"Віджет","description":"Гарний віджет"},"zz":{"title":"Zz","description":"Zz desc"}}}`
		resp := doRequest(t, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"invalid language code: zz"}`, string(respBody))
	})
}
