package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(main *testing.T) {
	main.Run("EmptyDataDir", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "nonexistent")

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		assert.Empty(t, cat.Products)
	})

	main.Run("LoadsProducts", func(t *testing.T) {
		dataDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", "018f4e3a-0000-7000-8000-000000000001.yaml"),
			[]byte(`data:
  EN:
    title: Widget
    description: A fine widget
`), 0644))

		cat, err := loader.Load(dataDir)
		require.NoError(t, err)
		require.Len(t, cat.Products, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", cat.Products[0].ID)
		assert.Equal(t, "Widget", cat.Products[0].Data["EN"].Title)
	})

	main.Run("MalformedProductYAML", func(t *testing.T) {
		dataDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "products"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dataDir, "products", "018f4e3a-0000-7000-8000-000000000001.yaml"),
			[]byte(`data: [not a map`),
			0644))

		_, err := loader.Load(dataDir)
		assert.Error(t, err)
	})
}
