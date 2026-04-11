package shop

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type UpdateRequest struct {
	Names        map[string]string `json:"names"`
	Descriptions map[string]string `json:"descriptions"`
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Pass 1: upsert names; include description for same lang if provided.
	for lang, name := range req.Names {
		var desc *string
		if d, ok := req.Descriptions[lang]; ok {
			desc = &d
		}
		if _, err = tx.Exec(ctx,
			`INSERT INTO shop_metadata (shop_id, lang_id, name, description)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (shop_id, lang_id) DO UPDATE
			 SET name = excluded.name,
			     description = COALESCE(excluded.description, shop_metadata.description)`,
			id, lang, name, desc,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				switch pgErr.ConstraintName {
				case "shop_metadata_shop_id_fkey":
					return ErrShopNotFound
				case "shop_metadata_lang_id_fkey":
					return ErrInvalidLanguage
				}
			}
			return fmt.Errorf("upsert shop metadata: %w", err)
		}
	}

	// Pass 2: update description only for langs not handled in pass 1.
	for lang, desc := range req.Descriptions {
		if _, inNames := req.Names[lang]; inNames {
			continue
		}
		d := desc
		var tag pgconn.CommandTag
		if tag, err = tx.Exec(ctx,
			`UPDATE shop_metadata SET description = $1 WHERE shop_id = $2 AND lang_id = $3`,
			&d, id, lang,
		); err != nil {
			return fmt.Errorf("update shop description: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrInvalidLanguage
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
