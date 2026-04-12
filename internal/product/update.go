package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) error {
	if req.Content["EN"].Title == "" {
		return ErrMissingEnTitle
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Update timestamp and verify the product exists in one atomic step.
	tag, err := tx.Exec(ctx,
		"UPDATE products SET updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL", id,
	)
	if err != nil {
		return fmt.Errorf("update product timestamp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	if _, err = tx.Exec(ctx, "DELETE FROM product_data WHERE product_id = $1", id); err != nil {
		return fmt.Errorf("delete product content: %w", err)
	}

	for lang, c := range req.Content {
		if _, err = tx.Exec(ctx,
			"INSERT INTO product_data (product_id, lang_id, title, description) VALUES ($1, $2, $3, $4)",
			id, lang, c.Title, c.Description,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return ErrInvalidLanguage
			}
			return fmt.Errorf("insert product content: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
