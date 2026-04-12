package shop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type CreateRequest struct {
	ID           string            `json:"id"`
	Titles       map[string]string `json:"titles"`
	Descriptions map[string]string `json:"descriptions"`
	OwnerID      string            `json:"owner_id"`
}

func (r *CreateRequest) Trim() {
	r.ID = strings.TrimSpace(r.ID)
	r.OwnerID = strings.TrimSpace(r.OwnerID)
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

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Shop, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx,
		"INSERT INTO shops (id, owner_id) VALUES ($1, $2)",
		req.ID, req.OwnerID,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrShopAlreadyExists
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrInvalidOwner
		}
		return nil, fmt.Errorf("insert shop: %w", err)
	}

	for lang, title := range req.Titles {
		var desc *string
		if d, ok := req.Descriptions[lang]; ok {
			desc = &d
		}
		if _, err = tx.Exec(ctx,
			"INSERT INTO shop_data (shop_id, lang_id, title, description) VALUES ($1, $2, $3, $4)",
			req.ID, lang, title, desc,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return nil, ErrInvalidLanguage
			}
			return nil, fmt.Errorf("insert shop metadata: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &Shop{ID: req.ID, Titles: req.Titles, Descriptions: req.Descriptions}, nil
}
