package ganalytics

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSend(main *testing.T) {
	main.Run("PostsPayloadToCollectEndpoint", func(t *testing.T) {
		var gotPath, gotMID, gotSecret, gotCT string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMID = r.URL.Query().Get("measurement_id")
			gotSecret = r.URL.Query().Get("api_secret")
			gotCT = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		c := NewClient("G-TEST123", "s3cret", srv.URL)
		err := c.Send(t.Context(), Payload{
			ClientID: "1.2",
			Events:   []Event{{Name: "purchase", Params: map[string]any{"transaction_id": "o-1"}}},
		})
		require.NoError(t, err)

		assert.Equal(t, "/mp/collect", gotPath)
		assert.Equal(t, "G-TEST123", gotMID)
		assert.Equal(t, "s3cret", gotSecret)
		assert.Equal(t, "application/json", gotCT)
		var body map[string]any
		require.NoError(t, json.Unmarshal(gotBody, &body))
		assert.Equal(t, "1.2", body["client_id"])
		events := body["events"].([]any)
		require.Len(t, events, 1)
		assert.Equal(t, "purchase", events[0].(map[string]any)["name"])
	})

	main.Run("Non2xxReturnsAPIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		err := NewClient("G-TEST123", "s3cret", srv.URL).Send(t.Context(), Payload{ClientID: "1.2"})
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
	})

	main.Run("EmptyServiceURLUsesGoogle", func(t *testing.T) {
		assert.Equal(t, "https://www.google-analytics.com", NewClient("G-X", "s", "").serviceURL)
	})
}
