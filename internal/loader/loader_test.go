package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/loader"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeProductDir creates a product subdirectory under dataDir/products/<id>/
// with product.yaml containing the given content, and any extra files specified
// as relative path → content pairs.
func makeProductDir(t *testing.T, dataDir, id, yaml string, extraFiles map[string][]byte) {
	t.Helper()
	dir := filepath.Join(dataDir, "products", id)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "product.yaml"), []byte(yaml), 0644))
	for rel, data := range extraFiles {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, data, 0644))
	}
}

const minimalValidYAML = `
name:
  en: Test Product

description:
  en: A test product.

prices:
  default:
    currency: EUR
    value: 10
`

const minimalShopYAML = `
shop:
  countries:
    ua:
      name:
        en: Ukraine
      currency:
        en: UAH
      phone_code: "+380"
`

// setupDataDir creates a temp data directory containing a minimal valid shop.yaml.
// shop.yaml is mandatory at startup, so every loader test that should succeed must
// have one.
func setupDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	makeShopFile(t, dir, minimalShopYAML)
	return dir
}

func makePageFile(t *testing.T, dataDir, content string) {
	t.Helper()
	dir := filepath.Join(dataDir, "pages")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages.yaml"), []byte(content), 0644))
}

// listProducts rewrites shop.yaml as minimalShopYAML plus a `products` listing of the given ids.
func listProducts(t *testing.T, dataDir string, ids ...string) {
	t.Helper()
	content := minimalShopYAML + "  products:\n"
	for _, id := range ids {
		content += "    - " + id + "\n"
	}
	makeShopFile(t, dataDir, content)
}

// categorizedProductYAML is a minimal valid product.yaml in the given category.
func categorizedProductYAML(category string) string {
	return minimalValidYAML + "categories:\n  - " + category + "\n"
}

func makeShopFile(t *testing.T, dataDir, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "shop.yaml"), []byte(content), 0644))
}

func TestLoad(main *testing.T) {
	main.Run("MissingProductsSubdir", func(t *testing.T) {
		dataDir := setupDataDir(t)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("ProductSubdirWithoutYAMLIsSkipped", func(t *testing.T) {
		dataDir := setupDataDir(t)
		dir := filepath.Join(dataDir, "products", "content-only")
		require.NoError(t, os.MkdirAll(dir, 0755))
		// no product.yaml — should be silently skipped

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("LoadsProduct", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, "widget", cat.Products[0].ID)
		assert.Equal(t, "Test Product", cat.Products[0].Name["en"])
		assert.Equal(t, "A test product.", cat.Products[0].Description["en"])
	})

	main.Run("MalformedProductYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "bad", `name: [not a map`, nil)

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})

	main.Run("Validation_MissingName", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "name")
	})

	main.Run("Validation_MissingDescription", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
prices:
  default:
    currency: EUR
    value: 10
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "description")
	})

	main.Run("Validation_DescriptionLanguageMismatch", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
  uk: Продукт
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "description")
	})

	main.Run("Validation_SpecLanguageMismatch", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
  uk: Продукт
description:
  en: A product.
  uk: Продукт.
specs:
  weight:
    en:
      title: Weight
      value: 100g
prices:
  default:
    currency: EUR
    value: 10
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "spec")
	})

	main.Run("Validation_MissingDefaultPrice", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  ua:
    currency: UAH
    value: 999
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "prices")
	})

	main.Run("Validation_AttrLanguageMismatch", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
  uk: Продукт
description:
  en: A product.
  uk: Продукт.
prices:
  default:
    currency: EUR
    value: 10
attrs:
  color:
    en:
      title: Color
      values:
        red:
          title: Red
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "attr")
	})

	main.Run("Validation_AttrNoValues", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
attrs:
  color:
    en:
      title: Color
      values: {}
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "attr")
	})

	main.Run("Validation_ImageNotFound", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
images:
  - preview: thumb.png
    full: full.png
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "image")
	})

	main.Run("MissingPagesYAML_EmptyPages", func(t *testing.T) {
		dataDir := setupDataDir(t)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Pages)
	})

	main.Run("LoadsPages", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makePageFile(t, dataDir, `
pages:
  - id: about
    title:
      en: About
      uk: Про нас
  - id: contacts
    title:
      en: Contacts
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Pages, 2)
		assert.Equal(t, "about", cat.Pages[0].ID)
		assert.Equal(t, "About", cat.Pages[0].Title["en"])
		assert.Equal(t, "Про нас", cat.Pages[0].Title["uk"])
		assert.Equal(t, "contacts", cat.Pages[1].ID)
	})

	main.Run("MalformedPagesYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		makePageFile(t, dataDir, `pages: [not a list of maps`)

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})

	main.Run("NoShopProducts_EmptyProductItems", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Empty(t, cat.ProductItems)
	})

	main.Run("LoadsProductItems", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
  uk: Віджет
summary:
  en: A test product
  uk: Тестовий продукт
description:
  en: A long description.
  uk: Довгий опис.
categories:
  - clocks
  - organizers
prices:
  default:
    currency: EUR
    value: 10
`, nil)
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Equal(t, "widget", cat.ProductItems[0].ID)
		assert.Equal(t, map[string]string{"en": "Widget", "uk": "Віджет"}, cat.ProductItems[0].Title)
		assert.Equal(t, map[string]string{"en": "A test product", "uk": "Тестовий продукт"},
			cat.ProductItems[0].Description)
		assert.Equal(t, []string{"clocks", "organizers"}, cat.ProductItems[0].Categories)
		assert.Equal(t, []string{"clocks", "organizers"}, cat.Products[0].Categories)
	})

	main.Run("ProductItemsFollowShopOrderAndSkipUnlisted", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "alpha", minimalValidYAML, nil)
		makeProductDir(t, dataDir, "beta", minimalValidYAML, nil)
		makeProductDir(t, dataDir, "gamma", minimalValidYAML, nil)
		listProducts(t, dataDir, "gamma", "alpha")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 3)
		require.Len(t, cat.ProductItems, 2)
		assert.Equal(t, "gamma", cat.ProductItems[0].ID)
		assert.Equal(t, "alpha", cat.ProductItems[1].ID)
	})

	main.Run("Validation_UnknownListedProduct", func(t *testing.T) {
		dataDir := setupDataDir(t)
		listProducts(t, dataDir, "ghost")

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, `shop.yaml products: unknown product "ghost"`)
	})

	main.Run("Validation_DuplicateListedProduct", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)
		listProducts(t, dataDir, "widget", "widget")

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, `shop.yaml products: duplicate product "widget"`)
	})

	main.Run("Validation_SummaryLanguageMismatch", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
  uk: Продукт
summary:
  en: Short
description:
  en: A product.
  uk: Продукт.
prices:
  default:
    currency: EUR
    value: 10
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, `summary missing language "uk"`)
	})

	main.Run("MissingShopYAML_Fatal", func(t *testing.T) {
		dataDir := t.TempDir()

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "shop.yaml")
	})

	main.Run("LoadsShop", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "clock", categorizedProductYAML("clocks"), nil)
		makeProductDir(t, dataDir, "pot", categorizedProductYAML("organizers"), nil)
		makeShopFile(t, dataDir, `
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
  links:
    - title: Instagram
      icon: instagram
      url: https://example.com/ig
  google-analytics:
    id: G-ABC123
  cloudflare_analytics:
    token: cf-tok
  meta_pixel:
    id: "1234567890123456"
  announcement:
    en: Handmade
    uk: Ручна робота
  announcement_page: sale
  hero:
    eyebrow:
      en: Studio
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
    text:
      en: Every piece is unique
    image: story.jpg
    page: about
  gallery:
    - g1.jpg
    - g2.jpg
  menu:
    - about
    - contacts
  products:
    - clock
    - pot
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Shop.Countries, 1)
		ua, ok := cat.Shop.Countries["ua"]
		require.True(t, ok)
		assert.Equal(t, "Ukraine", ua.Name["en"])
		assert.Equal(t, "Україна", ua.Name["uk"])
		assert.Equal(t, "UAH", ua.Currency["en"])
		assert.Equal(t, "грн", ua.Currency["uk"])
		assert.Equal(t, "+380", ua.PhoneCode)
		assert.Equal(t, "🇺🇦", ua.Flag)
		assert.Equal(t, "Test Shop", cat.Shop.Name["en"])
		assert.Equal(t, "Test Shop", cat.Shop.Name["uk"])
		assert.Equal(t, "Handmade Goods", cat.Shop.Title["en"])
		assert.Equal(t, "Designed and made by hand", cat.Shop.Description["en"])
		require.Len(t, cat.Shop.Categories, 2)
		assert.Equal(t, "clocks", cat.Shop.Categories[0].ID)
		assert.Equal(t, "Clocks", cat.Shop.Categories[0].Title["en"])
		assert.Equal(t, "Годинники", cat.Shop.Categories[0].Title["uk"])
		assert.Equal(t, map[string]string{"en": "Clocks", "uk": "Годинники"}, cat.Shop.Categories[0].Title)
		assert.Equal(t, "organizers", cat.Shop.Categories[1].ID)
		assert.Equal(t, "Organizers", cat.Shop.Categories[1].Title["en"])
		assert.Equal(t, "Органайзери", cat.Shop.Categories[1].Title["uk"])
		require.Len(t, cat.Shop.Links, 1)
		assert.Equal(t, "Instagram", cat.Shop.Links[0].Title)
		assert.Equal(t, "instagram", cat.Shop.Links[0].Icon)
		assert.Equal(t, "https://example.com/ig", cat.Shop.Links[0].URL)
		require.NotNil(t, cat.Shop.GoogleAnalytics)
		assert.Equal(t, "G-ABC123", cat.Shop.GoogleAnalytics.ID)
		require.NotNil(t, cat.Shop.CloudflareAnalytics)
		assert.Equal(t, "cf-tok", cat.Shop.CloudflareAnalytics.Token)
		require.NotNil(t, cat.Shop.MetaPixel)
		assert.Equal(t, "1234567890123456", cat.Shop.MetaPixel.ID)
		assert.Equal(t, map[string]string{"en": "Handmade", "uk": "Ручна робота"}, cat.Shop.Announcement)
		assert.Equal(t, "sale", cat.Shop.AnnouncementPage)
		require.NotNil(t, cat.Shop.Hero)
		assert.Equal(t, "Studio", cat.Shop.Hero.Eyebrow["en"])
		assert.Equal(t, "Objects made by hand", cat.Shop.Hero.Title["en"])
		assert.Equal(t, "Clocks and lamps", cat.Shop.Hero.Text["en"])
		assert.Equal(t, []*shop.HeroImage{{File: "h1.jpg"}, {File: "h2.jpg", Category: "clocks"}}, cat.Shop.Hero.Images)
		require.Len(t, cat.Shop.Features, 1)
		assert.Equal(t, "clock", cat.Shop.Features[0].Icon)
		assert.Equal(t, "2 to 7 days", cat.Shop.Features[0].Title["en"])
		assert.Equal(t, "Production time", cat.Shop.Features[0].Text["en"])
		require.NotNil(t, cat.Shop.Story)
		assert.Equal(t, "Not mass-produced", cat.Shop.Story.Title["en"])
		assert.Equal(t, "story.jpg", cat.Shop.Story.Image)
		assert.Equal(t, "about", cat.Shop.Story.Page)
		assert.Equal(t, []string{"g1.jpg", "g2.jpg"}, cat.Shop.Gallery)
		assert.Equal(t, []string{"about", "contacts"}, cat.Shop.Menu)
	})

	main.Run("DropsShopCategoriesWithoutListedProducts", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "clock", categorizedProductYAML("clocks"), nil)
		makeProductDir(t, dataDir, "pot", categorizedProductYAML("organizers"), nil)
		makeProductDir(t, dataDir, "hidden-lamp", categorizedProductYAML("lamps"), nil)
		makeShopFile(t, dataDir, minimalShopYAML+`  categories:
    - id: organizers
      en: Organizers
    - id: lamps
      en: Lamps
    - id: empty
      en: Empty
    - id: clocks
      en: Clocks
  products:
    - clock
    - pot
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		ids := make([]string, 0, len(cat.Shop.Categories))
		for _, c := range cat.Shop.Categories {
			ids = append(ids, c.ID)
		}
		// "lamps" only has an unlisted product and "empty" has none; order is preserved.
		assert.Equal(t, []string{"organizers", "clocks"}, ids)
	})

	main.Run("MalformedShopYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, `shop: [not a map`)

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})

	main.Run("Validation_ShopMissingCountries", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, `
shop:
  name:
    en: Test Shop
`)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "countries")
	})

	main.Run("Validation_ShopEmptyCountries", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, `
shop:
  countries: {}
  name:
    en: Test Shop
`)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "countries")
	})

	main.Run("ProductItemGetsImageFromProduct", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: EUR
    value: 10
images:
  - preview: thumb.png
    full: full.png
`, map[string][]byte{
			"images/thumb.png": []byte("fake"),
			"images/full.png":  []byte("fake"),
		})
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		require.NotNil(t, cat.ProductItems[0].Image)
		assert.Equal(t, "/images/widget/thumb.png", *cat.ProductItems[0].Image)
	})

	main.Run("ProductItemAvailableTrueByDefault", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.True(t, cat.ProductItems[0].Available)
	})

	main.Run("ProductItemAvailableFalseWhenSet", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
available: false
prices:
  default:
    currency: EUR
    value: 10
`, nil)
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.False(t, cat.ProductItems[0].Available)
	})

	main.Run("ProductItemImageUsesCover", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: EUR
    value: 10
cover: 02-preview.png
images:
  - preview: 01-preview.png
    full: 01.png
  - preview: 02-preview.png
    full: 02.png
`, map[string][]byte{
			"images/01-preview.png": []byte("fake"),
			"images/01.png":         []byte("fake"),
			"images/02-preview.png": []byte("fake"),
			"images/02.png":         []byte("fake"),
		})
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		require.NotNil(t, cat.ProductItems[0].Image)
		assert.Equal(t, "/images/widget/02-preview.png", *cat.ProductItems[0].Image)
	})

	main.Run("Validation_CoverNotFound", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
cover: missing.png
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "cover file not found")
	})

	main.Run("Validation_CoverIsVideo", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
cover: clip.mp4
images:
  - type: video
    preview: clip.mp4
    full: clip.mp4
`, map[string][]byte{"images/clip.mp4": []byte("fake")})

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "cover must be a still image")
	})

	main.Run("ProductItemGetsPricesFromProduct", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: EUR
    value: 10
  ua:
    currency: UAH
    value: 400
`, nil)
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Equal(t, map[string]product.PriceItem{
			"default": {Currency: "EUR", Value: 10},
			"ua":      {Currency: "UAH", Value: 400},
		}, cat.ProductItems[0].Prices)
	})

	main.Run("ProductItemImageAbsentWhenProductHasNoImages", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Nil(t, cat.ProductItems[0].Image)
		assert.Nil(t, cat.ProductItems[0].Images)
	})

	main.Run("ProductItemImagesAreFirstTwoStills", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: EUR
    value: 10
images:
  - preview: 01-preview.png
    full: 01.png
  - type: video
    preview: clip.mp4
    full: clip.mp4
  - preview: 02-preview.png
    full: 02.png
  - preview: 03-preview.png
    full: 03.png
  - preview: 04-preview.png
    full: 04.png
  - preview: 05-preview.png
    full: 05.png
`, map[string][]byte{
			"images/01-preview.png": []byte("fake"),
			"images/01.png":         []byte("fake"),
			"images/clip.mp4":       []byte("fake"),
			"images/02-preview.png": []byte("fake"),
			"images/02.png":         []byte("fake"),
			"images/03-preview.png": []byte("fake"),
			"images/03.png":         []byte("fake"),
			"images/04-preview.png": []byte("fake"),
			"images/04.png":         []byte("fake"),
			"images/05-preview.png": []byte("fake"),
			"images/05.png":         []byte("fake"),
		})
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Equal(t, []string{
			"/images/widget/01-preview.png",
			"/images/widget/02-preview.png",
		}, cat.ProductItems[0].Images)
	})

	main.Run("ProductItemImagesStartWithCover", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: EUR
    value: 10
cover: 02-preview.png
images:
  - preview: 01-preview.png
    full: 01.png
  - preview: 02-preview.png
    full: 02.png
  - preview: 03-preview.png
    full: 03.png
`, map[string][]byte{
			"images/01-preview.png": []byte("fake"),
			"images/01.png":         []byte("fake"),
			"images/02-preview.png": []byte("fake"),
			"images/02.png":         []byte("fake"),
			"images/03-preview.png": []byte("fake"),
			"images/03.png":         []byte("fake"),
		})
		listProducts(t, dataDir, "widget")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Equal(t, []string{
			"/images/widget/02-preview.png",
			"/images/widget/01-preview.png",
		}, cat.ProductItems[0].Images)
	})

	main.Run("LoadsProductWithAttrColors", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
attr_colors:
  color:
    red: "#D92D20"
`, nil)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, map[string]map[string]string{"color": {"red": "#D92D20"}}, cat.Products[0].AttrColors)
	})

	main.Run("Validation_AttrColorInvalid", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
attr_colors:
  color:
    red: red
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "attr_colors[color][red]")
	})

	promoYAML := func(codes string) string {
		return `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
promo_codes:
` + codes
	}

	main.Run("LoadsPromoCodes", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", promoYAML("  SPRING15: 15\n"), nil)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, map[string]int{"SPRING15": 15}, cat.Products[0].PromoCodes)
	})

	main.Run("Validation_PromoPercentOutOfRange", func(t *testing.T) {
		for _, pct := range []string{"0", "100"} {
			dataDir := setupDataDir(t)
			makeProductDir(t, dataDir, "p", promoYAML("  SPRING15: "+pct+"\n"), nil)

			_, err := loader.Load(dataDir)
			assert.ErrorContains(t, err, "promo_codes[SPRING15]", pct)
		}
	})

	main.Run("Validation_PromoCodeWhitespace", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", promoYAML("  \"SPRING 15\": 15\n"), nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "promo_codes")
	})

	main.Run("Validation_PromoCodeDuplicateCase", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", promoYAML("  Sale: 10\n  SALE: 20\n"), nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "duplicate promo code")
	})

	main.Run("Validation_AttrImageNotFound", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
attr_images:
  color:
    red: missing.png
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "attr_images")
	})

	main.Run("LoadsProductWithAttrImages", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
attr_images:
  color:
    red: thumb.png
`, map[string][]byte{
			"images/thumb.png": []byte("fake png"),
		})

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, "thumb.png", cat.Products[0].AttrImages["color"]["red"])
	})

	main.Run("EmptyShopYAML_Fatal", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, "")

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "countries")
	})

	main.Run("LoadsProductWithImages", func(t *testing.T) {
		dataDir := setupDataDir(t)
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
prices:
  default:
    currency: EUR
    value: 10
images:
  - preview: thumb.png
    full: full.png
`, map[string][]byte{
			"images/thumb.png": []byte("fake png"),
			"images/full.png":  []byte("fake png"),
		})

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, "/images/p/thumb.png", cat.Products[0].Images[0].Preview)
		assert.Equal(t, "/images/p/full.png", cat.Products[0].Images[0].Full)
	})

	main.Run("LoadsEmailTemplates", func(t *testing.T) {
		dir := setupDataDir(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "emails", "paid"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "emails", "paid", "en.md"),
			[]byte("---\nsubject: Paid\n---\nBody"), 0644))

		c, err := loader.Load(dir)
		require.NoError(t, err)
		require.NotNil(t, c.EmailTemplates)
		assert.NotNil(t, c.EmailTemplates.All()["paid"]["en"])
	})

	main.Run("MissingEmailsDirIsNotAnError", func(t *testing.T) {
		dir := setupDataDir(t)
		c, err := loader.Load(dir)
		require.NoError(t, err)
		require.NotNil(t, c.EmailTemplates)
		assert.Empty(t, c.EmailTemplates.All())
	})

	main.Run("MalformedEmailTemplateIsFatal", func(t *testing.T) {
		dir := setupDataDir(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "emails", "paid"), 0755))
		// Missing subject — should be fatal.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "emails", "paid", "en.md"),
			[]byte("---\nfoo: bar\n---\nbody"), 0644))

		_, err := loader.Load(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subject")
	})
}
