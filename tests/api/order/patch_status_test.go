//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const operatorAPIKey = "test-operator-key"

// seedOrderInStatus inserts a minimal orders row directly via SQL and forces
// it into the given status. Returns the generated order id. Subtests use this
// to start from "paid" (or any post-paid status) without running the full
// POST/webhook dance.
func seedOrderInStatus(t *testing.T, dsn, status, lang string) string {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	var id string
	err = pool.QueryRow(t.Context(), `
        INSERT INTO orders (
            product_id, status, email, price, currency, lang,
            first_name, last_name, country, city, phone, address
        ) VALUES (
            'widget', $1::order_status, 'c@e', 1000, 'UAH', $2,
            'F', 'L', 'ua', 'Kyiv', '+380', 'Some 1'
        ) RETURNING id::text`, status, lang).Scan(&id)
	require.NoError(t, err)
	return id
}

// patchStatus issues PATCH /orders/{id}/status with the given body. Returns
// (status code, parsed JSON body or nil for empty bodies).
func patchStatus(t *testing.T, a *testapp.App, id, apiKey string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch,
		a.URL("/orders/"+id+"/status"), bytes.NewReader(raw))
	require.NoError(t, err)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	rb, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if len(rb) > 0 && bytes.HasPrefix(bytes.TrimSpace(rb), []byte("{")) {
		_ = json.Unmarshal(rb, &parsed)
	}
	return resp.StatusCode, parsed
}

func TestPatchOrderStatus(main *testing.T) {
	dataDir := makeDataDir(main)
	tgSrv, tgCh := newTelegramStub(main)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.RateLimit = -1
		cfg.Server.APIKey = operatorAPIKey
		cfg.Telegram.Token = "stub-token"
		cfg.Telegram.ChatID = "chat-1"
		cfg.Telegram.ServiceURL = tgSrv.URL
	})
	a.Start()
	dsn := a.DSN()

	main.Run("OK_FullForwardPath", func(t *testing.T) {
		truncateOrders(t, dsn)
		drainTGChannel(tgCh)
		id := seedOrderInStatus(t, dsn, "paid", "en")

		// paid -> processing
		code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": "processing"})
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "processing", body["status"])

		// processing -> shipped (with tracking)
		code, body = patchStatus(t, a, id, operatorAPIKey, map[string]any{
			"status": "shipped", "tracking_number": "TRK-XYZ",
		})
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "shipped", body["status"])

		// Verify tracking persists in DB.
		pool, err := pgxpool.New(t.Context(), dsn)
		require.NoError(t, err)
		defer pool.Close()
		var tn *string
		require.NoError(t, pool.QueryRow(t.Context(),
			"SELECT tracking_number FROM orders WHERE id = $1::uuid", id).Scan(&tn))
		require.NotNil(t, tn)
		assert.Equal(t, "TRK-XYZ", *tn)

		// Verify tracking is exposed via GET /orders.
		listReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/orders"), nil)
		require.NoError(t, err)
		listReq.Header.Set("Authorization", "Bearer "+operatorAPIKey)
		listResp, err := http.DefaultClient.Do(listReq)
		require.NoError(t, err)
		defer func() { _ = listResp.Body.Close() }()
		require.Equal(t, http.StatusOK, listResp.StatusCode)
		var orders []map[string]any
		require.NoError(t, json.NewDecoder(listResp.Body).Decode(&orders))
		require.Len(t, orders, 1)
		assert.Equal(t, "TRK-XYZ", orders[0]["tracking_number"])

		// shipped -> delivered
		code, _ = patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": "delivered"})
		assert.Equal(t, http.StatusOK, code)
	})

	main.Run("OK_RefundFromProcessing", func(t *testing.T) {
		truncateOrders(t, dsn)
		drainTGChannel(tgCh)
		id := seedOrderInStatus(t, dsn, "processing", "en")
		code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": "refunded"})
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "refunded", body["status"])
	})

	main.Run("OK_CancelFromAnyState", func(t *testing.T) {
		for _, from := range []string{
			"new", "awaiting_payment", "payment_processing", "payment_hold",
			"paid", "processing", "shipped", "delivered",
			"refund_requested", "returned", "refunded",
		} {
			truncateOrders(t, dsn)
			drainTGChannel(tgCh)
			id := seedOrderInStatus(t, dsn, from, "en")
			code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{
				"status": "cancelled", "note": "operator cancel",
			})
			assert.Equal(t, http.StatusOK, code, "from=%s", from)
			assert.Equal(t, "cancelled", body["status"], "from=%s", from)
		}
	})

	main.Run("OK_RefundReturnPath", func(t *testing.T) {
		truncateOrders(t, dsn)
		drainTGChannel(tgCh)
		id := seedOrderInStatus(t, dsn, "delivered", "en")
		for _, target := range []string{"refund_requested", "returned", "refunded"} {
			code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": target})
			assert.Equal(t, http.StatusOK, code, "target=%s", target)
			assert.Equal(t, target, body["status"], "target=%s", target)
		}
	})

	main.Run("OK_Idempotent", func(t *testing.T) {
		truncateOrders(t, dsn)
		drainTGChannel(tgCh)
		id := seedOrderInStatus(t, dsn, "shipped", "en")

		pool, err := pgxpool.New(t.Context(), dsn)
		require.NoError(t, err)
		defer pool.Close()

		// Force tracking_number on row so the PATCH below is a true same-status no-op.
		_, err = pool.Exec(t.Context(),
			"UPDATE orders SET tracking_number = 'TRK-XYZ' WHERE id = $1::uuid", id)
		require.NoError(t, err)

		countShippedHistory := func() int {
			var n int
			require.NoError(t, pool.QueryRow(t.Context(),
				"SELECT count(*) FROM order_history WHERE order_id = $1::uuid AND status = 'shipped'",
				id).Scan(&n))
			return n
		}
		before := countShippedHistory()

		code, _ := patchStatus(t, a, id, operatorAPIKey, map[string]any{
			"status": "shipped", "tracking_number": "TRK-XYZ",
		})
		assert.Equal(t, http.StatusOK, code)

		after := countShippedHistory()
		assert.Equal(t, before, after,
			"idempotent shipped PATCH must not create a new shipped history row")
	})

	main.Run("Err_NoAuth", func(t *testing.T) {
		truncateOrders(t, dsn)
		id := seedOrderInStatus(t, dsn, "paid", "en")
		code, _ := patchStatus(t, a, id, "", map[string]any{"status": "processing"})
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	main.Run("Err_BadKey", func(t *testing.T) {
		truncateOrders(t, dsn)
		id := seedOrderInStatus(t, dsn, "paid", "en")
		code, _ := patchStatus(t, a, id, "wrong-key", map[string]any{"status": "processing"})
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	main.Run("Err_NotFound", func(t *testing.T) {
		truncateOrders(t, dsn)
		// Well-formed UUID with no row.
		code, _ := patchStatus(t, a, "018f4e3a-0000-7000-8000-000000000099",
			operatorAPIKey, map[string]any{"status": "processing"})
		assert.Equal(t, http.StatusNotFound, code)
	})

	main.Run("Err_TrackingMissingOnShipped", func(t *testing.T) {
		truncateOrders(t, dsn)
		id := seedOrderInStatus(t, dsn, "processing", "en")
		code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": "shipped"})
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "tracking_number required", body["error"])
	})

	main.Run("Err_TrackingOnWrongStatus", func(t *testing.T) {
		truncateOrders(t, dsn)
		id := seedOrderInStatus(t, dsn, "shipped", "en")
		code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{
			"status": "delivered", "tracking_number": "TRK-XYZ",
		})
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "tracking_number only valid for shipped", body["error"])
	})

	main.Run("Err_NotAllowed", func(t *testing.T) {
		truncateOrders(t, dsn)
		id := seedOrderInStatus(t, dsn, "paid", "en")
		code, body := patchStatus(t, a, id, operatorAPIKey, map[string]any{"status": "delivered"})
		assert.Equal(t, http.StatusConflict, code)
		assert.Equal(t, "transition not allowed", body["error"])
	})

	main.Run("TelegramFiredOnShipped", func(t *testing.T) {
		truncateOrders(t, dsn)
		drainTGChannel(tgCh)
		id := seedOrderInStatus(t, dsn, "processing", "en")

		code, _ := patchStatus(t, a, id, operatorAPIKey, map[string]any{
			"status": "shipped", "tracking_number": "TRK-XYZ",
		})
		require.Equal(t, http.StatusOK, code)

		got := waitForTGRequest(t, tgCh, 5*time.Second)
		assert.True(t, strings.Contains(got.Text, "Tracking: `TRK-XYZ`"),
			"telegram message should include tracking line, got: %s", got.Text)
	})
}
