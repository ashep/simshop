//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

func TestGetOrderStatus(main *testing.T) {
	dataDir := makeDataDir(main)

	pubPayload := pubKeyPayload(main)

	var mbCounter atomic.Uint64
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubPayload)
			return
		}
		n := mbCounter.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"invoiceId":"inv-status-%d","pageUrl":"https://pay.example/inv-status-%d"}`, n, n)
	}))
	main.Cleanup(mbServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1
	})
	a.Start()

	getStatus := func(t *testing.T, id string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/orders/"+id), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	postOrder := func(t *testing.T, body []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	main.Run("ExistingOrderReturnsAwaitingPayment", func(t *testing.T) {
		truncateOrders(t, a.DSN())

		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"first_name": "Alice",
			"last_name":  "Smith",
			"phone":      "+1",
			"email":      "alice@example.com",
			"country":    "ua",
			"city":       "Kyiv",
			"address":    "Addr 1",
		}))

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)

		resp := getStatus(t, rows[0].ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var got map[string]string
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "awaiting_payment", got["status"])
	})

	main.Run("UnknownIDReturns404", func(t *testing.T) {
		truncateOrders(t, a.DSN())

		// Valid UUID format but not present in the DB.
		resp := getStatus(t, "018f4e3a-0000-7000-8000-0000000000ff")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var got map[string]string
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "order not found", got["error"])
	})

	main.Run("MalformedUUIDReturns400", func(t *testing.T) {
		// The OpenAPI request validator rejects non-UUID path values with 400.
		resp := getStatus(t, "not-a-uuid")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
