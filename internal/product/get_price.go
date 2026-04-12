package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetPrice(ctx context.Context, id string, countryID string) (*PriceResult, error) {
	var count int
	err := s.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM products WHERE id = $1 AND deleted_at IS NULL", id,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("check product existence: %w", err)
	}
	if count == 0 {
		return nil, ErrProductNotFound
	}

	result := &PriceResult{}
	err = s.db.QueryRow(ctx, `
		SELECT country_id, value
		FROM product_prices
		WHERE product_id = $1 AND country_id = ANY($2)
		ORDER BY CASE WHEN country_id = $3 THEN 0 ELSE 1 END
		LIMIT 1
	`, id, []string{countryID, "DEFAULT"}, countryID).Scan(&result.CountryID, &result.Value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PriceResult{CountryID: "DEFAULT", Value: 0}, nil
		}
		return nil, fmt.Errorf("query product price: %w", err)
	}

	return result, nil
}
