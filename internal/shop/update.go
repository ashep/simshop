package shop

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type UpdateRequest struct {
	Names map[string]string `json:"names"`
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for lang, name := range req.Names {
		if _, err = tx.Exec(ctx,
			`INSERT INTO shop_names (shop_id, lang_id, name)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (shop_id, lang_id) DO UPDATE SET name = excluded.name`,
			id, lang, name,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				switch pgErr.ConstraintName {
				case "shop_names_shop_id_fkey":
					return ErrShopNotFound
				case "shop_names_lang_id_fkey":
					return ErrInvalidLanguage
				}
			}
			return fmt.Errorf("upsert shop name: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
