package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/loader"
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

price:
  default:
    currency: EUR
    value: 10
`

func makePageFile(t *testing.T, dataDir, content string) {
	t.Helper()
	dir := filepath.Join(dataDir, "pages")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages.yaml"), []byte(content), 0644))
}

func makeProductsFile(t *testing.T, dataDir, content string) {
	t.Helper()
	dir := filepath.Join(dataDir, "products")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "products.yaml"), []byte(content), 0644))
}

func makeShopFile(t *testing.T, dataDir, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "shop.yaml"), []byte(content), 0644))
}

func TestLoad(main *testing.T) {
	main.Run("EmptyDataDir", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "nonexistent")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("MissingProductsSubdir", func(t *testing.T) {
		dataDir := t.TempDir()

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("ProductSubdirWithoutYAMLIsSkipped", func(t *testing.T) {
		dataDir := t.TempDir()
		dir := filepath.Join(dataDir, "products", "content-only")
		require.NoError(t, os.MkdirAll(dir, 0755))
		// no product.yaml — should be silently skipped

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("LoadsProduct", func(t *testing.T) {
		dataDir := t.TempDir()
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
price:
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
price:
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
price:
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
price:
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
price:
  ua:
    currency: UAH
    value: 999
`, nil)

		_, err := loader.Load(dataDir)
		assert.ErrorContains(t, err, "price")
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
price:
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
price:
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
price:
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
		dataDir := t.TempDir()

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Pages)
	})

	main.Run("LoadsPages", func(t *testing.T) {
		dataDir := t.TempDir()
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

	main.Run("MissingProductsYAML_EmptyProductItems", func(t *testing.T) {
		dataDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.ProductItems)
	})

	main.Run("MalformedProductsYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductsFile(t, dataDir, `products: [not a list of maps`)

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})

	main.Run("LoadsProductItems", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductsFile(t, dataDir, `
products:
  - id: widget
    title:
      en: Widget
      uk: Widget
    description:
      en: A test product
      uk: Настільний годинник
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Equal(t, "widget", cat.ProductItems[0].ID)
		assert.Equal(t, "Widget", cat.ProductItems[0].Title["en"])
		assert.Equal(t, "A test product", cat.ProductItems[0].Description["en"])
	})

	main.Run("MissingShopYAML_EmptyShop", func(t *testing.T) {
		dataDir := t.TempDir()

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Shop.Name)
		assert.Empty(t, cat.Shop.Title)
		assert.Empty(t, cat.Shop.Description)
	})

	main.Run("LoadsShop", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, `
shop:
  name:
    en: D5Y Design
    uk: D5Y Design
  title:
    en: Crafted Interior Objects
    uk: Предмети інтер'єру ручної роботи
  description:
    en: Designed and made by hand
    uk: Спроєктовано та виготовлено вручну
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Equal(t, "D5Y Design", cat.Shop.Name["en"])
		assert.Equal(t, "D5Y Design", cat.Shop.Name["uk"])
		assert.Equal(t, "Crafted Interior Objects", cat.Shop.Title["en"])
		assert.Equal(t, "Designed and made by hand", cat.Shop.Description["en"])
	})

	main.Run("MalformedShopYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, `shop: [not a map`)

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})

	main.Run("ProductItemGetsImageFromProduct", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "widget", `
name:
  en: Widget
description:
  en: A test product
price:
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
		makeProductsFile(t, dataDir, `
products:
  - id: widget
    title:
      en: Widget
    description:
      en: A test product
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		require.NotNil(t, cat.ProductItems[0].Image)
		assert.Equal(t, "/images/widget/thumb.png", *cat.ProductItems[0].Image)
	})

	main.Run("ProductItemImageAbsentWhenProductHasNoImages", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "widget", minimalValidYAML, nil)
		makeProductsFile(t, dataDir, `
products:
  - id: widget
    title:
      en: Widget
    description:
      en: A test product
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Nil(t, cat.ProductItems[0].Image)
	})

	main.Run("ProductItemImageAbsentWhenNoMatchingProduct", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductsFile(t, dataDir, `
products:
  - id: ghost
    title:
      en: Ghost
    description:
      en: No product dir
`)

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.ProductItems, 1)
		assert.Nil(t, cat.ProductItems[0].Image)
	})

	main.Run("EmptyShopYAML_EmptyShop", func(t *testing.T) {
		dataDir := t.TempDir()
		makeShopFile(t, dataDir, "")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.NotNil(t, cat.Shop)
		assert.Empty(t, cat.Shop.Name)
		assert.Empty(t, cat.Shop.Title)
		assert.Empty(t, cat.Shop.Description)
	})

	main.Run("LoadsProductWithImages", func(t *testing.T) {
		dataDir := t.TempDir()
		makeProductDir(t, dataDir, "p", `
name:
  en: A Product
description:
  en: A product.
price:
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
}
