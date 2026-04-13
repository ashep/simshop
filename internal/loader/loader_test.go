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
          add_price: 0
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
