package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(main *testing.T) {
	main.Run("SendMessageHappyPath", func(t *testing.T) {
		var gotPath, gotMethod, gotContentType string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			gotContentType = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "@chan", "hello", "MarkdownV2")
		require.NoError(t, err)
		assert.Equal(t, "/bottok/sendMessage", gotPath)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "application/json", gotContentType)

		var payload struct {
			ChatID    string `json:"chat_id"`
			Text      string `json:"text"`
			ParseMode string `json:"parse_mode"`
		}
		require.NoError(t, json.Unmarshal(gotBody, &payload))
		assert.Equal(t, "@chan", payload.ChatID)
		assert.Equal(t, "hello", payload.Text)
		assert.Equal(t, "MarkdownV2", payload.ParseMode)
	})

	main.Run("SendMessageOmitsParseModeWhenEmpty", func(t *testing.T) {
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		require.NoError(t, c.SendMessage(context.Background(), "1", "hi", ""))
		// json:"parse_mode,omitempty" — empty string must not appear in the body.
		assert.NotContains(t, string(gotBody), "parse_mode")
	})

	main.Run("SendMessage400ReturnsAPIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"chat not found"}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		require.Error(t, err)
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusBadRequest, apiErr.HTTPStatus)
		assert.Equal(t, "chat not found", apiErr.Description)
		assert.Equal(t, time.Duration(0), apiErr.RetryAfter)
	})

	main.Run("SendMessage401ReturnsAPIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "bad", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	})

	main.Run("SendMessage429PopulatesRetryAfter", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusTooManyRequests, apiErr.HTTPStatus)
		assert.Equal(t, 7*time.Second, apiErr.RetryAfter)
	})

	main.Run("SendMessage5xxReturnsAPIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusBadGateway, apiErr.HTTPStatus)
	})

	main.Run("SendMessageTimeoutReturnsTransportError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: 50 * time.Millisecond}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		require.Error(t, err)
		var apiErr *APIError
		assert.False(t, errors.As(err, &apiErr), "transport timeouts must not be APIError")
	})

	main.Run("SendMessageBodyCapDoesNotOOM", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			big := strings.Repeat("x", 2<<20) // 2 MB > 1 MB cap
			_, _ = w.Write([]byte(big))
		}))
		t.Cleanup(srv.Close)

		c := &Client{apiKey: "tok", serviceURL: srv.URL, httpClient: &http.Client{Timeout: time.Second}}
		err := c.SendMessage(context.Background(), "1", "hi", "")
		require.Error(t, err)
	})

	main.Run("NewClientDefaultsServiceURL", func(t *testing.T) {
		c := NewClient("tok", "")
		assert.Equal(t, "https://api.telegram.org", c.serviceURL)
		assert.Equal(t, "tok", c.apiKey)
		assert.NotNil(t, c.httpClient)
	})

	main.Run("NewClientHonorsCustomServiceURL", func(t *testing.T) {
		c := NewClient("tok", "https://example.test")
		assert.Equal(t, "https://example.test", c.serviceURL)
	})
}
