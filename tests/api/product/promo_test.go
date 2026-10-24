//go:build functest

package product_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	simapp "github.com/ashep/simshop/internal/app"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPromoCode(main *testing.T) {
	dataDir := makeDataDir(main, []string{"widget"}, map[string]string{
		"widget": testProductYAML + "promo_codes:\n  SPRING15: 15\n",
	})
	app := testapp.New(main, dataDir)
	app.Start()

	get := func(t *testing.T, path string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL(path), nil)
		require.NoError(t, err)
		// Avoids a live ipinfo.io call on the product endpoints.
		req.Header.Set("CF-IPCountry", "XX")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, body
	}

	main.Run("Returns200ForValidCode", func(t *testing.T) {
		status, body := get(t, "/products/widget/promo-codes/spring15")
		require.Equal(t, http.StatusOK, status, string(body))
		assert.JSONEq(t, `{"code":"SPRING15","discount_percent":15}`, string(body))
	})

	main.Run("Returns404ForUnknownCode", func(t *testing.T) {
		status, body := get(t, "/products/widget/promo-codes/NOPE")
		require.Equal(t, http.StatusNotFound, status)
		assert.JSONEq(t, `{"error":"promo code not found"}`, string(body))
	})

	main.Run("ProductEndpointsDoNotLeakCodes", func(t *testing.T) {
		for _, path := range []string{"/products/widget/en", "/products"} {
			status, body := get(t, path)
			require.Equal(t, http.StatusOK, status, path)
			assert.NotContains(t, string(body), "SPRING15", path)
			assert.NotContains(t, string(body), "promo_codes", path)
			assert.True(t, json.Valid(body), path)
		}
	})

	main.Run("RateLimited", func(t *testing.T) {
		limitedApp := testapp.New(t, dataDir, func(cfg *simapp.Config) { cfg.RateLimit = 1 })
		limitedApp.Start()

		check := func() int {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
				limitedApp.URL("/products/widget/promo-codes/NOPE"), nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			return resp.StatusCode
		}

		assert.Equal(t, http.StatusNotFound, check())
		assert.Equal(t, http.StatusTooManyRequests, check())
	})
}
