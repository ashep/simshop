package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookupPromo(main *testing.T) {
	p := &Product{PromoCodes: map[string]int{"SPRING15": 15, "Insta10": 10}}

	main.Run("ExactMatch", func(t *testing.T) {
		code, pct, ok := p.LookupPromo("SPRING15")
		assert.True(t, ok)
		assert.Equal(t, "SPRING15", code)
		assert.Equal(t, 15, pct)
	})

	main.Run("CaseAndWhitespaceInsensitive", func(t *testing.T) {
		code, pct, ok := p.LookupPromo(" spring15 ")
		assert.True(t, ok)
		assert.Equal(t, "SPRING15", code)
		assert.Equal(t, 15, pct)
	})

	main.Run("ReturnsCanonicalSpelling", func(t *testing.T) {
		code, pct, ok := p.LookupPromo("INSTA10")
		assert.True(t, ok)
		assert.Equal(t, "Insta10", code)
		assert.Equal(t, 10, pct)
	})

	main.Run("UnknownOrEmpty", func(t *testing.T) {
		for _, in := range []string{"NOPE", "", "   "} {
			_, _, ok := p.LookupPromo(in)
			assert.False(t, ok, in)
		}
	})

	main.Run("NilCodes", func(t *testing.T) {
		_, _, ok := (&Product{}).LookupPromo("SPRING15")
		assert.False(t, ok)
	})
}

func TestDiscount(main *testing.T) {
	main.Run("TotalRoundsToWholeUnits", func(t *testing.T) {
		assert.Equal(t, 37500, Discount(249900, 15)) // 2124.15 → 2124
		assert.Equal(t, 799, Discount(4999, 15))     // 42.49 → 42
		assert.Equal(t, 599, Discount(5499, 10))     // 49.49 → 49
		assert.Equal(t, 0, Discount(1000, 1))        // 9.90 → 10
	})

	main.Run("HalfRoundsUp", func(t *testing.T) {
		assert.Equal(t, 500, Discount(1100, 50)) // 5.50 → 6
	})
}
