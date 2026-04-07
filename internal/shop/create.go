package shop

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type CreateRequest struct {
	ID    string            `json:"id"`
	Names map[string]string `json:"names"`
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Shop, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx, "INSERT INTO shops (id) VALUES ($1)", req.ID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrShopAlreadyExists
		}
		return nil, fmt.Errorf("insert shop: %w", err)
	}

	for lang, name := range req.Names {
		if _, err = tx.Exec(ctx,
			"INSERT INTO shop_names (shop_id, lang_id, name) VALUES ($1, $2, $3)",
			req.ID, lang, name,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return nil, ErrInvalidLanguage
			}
			return nil, fmt.Errorf("insert shop name: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &Shop{ID: req.ID, Names: req.Names}, nil
}
