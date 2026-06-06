package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(main *testing.T) {
	main.Run("preserves file order and defaults to first shop", func(t *testing.T) {
		cfg, err := parseConfig([]byte(`
b-shop:
  url: https://b.example
  api_key: bkey
a-shop:
  url: https://a.example
  api_key: akey
`))
		require.NoError(t, err)
		require.Len(t, cfg.Shops, 2)
		assert.Equal(t, "b-shop", cfg.Shops[0].Name)
		assert.Equal(t, "a-shop", cfg.Shops[1].Name)
		assert.Equal(t, "b-shop", cfg.DefaultName())
	})

	main.Run("honors explicit default", func(t *testing.T) {
		cfg, err := parseConfig([]byte(`
s1:
  url: https://1.example
  api_key: k1
s2:
  url: https://2.example
  api_key: k2
  default: true
`))
		require.NoError(t, err)
		assert.Equal(t, "s2", cfg.DefaultName())
	})

	main.Run("rejects two defaults", func(t *testing.T) {
		_, err := parseConfig([]byte(`
s1:
  url: https://1.example
  api_key: k1
  default: true
s2:
  url: https://2.example
  api_key: k2
  default: true
`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most one")
	})

	main.Run("requires api_key", func(t *testing.T) {
		_, err := parseConfig([]byte("s1:\n  url: https://1.example\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api_key")
	})

	main.Run("requires url", func(t *testing.T) {
		_, err := parseConfig([]byte("s1:\n  api_key: k1\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url")
	})

	main.Run("rejects empty config", func(t *testing.T) {
		_, err := parseConfig([]byte(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	main.Run("rejects non-mapping root", func(t *testing.T) {
		_, err := parseConfig([]byte("- foo\n- bar\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mapping")
	})
}

func TestConfigSelect(main *testing.T) {
	cfg, err := parseConfig([]byte(`
s1:
  url: https://1.example
  api_key: k1
s2:
  url: https://2.example
  api_key: k2
  default: true
`))
	require.NoError(main, err)

	main.Run("returns default when name empty", func(t *testing.T) {
		s, err := cfg.Select("")
		require.NoError(t, err)
		assert.Equal(t, "s2", s.Name)
	})
	main.Run("returns named shop", func(t *testing.T) {
		s, err := cfg.Select("s1")
		require.NoError(t, err)
		assert.Equal(t, "https://1.example", s.URL)
		assert.Equal(t, "k1", s.APIKey)
	})
	main.Run("errors on unknown shop", func(t *testing.T) {
		_, err := cfg.Select("nope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")
	})
}
