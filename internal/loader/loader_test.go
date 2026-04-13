package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/loader"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(main *testing.T) {
	main.Run("EmptyDataDir", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "nonexistent")
		publicDir := t.TempDir()

		cat, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
		assert.Empty(t, cat.Properties)
		assert.Empty(t, cat.Files)
	})

	main.Run("LoadsProducts", func(t *testing.T) {
		dataDir := t.TempDir()
		publicDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", "018f4e3a-0000-7000-8000-000000000001.yaml"),
			[]byte(`data:
  EN:
    title: Widget
    description: A fine widget
prices:
  DEFAULT: 1000
  US: 1200
`), 0644))

		cat, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", cat.Products[0].ID)
		assert.Equal(t, "Widget", cat.Products[0].Data["EN"].Title)
		assert.Equal(t, 1000, cat.Products[0].Prices["DEFAULT"])
		assert.Equal(t, 1200, cat.Products[0].Prices["US"])
	})

	main.Run("LoadsProperties", func(t *testing.T) {
		dataDir := t.TempDir()
		publicDir := t.TempDir()

		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "properties.yaml"),
			[]byte(`- id: "018f4e3a-0000-7000-8000-000000000001"
  titles:
    EN: Color
    UK: Колір
`), 0644))

		cat, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		require.NoError(t, err)
		require.Len(t, cat.Properties, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", cat.Properties[0].ID)
		assert.Equal(t, "Color", cat.Properties[0].Titles["EN"])
		assert.Equal(t, "Колір", cat.Properties[0].Titles["UK"])
	})

	main.Run("LoadsProductFiles", func(t *testing.T) {
		const productID = "018f4e3a-0000-7000-8000-000000000001"
		dataDir := t.TempDir()
		publicDir := t.TempDir()

		// Product YAML referencing a file
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", productID+".yaml"),
			[]byte(`data:
  EN:
    title: Widget
    description: Desc
files:
  - image.jpg
`), 0644))

		// Binary file on disk — minimal JPEG bytes for MIME sniffing
		fileDir := filepath.Join(publicDir, productID)
		require.NoError(t, os.MkdirAll(fileDir, 0755))
		jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
		require.NoError(t, os.WriteFile(filepath.Join(fileDir, "image.jpg"), jpegBytes, 0644))

		cat, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		require.NoError(t, err)

		files := cat.Files[productID]
		require.Len(t, files, 1)
		assert.Equal(t, "image.jpg", files[0].Name)
		assert.Equal(t, "image/jpeg", files[0].MimeType)
		assert.Equal(t, len(jpegBytes), files[0].SizeBytes)
		assert.Equal(t, "/files/"+productID+"/image.jpg", files[0].Path)
	})

	main.Run("MissingFileSilentlySkipped", func(t *testing.T) {
		const productID = "018f4e3a-0000-7000-8000-000000000002"
		dataDir := t.TempDir()
		publicDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", productID+".yaml"),
			[]byte(`data:
  EN:
    title: Widget
    description: Desc
files:
  - missing.jpg
`), 0644))
		// No binary file created on disk

		cat, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		require.NoError(t, err)
		assert.Empty(t, cat.Files[productID])
	})

	main.Run("MalformedProductYAML", func(t *testing.T) {
		dataDir := t.TempDir()
		publicDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", "018f4e3a-0000-7000-8000-000000000001.yaml"),
			[]byte(`data: [not a map`),
			0644))

		_, err := loader.Load(dataDir, publicDir, zerolog.Nop())
		assert.Error(t, err)
	})
}
