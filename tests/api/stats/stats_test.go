//go:build functest

package stats_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
)

const testAPIKey = "test-api-key"

const testShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
      currency:
        en: UAH
      phone_code: "+380"
  products:
    - widget
`

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

func TestStats(main *testing.T) {
	dataDir := makeDataDir(main)

	var fail atomic.Bool
	today := time.Now().UTC().Format("2006-01-02")

	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		// The two queries differ by their dimension: date vs requestPath.
		if strings.Contains(req.Query, "dimensions { date }") {
			_, _ = w.Write([]byte(`{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[` +
				`{"dimensions":{"date":"` + today + `"},"sum":{"visits":5}}]}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[` +
			`{"dimensions":{"requestPath":"/product/widget"},"count":4},` +
			`{"dimensions":{"requestPath":"/uk/product/widget/"},"count":3},` +
			`{"dimensions":{"requestPath":"/catalog"},"count":9}]}]}}}`))
	}))
	main.Cleanup(cfServer.Close)

	a := testapp.New(main, dataDir, func(cfg *app.Config) {
		cfg.Server.APIKey = testAPIKey
		cfg.Cloudflare = app.CloudflareConfig{
			APIToken: "cf-token", AccountID: "acc", SiteTag: "site", ServiceURL: cfServer.URL,
		}
	})
	a.Start()

	get := func(t *testing.T, base func(string) string, auth, query string) *http.Response {
		t.Helper()
		target := base("/stats")
		if query != "" {
			target += "?" + query
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		require.NoError(t, err)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	main.Run("NoAPIKey", func(t *testing.T) {
		resp := get(t, a.URL, "", "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	main.Run("InvalidDays", func(t *testing.T) {
		resp := get(t, a.URL, "Bearer "+testAPIKey, "days=5")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	main.Run("Default", func(t *testing.T) {
		resp := get(t, a.URL, "Bearer "+testAPIKey, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var got struct {
			Days []struct {
				Date   string `json:"date"`
				Visits int    `json:"visits"`
			} `json:"days"`
			TopProducts []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Views int    `json:"views"`
			} `json:"top_products"`
		}
		require.NoError(t, json.Unmarshal(body, &got))
		require.Len(t, got.Days, 7)
		assert.Equal(t, today, got.Days[6].Date)
		assert.Equal(t, 5, got.Days[6].Visits)
		assert.Equal(t, 0, got.Days[0].Visits)
		require.Len(t, got.TopProducts, 1)
		assert.Equal(t, "widget", got.TopProducts[0].ID)
		assert.Equal(t, "Widget", got.TopProducts[0].Title)
		assert.Equal(t, 7, got.TopProducts[0].Views)
	})

	// Keep last: the report for 14 days is not cached yet, so the failing stub is hit.
	main.Run("UpstreamFailure", func(t *testing.T) {
		fail.Store(true)
		t.Cleanup(func() { fail.Store(false) })
		resp := get(t, a.URL, "Bearer "+testAPIKey, "days=14")
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"bad gateway"}`, string(body))
	})
}

func TestStatsNotConfigured(main *testing.T) {
	a := testapp.New(main, makeDataDir(main), func(cfg *app.Config) {
		cfg.Server.APIKey = testAPIKey
	})
	a.Start()

	main.Run("NotConfigured", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL("/stats"), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+testAPIKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestStatsConfigValidation(main *testing.T) {
	main.Run("PartialCloudflareConfig", func(t *testing.T) {
		a := testapp.New(t, makeDataDir(t), func(cfg *app.Config) {
			cfg.Cloudflare.APIToken = "cf-token"
		})
		err := a.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflare api_token, account_id and site_tag must be set together")
	})
}
