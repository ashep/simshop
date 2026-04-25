//go:build functest

package shop_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeDataDir(t *testing.T, shopYAML string) string {
	t.Helper()
	dataDir := t.TempDir()
	if shopYAML != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dataDir, "shop.yaml"), []byte(shopYAML), 0644))
	}
	return dataDir
}

const testShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
        uk: Україна
      currency:
        en: UAH
        uk: грн
      phone_code: "+380"
      flag: "🇺🇦"
  name:
    en: D5Y Design
    uk: D5Y Design
  title:
    en: Crafted Interior Objects
    uk: Предмети інтер'єру ручної роботи
  description:
    en: Designed and made by hand
    uk: Спроєктовано та виготовлено вручну
`

func TestGetShop(main *testing.T) {
	dataDir := makeDataDir(main, testShopYAML)
	app := testapp.New(main, dataDir)
	app.Start()

	main.Run("ReturnsShopData", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL("/shop"), nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

		countries, ok := body["countries"].(map[string]any)
		require.True(t, ok)
		ua, ok := countries["ua"].(map[string]any)
		require.True(t, ok)
		uaName, ok := ua["name"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Ukraine", uaName["en"])
		assert.Equal(t, "Україна", uaName["uk"])
		uaCurrency, ok := ua["currency"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "UAH", uaCurrency["en"])
		assert.Equal(t, "грн", uaCurrency["uk"])
		assert.Equal(t, "+380", ua["phone_code"])
		assert.Equal(t, "🇺🇦", ua["flag"])

		name, ok := body["name"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "D5Y Design", name["en"])
		assert.Equal(t, "D5Y Design", name["uk"])

		title, ok := body["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Crafted Interior Objects", title["en"])
		assert.Equal(t, "Предмети інтер'єру ручної роботи", title["uk"])

		description, ok := body["description"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Designed and made by hand", description["en"])
		assert.Equal(t, "Спроєктовано та виготовлено вручну", description["uk"])
	})
}
