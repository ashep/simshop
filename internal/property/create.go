package property

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Property, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if req.Titles["EN"] == "" {
		return nil, ErrMissingEnTitle
	}

	var propertyID string
	if err = tx.QueryRow(ctx,
		"INSERT INTO properties DEFAULT VALUES RETURNING id",
	).Scan(&propertyID); err != nil {
		return nil, fmt.Errorf("insert property: %w", err)
	}

	for lang, title := range req.Titles {
		if _, err = tx.Exec(ctx,
			"INSERT INTO property_titles (property_id, lang_id, title) VALUES ($1, $2, $3)",
			propertyID, lang, title,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case "23503":
					// property_id came from RETURNING above and cannot violate the FK;
					// 23503 here is always an invalid lang_id.
					return nil, ErrInvalidLanguage
				case "23505":
					return nil, ErrDuplicateTitle
				}
			}
			return nil, fmt.Errorf("insert property title: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &Property{ID: propertyID}, nil
}
