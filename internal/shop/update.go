package shop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type UpdateRequest struct {
	Titles       map[string]string `json:"titles"`
	Descriptions map[string]string `json:"descriptions"`
}

func (r *UpdateRequest) Trim() {
	trimmed := make(map[string]string, len(r.Titles))
	for k, v := range r.Titles {
		trimmed[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	r.Titles = trimmed
	trimmed = make(map[string]string, len(r.Descriptions))
	for k, v := range r.Descriptions {
		trimmed[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	r.Descriptions = trimmed
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Atomic existence check: touch updated_at; zero rows means shop doesn't exist.
	tag, err := tx.Exec(ctx,
		"UPDATE shops SET updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("check shop existence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrShopNotFound
	}

	// Full replace: delete all existing language rows, then insert the new set.
	if _, err = tx.Exec(ctx, "DELETE FROM shop_data WHERE shop_id = $1", id); err != nil {
		return fmt.Errorf("delete shop data: %w", err)
	}

	for lang, title := range req.Titles {
		var desc *string
		if d, ok := req.Descriptions[lang]; ok {
			desc = &d
		}
		if _, err = tx.Exec(ctx,
			"INSERT INTO shop_data (shop_id, lang_id, title, description) VALUES ($1, $2, $3, $4)",
			id, lang, title, desc,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return &InvalidLanguageError{Lang: lang}
			}
			return fmt.Errorf("insert shop data: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
