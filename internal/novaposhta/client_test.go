package novaposhta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		apiKey:     "test-key",
		httpClient: srv.Client(),
		serviceURL: srv.URL,
	}
}

func TestSearchCities(main *testing.T) {
	main.Run("ReturnsParsedCities", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req npRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "test-key", req.APIKey)
			assert.Equal(t, "Address", req.ModelName)
			assert.Equal(t, "searchSettlements", req.CalledMethod)
			assert.Equal(t, "Київ", req.MethodProperties["CityName"])

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [{
					"TotalCount": "1",
					"Addresses": [
						{"Ref": "ref-1", "Present": "м. Київ, Київська обл."}
					]
				}]
			}`))
		}))
		defer srv.Close()

		cities, err := newTestClient(srv).SearchCities(context.Background(), "Київ")
		require.NoError(t, err)
		require.Len(t, cities, 1)
		assert.Equal(t, "ref-1", cities[0].Ref)
		assert.Equal(t, "м. Київ, Київська обл.", cities[0].Name)
	})

	main.Run("ReturnsEmptySliceWhenNoResults", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": true, "data": []}`))
		}))
		defer srv.Close()

		cities, err := newTestClient(srv).SearchCities(context.Background(), "xyz")
		require.NoError(t, err)
		assert.Equal(t, []City{}, cities)
	})

	main.Run("ErrorOnNonOKStatus", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := newTestClient(srv).SearchCities(context.Background(), "Київ")
		assert.Error(t, err)
	})

	main.Run("ErrorOnSuccessFalse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": false, "errors": ["API key is invalid"]}`))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).SearchCities(context.Background(), "Київ")
		assert.Error(t, err)
	})
}

func TestSearchBranches(main *testing.T) {
	main.Run("ReturnsParsedBranches", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req npRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "test-key", req.APIKey)
			assert.Equal(t, "AddressGeneral", req.ModelName)
			assert.Equal(t, "getWarehouses", req.CalledMethod)
			assert.Equal(t, "city-ref-1", req.MethodProperties["CityRef"])
			assert.Equal(t, "Хрещ", req.MethodProperties["FindByString"])

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [
					{"Ref": "branch-ref-1", "Description": "Відділення №1 (до 30 кг): вул. Хрещатик, 22"}
				]
			}`))
		}))
		defer srv.Close()

		branches, err := newTestClient(srv).SearchBranches(context.Background(), "city-ref-1", "Хрещ")
		require.NoError(t, err)
		require.Len(t, branches, 1)
		assert.Equal(t, "branch-ref-1", branches[0].Ref)
		assert.Equal(t, "Відділення №1 (до 30 кг): вул. Хрещатик, 22", branches[0].Name)
	})

	main.Run("ReturnsEmptySliceWhenNoResults", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": true, "data": []}`))
		}))
		defer srv.Close()

		branches, err := newTestClient(srv).SearchBranches(context.Background(), "city-ref-1", "xyz")
		require.NoError(t, err)
		assert.Equal(t, []Branch{}, branches)
	})

	main.Run("ErrorOnNonOKStatus", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		_, err := newTestClient(srv).SearchBranches(context.Background(), "city-ref-1", "Хрещ")
		assert.Error(t, err)
	})

	main.Run("ErrorOnSuccessFalse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": false}`))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).SearchBranches(context.Background(), "city-ref-1", "Хрещ")
		assert.Error(t, err)
	})
}
