//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
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
		SELECT status::text, note
		FROM order_history WHERE order_id = $1::uuid ORDER BY created_at`, orderID)
	require.NoError(t, err)
	defer rows.Close()
	var out []orderHistoryFull
	for rows.Next() {
		var r orderHistoryFull
		require.NoError(t, rows.Scan(&r.Status, &r.Note))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

type orderHistoryFull struct {
	Status string
	Note   *string
}

func fetchInvoiceHistory(t *testing.T, dsn, orderID string) []invoiceHistoryRow {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()
	rows, err := pool.Query(t.Context(), `
		SELECT invoice_id, provider, status::text, note, payload, event_at
		FROM invoice_history WHERE order_id = $1::uuid
		ORDER BY event_at, created_at`, orderID)
	require.NoError(t, err)
	defer rows.Close()
	var out []invoiceHistoryRow
	for rows.Next() {
		var r invoiceHistoryRow
		var payload []byte
		require.NoError(t, rows.Scan(&r.InvoiceID, &r.Provider, &r.Status, &r.Note, &payload, &r.EventAt))
		r.Payload = json.RawMessage(payload)
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

type invoiceHistoryRow struct {
	InvoiceID string
	Provider  string
	Status    string
	Note      *string
	Payload   json.RawMessage
	EventAt   time.Time
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

		ih := fetchInvoiceHistory(t, a.DSN(), id)
		require.Len(t, ih, 1)
		assert.Equal(t, "inv-1", ih[0].InvoiceID)
		assert.Equal(t, "monobank", ih[0].Provider)
		assert.Equal(t, "paid", ih[0].Status)
		require.NotNil(t, ih[0].Note)
		assert.Equal(t, "monobank: success, finalAmount=4999", *ih[0].Note)
		assert.JSONEq(t, string(body), string(ih[0].Payload))
	})

	main.Run("FailureTransitionsToCancelledWithReason", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-1","status":"failure","reference":"` + id + `","errCode":"LIMIT_EXCEEDED","failureReason":"limit","amount":4999,"ccy":840,"finalAmount":0,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		resp := postWebhook(t, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		require.Equal(t, "cancelled", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 3)
		assert.Equal(t, "cancelled", hist[2].Status)
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
		require.Len(t, rows, 1)
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
		require.Len(t, rows, 1)
		require.Equal(t, "paid", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 4)
		assert.Equal(t, "payment_hold", hist[2].Status)
		assert.Equal(t, "paid", hist[3].Status)
	})

	main.Run("RetryAfterFailureSucceeds", func(t *testing.T) {
		// Customer initiates payment (processing@t1), it fails (failure@t2), then
		// they retry on Monobank's side (Monobank reuses the same invoiceId) — a
		// fresh processing@t3 followed by success@t4 must drive the order to paid.
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		t1 := time.Now().UTC().Truncate(time.Second)
		t2 := t1.Add(1 * time.Second)
		t3 := t1.Add(2 * time.Second)
		t4 := t1.Add(3 * time.Second)
		mkBody := func(status string, ts time.Time) []byte {
			return []byte(`{"invoiceId":"inv-1","status":"` + status + `","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + t1.Format(time.RFC3339) + `","modifiedDate":"` + ts.Format(time.RFC3339) + `"}`)
		}
		require.Equal(t, http.StatusOK, postWebhook(t, mkBody("processing", t1)).StatusCode)
		require.Equal(t, http.StatusOK, postWebhook(t, mkBody("failure", t2)).StatusCode)
		require.Equal(t, http.StatusOK, postWebhook(t, mkBody("processing", t3)).StatusCode)
		require.Equal(t, http.StatusOK, postWebhook(t, mkBody("success", t4)).StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		assert.Equal(t, "paid", rows[0].Status)

		ih := fetchInvoiceHistory(t, a.DSN(), id)
		require.Len(t, ih, 4)
		assert.Equal(t, "processing", ih[0].Status)
		assert.Equal(t, "failed", ih[1].Status)
		assert.Equal(t, "processing", ih[2].Status)
		assert.Equal(t, "paid", ih[3].Status)
	})

	main.Run("OutOfOrderDeliveryDoesNotDowngrade", func(t *testing.T) {
		// success@t2 arrives before processing@t1 (delayed network). The order
		// must end up paid: latest event by event_at wins, and the late
		// processing event is recorded but does not downgrade the order.
		truncateOrders(t, a.DSN())
		id := createOrder(t)
		t1 := time.Now().UTC().Truncate(time.Second)
		t2 := t1.Add(1 * time.Second)
		ok := []byte(`{"invoiceId":"inv-1","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + t1.Format(time.RFC3339) + `","modifiedDate":"` + t2.Format(time.RFC3339) + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, ok).StatusCode)
		late := []byte(`{"invoiceId":"inv-1","status":"processing","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + t1.Format(time.RFC3339) + `","modifiedDate":"` + t1.Format(time.RFC3339) + `"}`)
		require.Equal(t, http.StatusOK, postWebhook(t, late).StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		require.Equal(t, "paid", rows[0].Status)
		hist := fetchOrderHistoryFull(t, a.DSN(), id)
		require.Len(t, hist, 3)

		ih := fetchInvoiceHistory(t, a.DSN(), id)
		require.Len(t, ih, 2)
		assert.Equal(t, "processing", ih[0].Status) // older event_at
		assert.Equal(t, "paid", ih[1].Status)       // newer event_at
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
		// Same (invoice_id, provider, status, event_at) tuple → no duplicate row.
		ih := fetchInvoiceHistory(t, a.DSN(), id)
		require.Len(t, ih, 1)
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
		require.Len(t, rows, 1)
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
		var (
			capturedMu sync.Mutex
			captured   map[string]any
		)
		captureSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/merchant/pubkey":
				_, _ = w.Write(pubPayload)
			case "/api/merchant/invoice/create":
				b, _ := io.ReadAll(r.Body)
				capturedMu.Lock()
				_ = json.Unmarshal(b, &captured)
				capturedMu.Unlock()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"invoiceId":"inv-cap","pageUrl":"https://pay.example/inv-cap"}`))
			}
		}))
		defer captureSrv.Close()
		captureApp := testapp.New(t, dataDir, func(cfg *app.Config) {
			cfg.Monobank.ServiceURL = captureSrv.URL
			cfg.Server.PublicURL = "https://capture.example"
			cfg.RateLimit = -1
		})
		captureApp.Start()
		body := []byte(`{"product_id":"widget","lang":"en","first_name":"a","last_name":"b","phone":"+1","email":"a@b","country":"ua","city":"c","address":"d"}`)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, captureApp.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		capturedMu.Lock()
		defer capturedMu.Unlock()
		require.NotNil(t, captured)
		assert.Equal(t, "https://capture.example/monobank/webhook", captured["webHookUrl"])
	})
}

func TestMonobankWebhook_Telegram(main *testing.T) {
	dataDir := makeDataDir(main)

	pubPayload := pubKeyPayload(main)
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/merchant/pubkey":
			_, _ = w.Write(pubPayload)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"invoiceId":"inv-tg-hook","pageUrl":"https://pay.example/inv-tg-hook"}`))
		}
	}))
	main.Cleanup(mbServer.Close)

	tgSrv, tgCh := newTelegramStub(main)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1
		cfg.Telegram = app.TelegramConfig{
			Token:      "test-token",
			ChatID:     "@simshop-test",
			ServiceURL: tgSrv.URL,
		}
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

	main.Run("SuccessWebhookEmitsPaidMessage", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		drainTGChannel(tgCh)

		id := createOrder(t)
		// Drain the two messages emitted by the order placement (new, awaiting_payment).
		_ = waitForTGRequest(t, tgCh, 2*time.Second)
		_ = waitForTGRequest(t, tgCh, 2*time.Second)

		ts := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"invoiceId":"inv-tg-hook","status":"success","reference":"` + id + `","amount":4999,"ccy":840,"finalAmount":4999,"createdDate":"` + ts + `","modifiedDate":"` + ts + `"}`)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/monobank/webhook"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sign", signWebhookBody(t, body))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		got := waitForTGRequest(t, tgCh, 2*time.Second)
		assert.Equal(t, "@simshop-test", got.ChatID)
		assert.Equal(t, "MarkdownV2", got.ParseMode)
		assert.Contains(t, got.Text, "— *paid*")
		assert.Contains(t, got.Text, `monobank: success, finalAmount\=4999`)
		// Slim format on status updates: no full-info fields.
		assert.NotContains(t, got.Text, "Product:")
		assert.NotContains(t, got.Text, "Customer:")
	})
}
