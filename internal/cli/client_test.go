package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientListOrders(main *testing.T) {
	main.Run("sends bearer and parses orders", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/orders", r.URL.Path)
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`[{"id":"o1","status":"paid","price":1050,"currency":"usd"}]`))
		}))
		defer srv.Close()

		orders, err := NewClient(srv.URL, "secret").ListOrders(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, orders, 1)
		assert.Equal(t, "o1", orders[0].ID)
		assert.Equal(t, 1050, orders[0].Price)
	})

	main.Run("passes status filter", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Query().Get("status")
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		_, err := NewClient(srv.URL, "k").ListOrders(context.Background(), []string{"paid", "shipped"})
		require.NoError(t, err)
		assert.Equal(t, "paid,shipped", got)
	})

	main.Run("surfaces error body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
		}))
		defer srv.Close()

		_, err := NewClient(srv.URL, "k").ListOrders(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid api key")
	})
}

func TestClientSetStatus(main *testing.T) {
	main.Run("sends patch with body and omits empty note", func(t *testing.T) {
		var body map[string]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPatch, r.Method)
			assert.Equal(t, "/orders/o1/status", r.URL.Path)
			assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"status":"shipped"}`))
		}))
		defer srv.Close()

		st, err := NewClient(srv.URL, "k").SetStatus(context.Background(), "o1", "shipped", "TRK1", "")
		require.NoError(t, err)
		assert.Equal(t, "shipped", st)
		assert.Equal(t, "shipped", body["status"])
		assert.Equal(t, "TRK1", body["tracking_number"])
		_, hasNote := body["note"]
		assert.False(t, hasNote)
	})

	main.Run("omits empty tracking, sends note", func(t *testing.T) {
		var body map[string]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"status":"processing"}`))
		}))
		defer srv.Close()

		st, err := NewClient(srv.URL, "k").SetStatus(context.Background(), "o1", "processing", "", "a note")
		require.NoError(t, err)
		assert.Equal(t, "processing", st)
		assert.Equal(t, "a note", body["note"])
		_, hasTracking := body["tracking_number"]
		assert.False(t, hasTracking)
	})
}

func TestClientGetOrder(main *testing.T) {
	main.Run("filters list by id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"a"},{"id":"b"}]`))
		}))
		defer srv.Close()

		o, err := NewClient(srv.URL, "k").GetOrder(context.Background(), "b")
		require.NoError(t, err)
		assert.Equal(t, "b", o.ID)
	})

	main.Run("returns not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"a"}]`))
		}))
		defer srv.Close()

		_, err := NewClient(srv.URL, "k").GetOrder(context.Background(), "z")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
