//go:build functest

package order_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

func fetchOrderHistoryFull(t *testing.T, dsn, orderID string) []orderHistoryFull {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()
	rows, err := pool.Query(t.Context(), `
		SELECT status::text, note, payload
		FROM order_history WHERE order_id = $1::uuid ORDER BY created_at`, orderID)
	require.NoError(t, err)
	defer rows.Close()
	var out []orderHistoryFull
	for rows.Next() {
		var r orderHistoryFull
		var payload []byte
		require.NoError(t, rows.Scan(&r.Status, &r.Note, &payload))
		if len(payload) > 0 {
			r.Payload = json.RawMessage(payload)
		}
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

type orderHistoryFull struct {
	Status  string
	Note    *string
	Payload json.RawMessage
}

func TestMonobankWebhook(main *testing.T) {
	dataDir := makeDataDir(main)

	pubPayload := pubKeyPayload(main)
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/merchant/pubkey":
			_, _ = w.Write(pubPayload)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"invoiceId":"inv-1","pageUrl":"https://pay.example/inv-1"}`))
		}
	}))
	main.Cleanup(mbServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1
	})
	a.Start()

	createOrder := func(t *testing.T) string {
		t.Helper()
		body := []byte(`{"product_id":"widget","lang":"en","first_name":"a","last_name":"b","phone":"+1","email":"a@b","country":"ua","city":"c","address":"d"}`)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		pool, err := pgxpool.New(t.Context(), a.DSN())
		require.NoError(t, err)
		defer pool.Close()
		var id string
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT id::text FROM orders ORDER BY created_at DESC LIMIT 1").Scan(&id))
		return id
	}

	postWebhook := func(t *testing.T, body []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/monobank/webhook"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sign", signWebhookBody(t, body))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	main.Run("SuccessTransitionsToPaid", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		resp := postWebhook(t, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		assert.Equal(t, "paid", rows[0].Status)

		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 3) // new, awaiting_payment, paid
		assert.Equal(t, "paid", hist[2].Status)
		require.NotNil(t, hist[2].Note)
		assert.Equal(t, "monobank: success, finalAmount=4999", *hist[2].Note)
		require.NotNil(t, hist[2].Payload)
		assert.JSONEq(t, string(body), string(hist[2].Payload))
	})

	main.Run("FailureTransitionsToCancelledWithReason", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"failure","reference":"` + id + `","errCode":"LIMIT_EXCEEDED","failureReason":"limit","amount":4999,"ccy":840,"finalAmount":0,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		resp := postWebhook(t, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Equal(t, "cancelled", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.NotNil(t, hist[2].Note)
		assert.Equal(t, "monobank: failure (LIMIT_EXCEEDED)", *hist[2].Note)
	})

	main.Run("ReversedTransitionsToRefunded", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		paid := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, paid).StatusCode)
		rev := []byte(`{"invoiceId":"inv-1","status":"reversed","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":0,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, rev).StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Equal(t, "refunded", rows[0].Status)
	})

	main.Run("HoldBeforeSuccess", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		hold := []byte(`{"invoiceId":"inv-1","status":"hold","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, hold).StatusCode)
		ok := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, ok).StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Equal(t, "paid", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 4)
		assert.Equal(t, "payment_hold", hist[2].Status)
		assert.Equal(t, "paid", hist[3].Status)
	})

	main.Run("OutOfOrderDeliveryDoesNotDowngrade", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		ok := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, ok).StatusCode)
		late := []byte(`{"invoiceId":"inv-1","status":"processing","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, late).StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Equal(t, "paid", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 3)
	})

	main.Run("IdempotentReplay", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, body).StatusCode)
		require.Equal(t, http.StatusOK, postWebhook(t, body).StatusCode)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 3)
	})

	main.Run("BadSignatureReturns401", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/monobank/webhook"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-Sign", "AAAA")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Equal(t, "awaiting_payment", rows[0].Status)
	})

	main.Run("UnknownReferenceReturns200NoWrite", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"success","reference":"018f4e3a-0000-7000-8000-0000aaaa0001","amount":1,"ccy":840,"finalAmount":1,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		resp := postWebhook(t, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		rows := fetchOrders(t, a.DSN())
		assert.Empty(t, rows)
	})

	main.Run("WebhookURLPropagatedToCreateInvoice", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		var captured map[string]any
		captureSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/merchant/pubkey":
				_, _ = w.Write(pubPayload)
			case "/api/merchant/invoice/create":
				b, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(b, &captured)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"invoiceId":"inv-cap","pageUrl":"https://pay.example/inv-cap"}`))
			}
		}))
		defer captureSrv.Close()
		captureApp := testapp.New(t, dataDir, func(cfg *app.Config) {
			cfg.Monobank.ServiceURL = captureSrv.URL
			cfg.Monobank.WebhookURL = "https://capture.example/monobank/webhook"
			cfg.RateLimit = -1
		})
		captureApp.Start()
		body := []byte(`{"product_id":"widget","lang":"en","first_name":"a","last_name":"b","phone":"+1","email":"a@b","country":"ua","city":"c","address":"d"}`)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, captureApp.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.NotNil(t, captured)
		assert.Equal(t, "https://capture.example/monobank/webhook", captured["webHookUrl"])
	})
}
