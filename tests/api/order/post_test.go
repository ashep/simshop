//go:build functest

package order_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const testProductYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attrs:
  display_color:
    en:
      title: Display color
      values:
        red:
          title: Red
    uk:
      title: Колір дисплея
      values:
        red:
          title: Червоний
attr_prices:
  display_color:
    red:
      default: 10.00
`

const testShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
      currency:
        en: UAH
      phone_code: "+380"
    us:
      name:
        en: United States
      currency:
        en: USD
      phone_code: "+1"
`

func encodePubPEM(pub *ecdsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

func makeDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shop.yaml"), []byte(testShopYAML), 0644))
	productDir := filepath.Join(dir, "products", "widget")
	require.NoError(t, os.MkdirAll(productDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(testProductYAML), 0644))
	return dir
}

// truncateOrders wipes orders and order_history so each subtest starts clean.
func truncateOrders(t *testing.T, dsn string) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(t.Context(), "TRUNCATE order_attrs, order_invoices, order_history, orders RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

type orderRow struct {
	ID           string
	ProductID    string
	Status       string
	Email        string
	Price        int
	Currency     string
	FirstName    string
	MiddleName   *string
	LastName     string
	Country      string
	City         string
	Phone        string
	Address      string
	CustomerNote *string
}

type orderAttrRow struct {
	OrderID   string
	AttrName  string
	AttrValue string
	AttrPrice int
}

type orderHistoryRow struct {
	OrderID string
	Status  string
	Note    *string
}

func fetchOrders(t *testing.T, dsn string) []orderRow {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	rows, err := pool.Query(t.Context(), `
		SELECT id::text, product_id, status::text, email, price, currency,
		       first_name, middle_name, last_name,
		       country, city, phone, address,
		       customer_note
		FROM orders
		ORDER BY created_at`)
	require.NoError(t, err)
	defer rows.Close()

	var out []orderRow
	for rows.Next() {
		var r orderRow
		require.NoError(t, rows.Scan(
			&r.ID, &r.ProductID, &r.Status, &r.Email, &r.Price, &r.Currency,
			&r.FirstName, &r.MiddleName, &r.LastName,
			&r.Country, &r.City, &r.Phone, &r.Address,
			&r.CustomerNote,
		))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func fetchOrderAttrs(t *testing.T, dsn, orderID string) []orderAttrRow {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	rows, err := pool.Query(t.Context(), `
		SELECT order_id::text, attr_name, attr_value, attr_price
		FROM order_attrs
		WHERE order_id = $1::uuid
		ORDER BY attr_name`, orderID)
	require.NoError(t, err)
	defer rows.Close()

	var out []orderAttrRow
	for rows.Next() {
		var r orderAttrRow
		require.NoError(t, rows.Scan(&r.OrderID, &r.AttrName, &r.AttrValue, &r.AttrPrice))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func fetchOrderHistory(t *testing.T, dsn, orderID string) []orderHistoryRow {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	rows, err := pool.Query(t.Context(), `
		SELECT order_id::text, status::text, note
		FROM order_history
		WHERE order_id = $1::uuid
		ORDER BY created_at`, orderID)
	require.NoError(t, err)
	defer rows.Close()

	var out []orderHistoryRow
	for rows.Next() {
		var r orderHistoryRow
		require.NoError(t, rows.Scan(&r.OrderID, &r.Status, &r.Note))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestCreateOrder(main *testing.T) {
	dataDir := makeDataDir(main)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(main, err)
	pubPEM, err := encodePubPEM(&priv.PublicKey)
	require.NoError(main, err)
	pubKeyPayload := []byte(`{"key":"` + base64.StdEncoding.EncodeToString(pubPEM) + `"}`)
	_ = priv

	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubKeyPayload)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invoiceId":"inv-existing","pageUrl":"https://pay.example/inv-existing"}`))
	}))
	main.Cleanup(mbServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1 // disable rate limiting so subtests don't hit 429
	})
	a.Start()

	validBody := func(overrides map[string]any) []byte {
		payload := map[string]any{
			"product_id": "widget",
			"lang":       "en",
			"first_name": "Іван",
			"last_name":  "Іваненко",
			"phone":      "+380501234567",
			"email":      "ivan@example.com",
			"country":    "ua",
			"city":       "Київ",
			"address":    "Відділення №5",
		}
		for k, v := range overrides {
			if v == nil {
				delete(payload, k)
			} else {
				payload[k] = v
			}
		}
		b, _ := json.Marshal(payload)
		return b
	}

	do := func(t *testing.T, body []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	main.Run("Returns201AndPersistsRow", func(t *testing.T) {
		truncateOrders(t, a.DSN())

		resp := do(t, validBody(nil))
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"payment_url": "https://pay.example/inv-existing"}`, string(body))

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		r := rows[0]
		assert.Equal(t, "widget", r.ProductID)
		assert.Equal(t, "awaiting_payment", r.Status)
		assert.Equal(t, "ivan@example.com", r.Email)
		assert.Equal(t, 4999, r.Price)
		assert.Equal(t, "USD", r.Currency)
		assert.Equal(t, "Іван", r.FirstName)
		assert.Nil(t, r.MiddleName)
		assert.Equal(t, "Іваненко", r.LastName)
		assert.Equal(t, "ua", r.Country)
		assert.Equal(t, "Київ", r.City)
		assert.Equal(t, "+380501234567", r.Phone)
		assert.Equal(t, "Відділення №5", r.Address)
		assert.Nil(t, r.CustomerNote)

		assert.Empty(t, fetchOrderAttrs(t, a.DSN(), r.ID))

		history := fetchOrderHistory(t, a.DSN(), r.ID)
		require.Len(t, history, 2)
		assert.Equal(t, r.ID, history[0].OrderID)
		assert.Equal(t, "new", history[0].Status)
		assert.Nil(t, history[0].Note)
		assert.Equal(t, r.ID, history[1].OrderID)
		assert.Equal(t, "awaiting_payment", history[1].Status)
		assert.Nil(t, history[1].Note)
	})

	main.Run("WithAttributesCalculatesAddOnPriceAndPersistsAttrRow", func(t *testing.T) {
		truncateOrders(t, a.DSN())

		resp := do(t, validBody(map[string]any{
			"attributes": map[string]string{"display_color": "red"},
		}))
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		r := rows[0]
		assert.Equal(t, 5999, r.Price) // 49.99 + 10.00 → 5999 cents

		attrs := fetchOrderAttrs(t, a.DSN(), r.ID)
		require.Len(t, attrs, 1)
		assert.Equal(t, "Display color", attrs[0].AttrName)
		assert.Equal(t, "Red", attrs[0].AttrValue)
		assert.Equal(t, 1000, attrs[0].AttrPrice) // 10.00 → 1000 cents
	})

	main.Run("MissingProductIDReturns400AndDoesNotInsert", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		resp := do(t, validBody(map[string]any{"product_id": nil}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Empty(t, fetchOrders(t, a.DSN()))
	})

	main.Run("MissingEmailReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"email": nil}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("MissingCountryReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"country": nil}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("UnknownProductReturns404", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"product_id": "no-such-product"}))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("CountryNotInAllowedListReturns400AndDoesNotInsert", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		// "fr" is not in shop.yaml's countries (ua, us)
		resp := do(t, validBody(map[string]any{"country": "fr"}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "invalid country")
		assert.Empty(t, fetchOrders(t, a.DSN()))
	})

	main.Run("UnknownLangReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"lang": "fr"}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestCreateOrder_Monobank(main *testing.T) {
	dataDir := makeDataDir(main)

	priv2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(main, err)
	pubPEM2, err := encodePubPEM(&priv2.PublicKey)
	require.NoError(main, err)
	pubKeyPayload2 := []byte(`{"key":"` + base64.StdEncoding.EncodeToString(pubPEM2) + `"}`)
	_ = priv2

	// Monobank stub: configurable per-subtest via the captured handler var.
	var (
		mbHandler   http.HandlerFunc
		lastRequest *http.Request
		lastBody    []byte
	)
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubKeyPayload2)
			return
		}
		body, _ := io.ReadAll(r.Body)
		lastRequest = r.Clone(context.Background())
		lastBody = body
		// Re-attach the body for the inner handler.
		r.Body = io.NopCloser(bytes.NewReader(body))
		if mbHandler != nil {
			mbHandler(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invoiceId":"inv-default","pageUrl":"https://pay.example/inv-default"}`))
	}))
	main.Cleanup(mbServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer.URL
		cfg.RateLimit = -1
	})
	a.Start()

	const reqBody = `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`

	main.Run("HappyPath", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"invoiceId":"inv-1","pageUrl":"https://pay.example/inv-1"}`))
		}

		resp, err := http.Post(a.URL("/orders"), "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusCreated, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.JSONEq(t, `{"payment_url":"https://pay.example/inv-1"}`, string(body))

		// DB assertions.
		pool, err := pgxpool.New(t.Context(), a.DSN())
		require.NoError(t, err)
		defer pool.Close()

		var status string
		var orderID string
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT id::text, status::text FROM orders`).Scan(&orderID, &status))
		assert.Equal(t, "awaiting_payment", status)

		var invoiceID, pageURL, currency string
		var amount int
		require.NoError(t, pool.QueryRow(t.Context(),
			`SELECT id, page_url, amount, currency FROM order_invoices WHERE order_id = $1`, orderID,
		).Scan(&invoiceID, &pageURL, &amount, &currency))
		assert.Equal(t, "inv-1", invoiceID)
		assert.Equal(t, "https://pay.example/inv-1", pageURL)
		assert.Equal(t, 4999, amount)
		assert.Equal(t, "USD", currency)

		var historyCount int
		require.NoError(t, pool.QueryRow(t.Context(),
			`SELECT COUNT(*) FROM order_history WHERE order_id = $1`, orderID,
		).Scan(&historyCount))
		assert.Equal(t, 2, historyCount)
	})

	main.Run("MonobankReturns500", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}

		resp, err := http.Post(a.URL("/orders"), "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.JSONEq(t, `{"error":"bad gateway"}`, string(body))

		pool, err := pgxpool.New(t.Context(), a.DSN())
		require.NoError(t, err)
		defer pool.Close()

		var status string
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT status::text FROM orders`).Scan(&status))
		assert.Equal(t, "new", status)

		var invCount int
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM order_invoices`).Scan(&invCount))
		assert.Equal(t, 0, invCount)

		var historyCount int
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM order_history`).Scan(&historyCount))
		assert.Equal(t, 1, historyCount)
	})

	main.Run("MonobankReturnsErrCode", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errCode":"limit_exceeded","errText":"too many"}`))
		}

		resp, err := http.Post(a.URL("/orders"), "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.JSONEq(t, `{"error":"bad gateway"}`, string(body))

		pool, err := pgxpool.New(t.Context(), a.DSN())
		require.NoError(t, err)
		defer pool.Close()

		var status string
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT status::text FROM orders`).Scan(&status))
		assert.Equal(t, "new", status)
	})

	main.Run("RequestShape", func(t *testing.T) {
		truncateOrders(t, a.DSN())
		mbHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"invoiceId":"inv-rs","pageUrl":"https://pay.example/inv-rs"}`))
		}

		resp, err := http.Post(a.URL("/orders"), "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Inspect what the handler sent to the Monobank stub.
		require.NotNil(t, lastRequest)
		assert.Equal(t, "test-key", lastRequest.Header.Get("X-Token"))
		var body struct {
			Amount           int    `json:"amount"`
			Ccy              int    `json:"ccy"`
			RedirectURL      string `json:"redirectUrl"`
			MerchantPaymInfo struct {
				Reference   string `json:"reference"`
				Destination string `json:"destination"`
			} `json:"merchantPaymInfo"`
		}
		require.NoError(t, json.Unmarshal(lastBody, &body))
		assert.Equal(t, 4999, body.Amount)
		assert.Equal(t, 840, body.Ccy)
		assert.NotEmpty(t, body.MerchantPaymInfo.Reference)
		assert.Contains(t, body.RedirectURL, "order_id=")
	})
}

func TestCreateOrder_DBFailure(t *testing.T) {
	dataDir := makeDataDir(t)

	priv3, err3 := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err3)
	pubPEM3, err3 := encodePubPEM(&priv3.PublicKey)
	require.NoError(t, err3)
	pubKeyPayload3 := []byte(`{"key":"` + base64.StdEncoding.EncodeToString(pubPEM3) + `"}`)
	_ = priv3

	mbServer3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubKeyPayload3)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invoiceId":"inv-db-fail","pageUrl":"https://pay.example/inv-db-fail"}`))
	}))
	t.Cleanup(mbServer3.Close)

	a := testapp.New(t, dataDir, func(cfg *app.Config) {
		cfg.Monobank.ServiceURL = mbServer3.URL
		cfg.RateLimit = -1
	})
	a.Start()

	truncateOrders(t, a.DSN())

	// Make INSERT fail by temporarily renaming the table. Restore on cleanup so
	// reruns (against a persistent postgres container) start from a clean slate.
	pool, err := pgxpool.New(context.Background(), a.DSN())
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), "ALTER TABLE orders RENAME TO orders_disabled")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "ALTER TABLE orders_disabled RENAME TO orders")
		pool.Close()
	})

	body, _ := json.Marshal(map[string]any{
		"product_id": "widget",
		"lang":       "en",
		"first_name": "A",
		"last_name":  "B",
		"phone":      "1",
		"email":      "a@b.com",
		"country":    "ua",
		"city":       "C",
		"address":    "D",
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL("/orders"), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}
