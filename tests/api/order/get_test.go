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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const testAPIKey = "test-api-key"

func TestListOrders(main *testing.T) {
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
		_, _ = fmt.Fprintf(w, `{"invoiceId":"inv-existing-%d","pageUrl":"https://pay.example/inv-existing-%d"}`, n, n)
	}))
	main.Cleanup(mbServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1
		cfg.Server.APIKey = testAPIKey
	})
	a.Start()

	getOrders := func(t *testing.T, authHeader, query string) *http.Response {
		t.Helper()
		target := a.URL("/orders")
		if query != "" {
			target += "?" + query
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		require.NoError(t, err)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
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

	main.Run("MissingAuthHeaderReturns401", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		resp := getOrders(t, "", "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "missing or invalid authorization header")
	})

	main.Run("WrongKeyReturns401", func(t *testing.T) {
		resp := getOrders(t, "Bearer wrong", "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "invalid api key")
	})

	main.Run("EmptyDBReturnsEmptyArray", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		resp := getOrders(t, "Bearer "+testAPIKey, "")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(body))
	})

	main.Run("PopulatedDBReturnsRecords", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbCounter.Store(0)

		// Seed two orders via the POST endpoint to exercise the writer side too.
		// The second one carries an attribute so we can assert the attrs payload.
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
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"attributes": map[string]string{"display_color": "red"},
			"first_name": "Bob",
			"last_name":  "Brown",
			"phone":      "+2",
			"email":      "bob@example.com",
			"country":    "ua",
			"city":       "Kyiv",
			"address":    "Addr 2",
		}))

		resp := getOrders(t, "Bearer "+testAPIKey, "")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got, 2)

		// Newest first: Bob (the second insert) should be index 0.
		assert.Equal(t, "Bob", got[0]["first_name"])
		assert.Equal(t, "Alice", got[1]["first_name"])

		// Bob has the display_color=red attr and price 5999.
		assert.Equal(t, float64(5999), got[0]["price"])
		bobAttrs, ok := got[0]["attrs"].([]any)
		require.True(t, ok)
		require.Len(t, bobAttrs, 1)
		assert.Equal(t, "Display color", bobAttrs[0].(map[string]any)["name"])
		assert.Equal(t, "Red", bobAttrs[0].(map[string]any)["value"])
		assert.Equal(t, float64(1000), bobAttrs[0].(map[string]any)["price"])

		// Both have two history entries: "new" (initial) + "awaiting_payment" (after Monobank invoice).
		bobHistory, ok := got[0]["history"].([]any)
		require.True(t, ok)
		require.Len(t, bobHistory, 2)
		assert.Equal(t, "new", bobHistory[0].(map[string]any)["status"])
		assert.Equal(t, "awaiting_payment", bobHistory[1].(map[string]any)["status"])

		// Bob has one invoice from Monobank.
		bobInvoices, ok := got[0]["invoices"].([]any)
		require.True(t, ok)
		require.Len(t, bobInvoices, 1)
		assert.Equal(t, "monobank", bobInvoices[0].(map[string]any)["provider"])
		assert.Equal(t, "inv-existing-2", bobInvoices[0].(map[string]any)["id"])
		assert.Equal(t, "https://pay.example/inv-existing-2", bobInvoices[0].(map[string]any)["page_url"])

		// Alice has no attrs and two history entries.
		aliceAttrs, ok := got[1]["attrs"].([]any)
		require.True(t, ok)
		assert.Empty(t, aliceAttrs)
		aliceHistory, ok := got[1]["history"].([]any)
		require.True(t, ok)
		require.Len(t, aliceHistory, 2)
		assert.Equal(t, "new", aliceHistory[0].(map[string]any)["status"])
		assert.Equal(t, "awaiting_payment", aliceHistory[1].(map[string]any)["status"])

		// Alice has one invoice from Monobank.
		aliceInvoices, ok := got[1]["invoices"].([]any)
		require.True(t, ok)
		require.Len(t, aliceInvoices, 1)
		assert.Equal(t, "monobank", aliceInvoices[0].(map[string]any)["provider"])
		assert.Equal(t, "inv-existing-1", aliceInvoices[0].(map[string]any)["id"])
		assert.Equal(t, "https://pay.example/inv-existing-1", aliceInvoices[0].(map[string]any)["page_url"])
	})

	main.Run("StatusFilterSingleValue", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbCounter.Store(0)

		// Seed two orders. Both start as "awaiting_payment" (after Monobank invoice).
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Alice", "last_name": "Smith",
			"phone": "+1", "email": "alice@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 1",
		}))
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Bob", "last_name": "Brown",
			"phone": "+2", "email": "bob@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 2",
		}))

		// Promote one order to "paid" via direct SQL so we can filter by it.
		promoteOneOrderToPaid(t, a.DSN())

		resp := getOrders(t, "Bearer "+testAPIKey, "status=paid")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var got []map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got, 1)
		assert.Equal(t, "paid", got[0]["status"])
	})

	main.Run("StatusFilterCSVMultiValue", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbCounter.Store(0)

		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Alice", "last_name": "Smith",
			"phone": "+1", "email": "alice@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 1",
		}))
		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Bob", "last_name": "Brown",
			"phone": "+2", "email": "bob@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 2",
		}))
		promoteOneOrderToPaid(t, a.DSN())
		// Both rows now: one "paid", one "awaiting_payment".

		resp := getOrders(t, "Bearer "+testAPIKey, "status=paid,awaiting_payment")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var got []map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got, 2)
	})

	main.Run("StatusFilterEmptyValueReturnsAll", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbCounter.Store(0)

		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Alice", "last_name": "Smith",
			"phone": "+1", "email": "alice@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 1",
		}))

		// Omitted ?status= must mean "no filter" → all orders. The handler's
		// parseStatusFilter also no-ops on a literal empty value, but the
		// OpenAPI middleware rejects an explicit `?status=` because [""] fails
		// the enum check. Sending no query at all is the supported "no filter"
		// invocation, so that's what we exercise here.
		resp := getOrders(t, "Bearer "+testAPIKey, "")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var got []map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got, 1)
	})

	main.Run("StatusFilterInvalidValueReturns400", func(t *testing.T) {
		resp := getOrders(t, "Bearer "+testAPIKey, "status=bogus")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("StatusFilterMatchesNothingReturnsEmpty", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbCounter.Store(0)

		postOrder(t, mustJSON(map[string]any{
			"product_id": "widget", "lang": "en",
			"first_name": "Alice", "last_name": "Smith",
			"phone": "+1", "email": "alice@example.com",
			"country": "ua", "city": "Kyiv", "address": "Addr 1",
		}))
		// Order is in awaiting_payment; filter for "delivered" matches nothing.

		resp := getOrders(t, "Bearer "+testAPIKey, "status=delivered")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(body))
	})
}

// TestListOrders_NoAPIKeyConfigured starts a separate testapp with no API key
// and verifies the route is not registered (405).
func TestListOrders_NoAPIKeyConfigured(t *testing.T) {
	dataDir := makeDataDir(t)

	pubPayloadNoKey := pubKeyPayload(t)

	mbServerNoKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubPayloadNoKey)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mbServerNoKey.Close)

	a := testapp.New(t, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServerNoKey.URL
		cfg.RateLimit = -1
		cfg.Server.APIKey = "" // explicit
	})
	a.Start()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/orders"), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	// POST /orders is registered unconditionally. With no API key, GET /orders is not
	// registered, so the stdlib mux returns 405 Method Not Allowed (path exists for POST).
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// promoteOneOrderToPaid bumps exactly one order to status='paid' via raw SQL,
// so a status filter can distinguish it from the others. Picks the most
// recently created order to keep ordering stable for the assertions.
func promoteOneOrderToPaid(t *testing.T, dsn string) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(t.Context(), `
		UPDATE orders
		SET status = 'paid', updated_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT id FROM orders ORDER BY created_at DESC LIMIT 1)`)
	require.NoError(t, err)
}
