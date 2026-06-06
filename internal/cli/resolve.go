package cli

import (
	"context"
	"fmt"
	"strings"
)

// isFullUUID reports whether s is a canonical 8-4-4-4-12 hex UUID. A full UUID is
// passed straight to the server; anything shorter is treated as a short id prefix.
func isFullUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexDigit(r) {
				return false
			}
		}
	}
	return true
}

// isHexDigit reports whether r is a hexadecimal digit (either case).
func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// matchOrder resolves a possibly-short id against orders. An exact (case-insensitive)
// id match wins; otherwise the id must be a unique case-insensitive prefix. No match
// errors as "not found"; more than one prefix match errors as "ambiguous".
func matchOrder(orders []Order, id string) (*Order, error) {
	lid := strings.ToLower(id)
	var matches []*Order
	for i := range orders {
		oid := strings.ToLower(orders[i].ID)
		if oid == lid {
			return &orders[i], nil
		}
		if strings.HasPrefix(oid, lid) {
			matches = append(matches, &orders[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("order %q not found", id)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("order id %q is ambiguous, matches %d orders: %s", id, len(matches), strings.Join(ids, ", "))
	}
}

// ResolveOrderID returns the full order id for a possibly-short id. A full UUID is
// returned as-is without a lookup; a short id is resolved against the order list.
func (c *Client) ResolveOrderID(ctx context.Context, id string) (string, error) {
	if isFullUUID(id) {
		return id, nil
	}
	orders, err := c.ListOrders(ctx, nil)
	if err != nil {
		return "", err
	}
	o, err := matchOrder(orders, id)
	if err != nil {
		return "", err
	}
	return o.ID, nil
}
