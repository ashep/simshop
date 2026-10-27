package metacapi

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
	main.Run("PostsEventsToPixelEndpoint", func(t *testing.T) {
		var gotPath, gotQuery, gotCT string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
			gotCT = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"events_received":1}`))
		}))
		defer srv.Close()

		c := NewClient("1234567890123456", "tok", "TEST123", srv.URL)
		require.NoError(t, c.Send(t.Context(), []Event{{EventName: "Purchase", EventID: "o-1"}}))

		assert.Equal(t, "/"+graphVersion+"/1234567890123456/events", gotPath)
		assert.Empty(t, gotQuery, "access token must not travel in the URL")
		assert.Equal(t, "application/json", gotCT)
		var body map[string]any
		require.NoError(t, json.Unmarshal(gotBody, &body))
		assert.Equal(t, "tok", body["access_token"])
		assert.Equal(t, "TEST123", body["test_event_code"])
		data := body["data"].([]any)
		require.Len(t, data, 1)
		assert.Equal(t, "Purchase", data[0].(map[string]any)["event_name"])
	})

	main.Run("TestEventCodeOmittedWhenEmpty", func(t *testing.T) {
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"events_received":1}`))
		}))
		defer srv.Close()

		require.NoError(t, NewClient("1", "tok", "", srv.URL).Send(t.Context(), []Event{{EventName: "Purchase"}}))
		assert.NotContains(t, string(gotBody), "test_event_code")
	})

	main.Run("Non2xxReturnsAPIErrorWithMessage", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid parameter","code":100}}`))
		}))
		defer srv.Close()

		err := NewClient("1", "tok", "", srv.URL).Send(t.Context(), []Event{{EventName: "Purchase"}})
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, http.StatusBadRequest, apiErr.HTTPStatus)
		assert.Equal(t, "Invalid parameter", apiErr.Message)
	})

	main.Run("EmptyServiceURLUsesGraph", func(t *testing.T) {
		assert.Equal(t, "https://graph.facebook.com", NewClient("1", "tok", "", "").serviceURL)
	})
}
