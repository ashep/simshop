package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) SetPrices(ctx context.Context, id string, prices map[string]int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify the product exists atomically.
	tag, err := tx.Exec(ctx,
		"UPDATE products SET updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL", id,
	)
	if err != nil {
		return fmt.Errorf("update product timestamp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	if _, err = tx.Exec(ctx, "DELETE FROM product_prices WHERE product_id = $1", id); err != nil {
		return fmt.Errorf("delete product prices: %w", err)
	}

	for countryID, value := range prices {
		if _, err = tx.Exec(ctx,
			"INSERT INTO product_prices (product_id, country_id, value) VALUES ($1, $2, $3)",
			id, countryID, value,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return &InvalidCountryError{Country: countryID}
			}
			return fmt.Errorf("insert product price: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
