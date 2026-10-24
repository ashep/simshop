package product

import (
	"strings"
)

// LookupPromo finds code among the product's promo codes, ignoring case and
// surrounding whitespace. It returns the code as spelled in product.yaml.
func (p *Product) LookupPromo(code string) (string, int, bool) {
	norm := normalizePromo(code)
	if norm == "" {
		return "", 0, false
	}
	for c, pct := range p.PromoCodes {
		if normalizePromo(c) == norm {
			return c, pct, true
		}
	}
	return "", 0, false
}

// Discount returns how much percent takes off subtotalCents, chosen so the
// discounted total lands on whole currency units (half rounds up): no kopecks on
// the storefront or the invoice. Integer math keeps it exact; main.js
// promoTotalCents mirrors it.
func Discount(subtotalCents, percent int) int {
	netUnits := (subtotalCents*(100-percent) + 5000) / 10000
	return subtotalCents - netUnits*100
}

func normalizePromo(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
