package monobank

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		apiKey:     "test-key",
		httpClient: srv.Client(),
		serviceURL: srv.URL,
	}
}

func TestCreateInvoice(main *testing.T) {
	main.Run("HappyPath", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/api/merchant/invoice/create", r.URL.Path)
			assert.Equal(t, "test-key", r.Header.Get("X-Token"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var body struct {
				Amount           int `json:"amount"`
				Ccy              int `json:"ccy"`
				MerchantPaymInfo struct {
					Reference   string `json:"reference"`
					Destination string `json:"destination"`
				} `json:"merchantPaymInfo"`
				RedirectURL string `json:"redirectUrl"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, 12345, body.Amount)
			assert.Equal(t, 980, body.Ccy)
			assert.Equal(t, "order-1", body.MerchantPaymInfo.Reference)
			assert.Equal(t, "Acme, order order-1", body.MerchantPaymInfo.Destination)
			assert.Equal(t, "https://shop.example/thanks?order_id=order-1", body.RedirectURL)

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"invoiceId":"inv-1","pageUrl":"https://pay.example/inv-1"}`))
		}))
		defer srv.Close()

		got, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{
			Amount: 12345,
			Ccy:    980,
			MerchantPaymInfo: MerchantPaymInfo{
				Reference:   "order-1",
				Destination: "Acme, order order-1",
			},
			RedirectURL: "https://shop.example/thanks?order_id=order-1",
		})
		require.NoError(t, err)
		assert.Equal(t, "inv-1", got.InvoiceID)
		assert.Equal(t, "https://pay.example/inv-1", got.PageURL)
	})

	main.Run("ErrorOnNonOKStatus", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`internal server error`))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{Amount: 100, Ccy: 980})
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr), "expected *APIError, got %T", err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
		assert.Contains(t, apiErr.Body, "internal server error")
	})

	main.Run("ErrorOnApplicationError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errCode":"limit_exceeded","errText":"too many invoices"}`))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{Amount: 100, Ccy: 980})
		require.Error(t, err)

		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, "limit_exceeded", apiErr.ErrCode)
		assert.Equal(t, "too many invoices", apiErr.ErrText)
	})

	main.Run("ErrorOnMalformedJSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{Amount: 100, Ccy: 980})
		require.Error(t, err)
	})

	main.Run("BodyCapEnforced", func(t *testing.T) {
		// Server responds with a payload larger than the 1MB cap. The client
		// should treat the truncated body as malformed JSON.
		big := strings.Repeat("x", (1<<20)+512)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(big))
		}))
		defer srv.Close()

		_, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{Amount: 100, Ccy: 980})
		require.Error(t, err)
	})

	main.Run("NetworkError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		// Close the server immediately so the next request fails at the
		// transport layer (Do) rather than reaching the response path.
		srv.Close()

		_, err := newTestClient(srv).CreateInvoice(context.Background(), CreateInvoiceRequest{Amount: 100, Ccy: 980})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "do request:")
	})
}

func TestNewClient(main *testing.T) {
	main.Run("DefaultsServiceURL", func(t *testing.T) {
		c := NewClient("k", "")
		assert.Equal(t, defaultServiceURL, c.serviceURL)
	})

	main.Run("OverridesServiceURL", func(t *testing.T) {
		c := NewClient("k", "https://custom/")
		assert.Equal(t, "https://custom/", c.serviceURL)
	})
}
