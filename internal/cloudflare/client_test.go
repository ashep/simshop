package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(main *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

	serve := func(t *testing.T, h http.HandlerFunc) *Client {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return NewClient("tok", "acc1", "site1", srv.URL)
	}

	main.Run("SendsAuthAndVariables", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/graphql", r.URL.Path)
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			var body struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.NotEmpty(t, body.Query)
			assert.Equal(t, "acc1", body.Variables["accountTag"])
			assert.Equal(t, "site1", body.Variables["siteTag"])
			assert.Equal(t, "2026-10-01T00:00:00Z", body.Variables["from"])
			assert.Equal(t, "2026-10-08T00:00:00Z", body.Variables["to"])
			_, _ = w.Write([]byte(`{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[]}]}}}`))
		})
		_, err := c.DailyVisits(context.Background(), from, to)
		assert.NoError(t, err)
	})

	main.Run("DailyVisits", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[
				{"dimensions":{"date":"2026-10-01"},"sum":{"visits":5}},
				{"dimensions":{"date":"2026-10-02"},"sum":{"visits":7}}]}]}}}`))
		})
		got, err := c.DailyVisits(context.Background(), from, to)
		require.NoError(t, err)
		assert.Equal(t, []DayVisits{{"2026-10-01", 5}, {"2026-10-02", 7}}, got)
	})

	main.Run("PathViews", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.EqualValues(t, 2, body.Variables["limit"])
			_, _ = w.Write([]byte(`{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[
				{"dimensions":{"requestPath":"/b"},"count":9},
				{"dimensions":{"requestPath":"/a"},"count":3}]}]}}}`))
		})
		got, err := c.PathViews(context.Background(), from, to, 2)
		require.NoError(t, err)
		assert.Equal(t, []PathViews{{"/b", 9}, {"/a", 3}}, got)
	})

	main.Run("HTTPError", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.DailyVisits(context.Background(), from, to)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 500, apiErr.Status)
	})

	main.Run("GraphQLErrors", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
		})
		_, err := c.PathViews(context.Background(), from, to, 5)
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, []string{"boom"}, apiErr.Messages)
	})

	main.Run("ContextCancel", func(t *testing.T) {
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) {})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.DailyVisits(ctx, from, to)
		assert.Error(t, err)
	})
}
