//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const paidTemplateEN = `---
subject: Order {{ .OrderShortID }} paid
---
Hi {{ .CustomerName }}, thank you for paying {{ .Total }}.

Order link: <{{ .OrderURL }}>`

// writePaidTemplate seeds the email template tree the loader and startup
// validator expect when Resend is enabled. The `paid` template carries the
// real assertions; the rest exist only to satisfy validateEmailTemplates.
func writePaidTemplate(t *testing.T, dataDir string) {
	t.Helper()
	dir := filepath.Join(dataDir, "emails", "paid")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.md"), []byte(paidTemplateEN), 0644))
	for _, st := range []string{"shipped", "delivered", "refund_requested", "refunded"} {
		sd := filepath.Join(dataDir, "emails", st)
		require.NoError(t, os.MkdirAll(sd, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sd, "en.md"),
			[]byte("---\nsubject: "+st+"\n---\nbody"), 0644))
	}
}

type recordedEmail struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func TestCreateOrder_ResendEmail(main *testing.T) {
	dataDir := makeDataDir(main)
	writePaidTemplate(main, dataDir)

	pubPayload := pubKeyPayload(main)

	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubPayload)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invoiceId":"inv-resend","pageUrl":"https://pay.example/inv-resend"}`))
	}))
	main.Cleanup(mbServer.Close)

	var (
		recvMu sync.Mutex
		recv   []recordedEmail
	)
	resendStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got recordedEmail
		require.NoError(main, json.NewDecoder(r.Body).Decode(&got))
		recvMu.Lock()
		recv = append(recv, got)
		recvMu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"e_1"}`))
	}))
	main.Cleanup(resendStub.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.Resend.APIKey = "test-resend-key"
		cfg.Resend.ServiceURL = resendStub.URL
		cfg.Mail.From = "orders@shop.example"
		cfg.Mail.OrderURL = "https://shop.example/order?id={id}"
		cfg.RateLimit = -1
	})
	a.Start()

	main.Run("PaidWebhookSendsEmail", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		recvMu.Lock()
		recv = nil
		recvMu.Unlock()

		// 1. Place an order.
		body, _ := json.Marshal(map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"first_name": "Jane",
			"last_name":  "Doe",
			"phone":      "+12025550123",
			"email":      "buyer@example.com",
			"country":    "us",
			"city":       "NYC",
			"address":    "1 St",
		})
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// 2. Find the order id.
		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		orderID := rows[0].ID

		// 3. Deliver a `paid` webhook.
		webhook := []byte(`{"invoiceId":"inv-resend","status":"success","reference":"` + orderID + `","modifiedDate":"2026-05-01T10:00:00Z"}`)
		sig := signWebhookBody(t, webhook)
		wreq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/monobank/webhook"), bytes.NewReader(webhook))
		require.NoError(t, err)
		wreq.Header.Set("Content-Type", "application/json")
		wreq.Header.Set("X-Sign", sig)
		wresp, err := http.DefaultClient.Do(wreq)
		require.NoError(t, err)
		defer func() { _ = wresp.Body.Close() }()
		require.Equal(t, http.StatusOK, wresp.StatusCode)

		// 4. Wait for the async notifier to flush.
		require.Eventually(t, func() bool {
			recvMu.Lock()
			defer recvMu.Unlock()
			return len(recv) == 1
		}, 3*time.Second, 50*time.Millisecond, "expected 1 resend email")

		recvMu.Lock()
		require.Len(t, recv, 1, "expected exactly one email; duplicates indicate a notifier double-fire")
		e := recv[0]
		recvMu.Unlock()
		assert.Equal(t, "orders@shop.example", e.From)
		assert.Equal(t, []string{"buyer@example.com"}, e.To)
		assert.Contains(t, e.Subject, "paid")
		assert.Contains(t, e.HTML, "Jane Doe")
		assert.Contains(t, e.HTML, "49.99 USD")
		assert.Contains(t, e.HTML, "https://shop.example/order?id="+orderID)
		assert.Contains(t, e.Text, "thank you for paying 49.99 USD")
	})
}
