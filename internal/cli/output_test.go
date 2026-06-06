package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOrders(main *testing.T) {
	main.Run("renders header, id, total, uppercased country", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderOrders(&buf, []Order{{
			ID: "o1", Status: "paid", CreatedAt: "2026-06-06T10:00:00Z",
			Country: "us", Email: "a@b.com", Price: 1050, Currency: "usd", ProductID: "p1",
		}})
		require.NoError(t, err)
		out := buf.String()
		for _, h := range []string{"ID", "STATUS", "CREATED", "COUNTRY", "EMAIL", "TOTAL", "PRODUCT"} {
			assert.Contains(t, out, h)
		}
		assert.Contains(t, out, "o1")
		assert.Contains(t, out, "10.50 USD")
		assert.Contains(t, out, "US")
		assert.Contains(t, out, "p1")
	})

	main.Run("renders negative total without malformed fraction", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderOrders(&buf, []Order{{ID: "r1", Status: "refunded", Price: -150, Currency: "usd"}})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "-1.50 USD")
	})
}

func TestRenderOrderDetail(main *testing.T) {
	main.Run("renders key fields and attrs", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderOrderDetail(&buf, &Order{
			ID: "o1", Status: "shipped", FirstName: "Ann", LastName: "Lee",
			Price: 2000, Currency: "eur", Country: "de",
			Attrs: []OrderAttr{{Name: "Size", Value: "L"}},
		})
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "o1")
		assert.Contains(t, out, "shipped")
		assert.Contains(t, out, "Ann Lee")
		assert.Contains(t, out, "Size")
		assert.Contains(t, out, "L")
		assert.Contains(t, out, "DE")
		assert.Contains(t, out, "20.00 EUR")
	})
}

func TestRenderShops(main *testing.T) {
	main.Run("marks default and masks api key", func(t *testing.T) {
		cfg, err := parseConfig([]byte(
			"s1:\n  url: https://1.example\n  api_key: supersecret\n" +
				"s2:\n  url: https://2.example\n  api_key: k2\n  default: true\n"))
		require.NoError(t, err)

		var buf bytes.Buffer
		require.NoError(t, RenderShops(&buf, cfg))
		out := buf.String()
		assert.Contains(t, out, "s1")
		assert.Contains(t, out, "s2")
		assert.Contains(t, out, "(default)")
		assert.NotContains(t, out, "supersecret")
		assert.NotContains(t, out, "k2")
		assert.Contains(t, out, "<hidden>")
	})
}

func TestRenderJSON(main *testing.T) {
	main.Run("marshals indented", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, RenderJSON(&buf, map[string]int{"a": 1}))
		assert.Contains(t, buf.String(), "\"a\": 1")
	})
}
