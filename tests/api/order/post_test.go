//go:build functest

package order_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func makeDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	productDir := filepath.Join(dir, "products", "widget")
	require.NoError(t, os.MkdirAll(productDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(testProductYAML), 0644))
	return dir
}

type fakeSheetsServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	rows [][][]any
}

func newFakeSheetsServer(t *testing.T) *fakeSheetsServer {
	t.Helper()
	f := &fakeSheetsServer{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var req struct {
			Values [][]any `json:"values"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.rows = append(f.rows, req.Values)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSheetsServer) capturedRows() [][][]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([][][]any, len(f.rows))
	copy(cp, f.rows)
	return cp
}

func TestCreateOrder(main *testing.T) {
	dataDir := makeDataDir(main)
	sheets := newFakeSheetsServer(main)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.GoogleSheets.ServiceURL = sheets.srv.URL
		cfg.GoogleSheets.SpreadsheetID = "test-sheet-id"
		cfg.GoogleSheets.SheetName = "Orders"
		cfg.RateLimit = -1 // negative value disables rate limiting in tests
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

	main.Run("Returns201AndWritesRowToSheet", func(t *testing.T) {
		initialCount := len(sheets.capturedRows())
		resp := do(t, validBody(nil))
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"payment_url": "https://foo.bar"}`, string(body))

		rows := sheets.capturedRows()
		require.Len(t, rows, initialCount+1)
		row := rows[initialCount][0]
		require.Len(t, row, 15)
		// row[0] is hex order ID, row[1] is status — just check non-empty
		assert.NotEmpty(t, row[0])
		assert.Equal(t, "New", row[1])
		assert.NotEmpty(t, row[2]) // date
		assert.NotEmpty(t, row[3]) // time
		assert.Equal(t, "Widget", row[4])
		assert.Equal(t, "", row[5])             // no attributes
		assert.Equal(t, "49.99 USD", row[6])
		assert.Equal(t, "ivan@example.com", row[7])
		assert.Equal(t, "Іван", row[8])
		assert.Equal(t, "", row[9])             // no middle name
		assert.Equal(t, "Іваненко", row[10])
		assert.Equal(t, "+380501234567", row[11])
		assert.Equal(t, "Київ", row[12])
		assert.Equal(t, "Відділення №5", row[13])
		assert.Equal(t, "", row[14]) // no notes
	})

	main.Run("WithAttributesCalculatesAddOnPrice", func(t *testing.T) {
		initialCount := len(sheets.capturedRows())
		resp := do(t, validBody(map[string]any{
			"attributes": map[string]string{"display_color": "red"},
		}))
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		rows := sheets.capturedRows()
		require.Len(t, rows, initialCount+1)
		row := rows[initialCount][0]
		assert.Equal(t, "Display color: Red", row[5]) // attributes
		assert.Equal(t, "59.99 USD", row[6])          // 49.99 + 10.00
	})

	main.Run("MissingProductIDReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"product_id": nil}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("MissingEmailReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"email": nil}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("UnknownProductReturns404", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"product_id": "no-such-product"}))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	main.Run("UnknownLangReturns400", func(t *testing.T) {
		resp := do(t, validBody(map[string]any{"lang": "fr"}))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

}

func TestCreateOrder_SheetsFailure(t *testing.T) {
	dataDir := makeDataDir(t)

	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(errSrv.Close)

	a := testapp.New(t, dataDir, func(cfg *app.Config) {
		cfg.GoogleSheets.ServiceURL = errSrv.URL
		cfg.GoogleSheets.SpreadsheetID = "test-sheet-id"
		cfg.GoogleSheets.SheetName = "Orders"
	})
	a.Start()

	body, _ := json.Marshal(map[string]any{
		"product_id": "widget",
		"lang":       "en",
		"first_name": "A",
		"last_name":  "B",
		"phone":      "1",
		"email":      "a@b.com",
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
