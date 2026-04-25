//go:build functest

package nova_poshta_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
      currency:
        en: UAH
      phone_code: "+380"
`

func makeFakeNPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CalledMethod string `json:"calledMethod"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		w.Header().Set("Content-Type", "application/json")
		switch req.CalledMethod {
		case "searchSettlements":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [{
					"TotalCount": "1",
					"Addresses": [
						{"Ref": "city-ref-1", "Present": "м. Київ, Київська обл."}
					]
				}]
			}`))
		case "getWarehouses":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [
					{"Ref": "branch-ref-1", "Description": "Відділення №1 (до 30 кг): вул. Хрещатик, 22"}
				]
			}`))
		case "searchSettlementStreets":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [{
					"TotalCount": 1,
					"Addresses": [
						{"SettlementStreetRef": "street-ref-1", "Present": "вул. Хрещатик"}
					]
				}]
			}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestSearchNovaPoshta(main *testing.T) {
	npSrv := makeFakeNPServer(main)
	main.Cleanup(npSrv.Close)

	dataDir := main.TempDir()
	require.NoError(main, os.WriteFile(filepath.Join(dataDir, "shop.yaml"), []byte(testShopYAML), 0644))
	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.NovaPoshta.ServiceURL = npSrv.URL
		cfg.NovaPoshta.APIKey = "test-key"
	})
	a.Start()

	main.Run("SearchCitiesReturnsResults", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/cities?q=Київ"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, "city-ref-1", body[0]["ref"])
		assert.Equal(t, "м. Київ, Київська обл.", body[0]["name"])
	})

	main.Run("SearchCitiesMissingQReturns400", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/cities"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("SearchBranchesReturnsResults", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/branches?city_ref=city-ref-1&q=Хрещ"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, "branch-ref-1", body[0]["ref"])
		assert.Equal(t, "Відділення №1 (до 30 кг): вул. Хрещатик, 22", body[0]["name"])
	})

	main.Run("SearchBranchesMissingCityRefReturns400", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/branches?q=Хрещ"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("SearchBranchesMissingQReturns400", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/branches?city_ref=city-ref-1"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("SearchStreetsReturnsResults", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/streets?city_ref=city-ref-1&q=Хрещ"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var body []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body, 1)
		assert.Equal(t, "street-ref-1", body[0]["ref"])
		assert.Equal(t, "вул. Хрещатик", body[0]["name"])
	})

	main.Run("SearchStreetsMissingCityRefReturns400", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/streets?q=Хрещ"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("SearchStreetsMissingQReturns400", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/nova-poshta/streets?city_ref=city-ref-1"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
