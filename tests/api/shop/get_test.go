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

// makeProduct writes a minimal valid product.yaml in the given category.
func makeProduct(t *testing.T, dataDir, id, category string) {
	t.Helper()
	dir := filepath.Join(dataDir, "products", id)
	require.NoError(t, os.MkdirAll(dir, 0755))
	yaml := "name:\n  en: P\ndescription:\n  en: P.\ncategories:\n  - " + category +
		"\nprices:\n  default:\n    currency: EUR\n    value: 10\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "product.yaml"), []byte(yaml), 0644))
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
    en: Test Shop
    uk: Test Shop
  title:
    en: Handmade Goods
    uk: Предмети інтер'єру ручної роботи
  description:
    en: Designed and made by hand
    uk: Спроєктовано та виготовлено вручну
  categories:
    - id: clocks
      en: Clocks
      uk: Годинники
    - id: organizers
      en: Organizers
      uk: Органайзери
    - id: lamps
      en: Lamps
      uk: Світильники
  products:
    - clock
    - pot
  links:
    - title: Instagram
      icon: instagram
      url: https://example.com/ig
  google-analytics:
    id: G-ABC123
  announcement:
    en: Handmade
    uk: Ручна робота
  announcement_page: sale
  hero:
    title:
      en: Objects made by hand
    text:
      en: Clocks and lamps
    images:
      - h1.jpg
      - file: h2.jpg
        category: clocks
  features:
    - icon: clock
      title:
        en: 2 to 7 days
      text:
        en: Production time
  story:
    title:
      en: Not mass-produced
    image: story.jpg
    page: about
  gallery:
    - g1.jpg
  menu:
    - about
`

func TestGetShop(main *testing.T) {
	dataDir := makeDataDir(main, testShopYAML)
	makeProduct(main, dataDir, "clock", "clocks")
	makeProduct(main, dataDir, "pot", "organizers")
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
		assert.Equal(t, "Test Shop", name["en"])
		assert.Equal(t, "Test Shop", name["uk"])

		title, ok := body["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Handmade Goods", title["en"])
		assert.Equal(t, "Предмети інтер'єру ручної роботи", title["uk"])

		description, ok := body["description"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Designed and made by hand", description["en"])
		assert.Equal(t, "Спроєктовано та виготовлено вручну", description["uk"])

		categories, ok := body["categories"].([]any)
		require.True(t, ok)
		require.Len(t, categories, 2)
		clocks, ok := categories[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "clocks", clocks["id"])
		clocksTitle, ok := clocks["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Clocks", clocksTitle["en"])
		assert.Equal(t, "Годинники", clocksTitle["uk"])
		assert.Len(t, clocksTitle, 2)
		assert.NotContains(t, clocks, "image")
		organizers, ok := categories[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "organizers", organizers["id"])
		// "lamps" has no listed products, so it is not served.

		links, ok := body["links"].([]any)
		require.True(t, ok)
		require.Len(t, links, 1)
		link, ok := links[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Instagram", link["title"])
		assert.Equal(t, "instagram", link["icon"])
		assert.Equal(t, "https://example.com/ig", link["url"])

		ga, ok := body["google_analytics"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "G-ABC123", ga["id"])

		assert.Equal(t, map[string]any{"en": "Handmade", "uk": "Ручна робота"}, body["announcement"])
		assert.Equal(t, "sale", body["announcement_page"])
		assert.Equal(t, map[string]any{
			"title": map[string]any{"en": "Objects made by hand"},
			"text":  map[string]any{"en": "Clocks and lamps"},
			"images": []any{
				map[string]any{"file": "h1.jpg"},
				map[string]any{"file": "h2.jpg", "category": "clocks"},
			},
		}, body["hero"])
		assert.Equal(t, []any{map[string]any{
			"icon":  "clock",
			"title": map[string]any{"en": "2 to 7 days"},
			"text":  map[string]any{"en": "Production time"},
		}}, body["features"])
		assert.Equal(t, map[string]any{
			"title": map[string]any{"en": "Not mass-produced"},
			"image": "story.jpg",
			"page":  "about",
		}, body["story"])
		assert.Equal(t, []any{"g1.jpg"}, body["gallery"])
		assert.Equal(t, []any{"about"}, body["menu"])
	})
}
