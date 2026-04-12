package property

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if len(req.Titles) == 0 {
		return ErrMissingTitle
	}

	// Verify property exists.
	var exists bool
	if err = tx.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM properties WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check property existence: %w", err)
	}
	if !exists {
		return ErrPropertyNotFound
	}

	// Replace all titles: delete existing, then re-insert.
	if _, err = tx.Exec(ctx,
		"DELETE FROM property_titles WHERE property_id = $1", id,
	); err != nil {
		return fmt.Errorf("delete property titles: %w", err)
	}

	for lang, title := range req.Titles {
		if _, err = tx.Exec(ctx,
			"INSERT INTO property_titles (property_id, lang_id, title) VALUES ($1, $2, $3)",
			id, lang, title,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return &InvalidLanguageError{Lang: lang}
			}
			return fmt.Errorf("insert property title: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
