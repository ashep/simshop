package monobank

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapCurrency(main *testing.T) {
	main.Run("UAH", func(t *testing.T) {
		got, err := MapCurrency("UAH")
		require.NoError(t, err)
		assert.Equal(t, 980, got)
	})

	main.Run("USD", func(t *testing.T) {
		got, err := MapCurrency("USD")
		require.NoError(t, err)
		assert.Equal(t, 840, got)
	})

	main.Run("EUR", func(t *testing.T) {
		got, err := MapCurrency("EUR")
		require.NoError(t, err)
		assert.Equal(t, 978, got)
	})

	main.Run("LowercaseAccepted", func(t *testing.T) {
		got, err := MapCurrency("usd")
		require.NoError(t, err)
		assert.Equal(t, 840, got)
	})

	main.Run("Unknown", func(t *testing.T) {
		_, err := MapCurrency("ZZZ")
		require.Error(t, err)
		var apiErr *APIError
		require.True(t, errors.As(err, &apiErr), "expected *APIError, got %T", err)
		assert.Equal(t, "unsupported_currency", apiErr.ErrCode)
		assert.Equal(t, "ZZZ", apiErr.ErrText)
	})
}
