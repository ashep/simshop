package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_SendEmail(main *testing.T) {
	main.Run("Returns_nil_on_2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/emails", r.URL.Path)
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			assert.Equal(t, "from@example.com", got["from"])
			assert.Equal(t, []any{"to@example.com"}, got["to"])
			assert.Equal(t, "Hello", got["subject"])
			assert.Equal(t, "<p>hi</p>", got["html"])
			assert.Equal(t, "hi", got["text"])

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"e_123"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("test-key", srv.URL)
		err := c.SendEmail(context.Background(), Email{
			From: "from@example.com", To: "to@example.com",
			Subject: "Hello", HTML: "<p>hi</p>", Text: "hi",
		})
		assert.NoError(t, err)
	})

	main.Run("Returns_APIError_on_4xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"statusCode":422,"name":"validation_error","message":"invalid to"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("k", srv.URL)
		err := c.SendEmail(context.Background(), Email{
			From: "f", To: "t", Subject: "s", HTML: "h", Text: "x",
		})
		require.Error(t, err)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusUnprocessableEntity, apiErr.HTTPStatus)
		assert.Equal(t, "invalid to", apiErr.Message)
		assert.Equal(t, time.Duration(0), apiErr.RetryAfter)
	})

	main.Run("Returns_APIError_with_RetryAfter_on_429", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"statusCode":429,"name":"rate_limit_exceeded","message":"slow down"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("k", srv.URL)
		err := c.SendEmail(context.Background(), Email{
			From: "f", To: "t", Subject: "s", HTML: "h", Text: "x",
		})
		require.Error(t, err)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.HTTPStatus)
		assert.Equal(t, 5*time.Second, apiErr.RetryAfter)
	})

	main.Run("Returns_APIError_on_5xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"statusCode":500,"name":"internal","message":"boom"}`))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("k", srv.URL)
		err := c.SendEmail(context.Background(), Email{
			From: "f", To: "t", Subject: "s", HTML: "h", Text: "x",
		})
		require.Error(t, err)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	})

	main.Run("Wraps_transport_error_plain", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
		srv.Close() // dial will fail

		c := NewClient("k", srv.URL)
		err := c.SendEmail(context.Background(), Email{From: "f", To: "t", Subject: "s", HTML: "h", Text: "x"})
		require.Error(t, err)
		var apiErr *APIError
		assert.False(t, errors.As(err, &apiErr), "transport errors must NOT be classified as *APIError")
	})

	main.Run("Caps_response_body_at_1MB", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			// Stream more than 1 MB of nonsense; client should not OOM.
			big := bytes.Repeat([]byte{'x'}, 2<<20)
			_, _ = io.Copy(w, bytes.NewReader(big))
		}))
		t.Cleanup(srv.Close)

		c := NewClient("k", srv.URL)
		err := c.SendEmail(context.Background(), Email{From: "f", To: "t", Subject: "s", HTML: "h", Text: "x"})
		require.Error(t, err)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.HTTPStatus)
		// No assertion on Message — it may be empty if the JSON didn't fit.
	})
}
