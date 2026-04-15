package googlesheets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

func newTestClient(srv *httptest.Server, spreadsheetID, sheetName string) *Client {
	return &Client{
		httpClient:    srv.Client(),
		serviceURL:    srv.URL,
		spreadsheetID: spreadsheetID,
		sheetName:     sheetName,
	}
}

func TestClient_Write(main *testing.T) {
	now := time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC)

	testOrder := order.Order{
		DateTime:    now,
		ProductName: "Тестовий товар",
		Attributes:  "Колір: Червоний",
		Price:       1500.00,
		Currency:    "UAH",
		FirstName:   "Іван",
		MiddleName:  "Іванович",
		LastName:    "Іваненко",
		Phone:       "+380501234567",
		Email:       "ivan@example.com",
		City:        "Київ",
		Address:     "Відділення №5",
		Notes:       "Примітка",
	}

	main.Run("AppendsRowToSheet", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/sheet-id/values/Orders!A1:append", r.URL.Path)
			assert.Equal(t, "RAW", r.URL.Query().Get("valueInputOption"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var err error
			capturedBody, err = io.ReadAll(r.Body)
			require.NoError(t, err)

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		c := newTestClient(srv, "sheet-id", "Orders")

		require.NoError(t, c.Write(context.Background(), testOrder))

		var body struct {
			Values [][]any `json:"values"`
		}
		require.NoError(t, json.Unmarshal(capturedBody, &body))
		require.Len(t, body.Values, 1)
		row := body.Values[0]
		require.Len(t, row, 15)
		assert.Equal(t, "2026-04-15", row[2])
		assert.Equal(t, "10:30:00", row[3])
		assert.Equal(t, "Тестовий товар", row[4])
		assert.Equal(t, "Колір: Червоний", row[5])
		assert.Equal(t, "1500.00 UAH", row[6])
		assert.Equal(t, "ivan@example.com", row[7])
		assert.Equal(t, "Іван", row[8])
		assert.Equal(t, "Іванович", row[9])
		assert.Equal(t, "Іваненко", row[10])
		assert.Equal(t, "+380501234567", row[11])
		assert.Equal(t, "Київ", row[12])
		assert.Equal(t, "Відділення №5", row[13])
		assert.Equal(t, "Примітка", row[14])
	})

	main.Run("DefaultsSheetNameToSheet1", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/sheet-id/values/Sheet1!A1:append", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		c := newTestClient(srv, "sheet-id", "")

		require.NoError(t, c.Write(context.Background(), order.Order{}))
	})

	main.Run("ErrorOnNonOKStatus", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := newTestClient(srv, "sheet-id", "Orders")

		assert.Error(t, c.Write(context.Background(), order.Order{}))
	})

	main.Run("ErrorOnNetworkFailure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		srv.Close() // close immediately so the request fails

		c := newTestClient(srv, "sheet-id", "Orders")
		assert.Error(t, c.Write(context.Background(), order.Order{}))
	})
}
