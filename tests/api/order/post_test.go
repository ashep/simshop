//go:build functest

package order_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
	_, err = pool.Exec(t.Context(), "TRUNCATE order_attrs, order_history, orders RESTART IDENTITY CASCADE")
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

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
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
		assert.JSONEq(t, `{"payment_url": "https://foo.bar"}`, string(body))

		rows := fetchOrders(t, a.DSN())
		require.Len(t, rows, 1)
		r := rows[0]
		assert.Equal(t, "widget", r.ProductID)
		assert.Equal(t, "new", r.Status)
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
		require.Len(t, history, 1)
		assert.Equal(t, r.ID, history[0].OrderID)
		assert.Equal(t, "new", history[0].Status)
		assert.Nil(t, history[0].Note)
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

func TestCreateOrder_DBFailure(t *testing.T) {
	dataDir := makeDataDir(t)

	a := testapp.New(t, dataDir, func(cfg *app.Config) {
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
