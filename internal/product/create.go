package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Product, error) {
	// Fetch shop existence, max_products, and its language set in one query.
	// LEFT JOIN: if shop exists but has no names → one row with null lang_id.
	// No rows at all → shop not found.
	rows, err := s.db.Query(ctx, `
		SELECT s.max_products, sn.lang_id
		FROM shops s
		LEFT JOIN shop_data sn ON sn.shop_id = s.id
		WHERE s.id = $1
	`, req.ShopID)
	if err != nil {
		return nil, fmt.Errorf("query shop languages: %w", err)
	}
	defer rows.Close()

	shopFound := false
	maxProducts := 0
	shopLangs := make(map[string]struct{})

	for rows.Next() {
		shopFound = true
		var lang *string
		if err := rows.Scan(&maxProducts, &lang); err != nil {
			return nil, fmt.Errorf("scan shop language: %w", err)
		}
		if lang != nil {
			shopLangs[*lang] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shop languages: %w", err)
	}
	rows.Close() // release pool connection before opening the transaction

	if !shopFound {
		return nil, ErrShopNotFound
	}

	// Check product limit (only non-deleted products count).
	var productCount int
	if err = s.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM products WHERE shop_id = $1 AND deleted_at IS NULL",
		req.ShopID,
	).Scan(&productCount); err != nil {
		return nil, fmt.Errorf("count shop products: %w", err)
	}
	if productCount >= maxProducts {
		return nil, ErrShopProductLimitReached
	}

	// Validate content covers every shop language and that title/description are non-empty.
	for lang := range shopLangs {
		c, ok := req.Data[lang]
		if !ok {
			return nil, &MissingContentError{Lang: lang}
		}
		if c.Title == "" {
			return nil, &MissingTitleError{Lang: lang}
		}
		if c.Description == "" {
			return nil, &MissingDescriptionError{Lang: lang}
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var productID string
	if err = tx.QueryRow(ctx,
		"INSERT INTO products (shop_id) VALUES ($1) RETURNING id",
		req.ShopID,
	).Scan(&productID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrShopNotFound
		}
		return nil, fmt.Errorf("insert product: %w", err)
	}

	for lang, c := range req.Data {
		if _, err = tx.Exec(ctx,
			"INSERT INTO product_data (product_id, lang_id, title, description) VALUES ($1, $2, $3, $4)",
			productID, lang, c.Title, c.Description,
		); err != nil {
			var pgErr *pgconn.PgError
			// product_id came from RETURNING above and cannot violate the FK;
			// 23503 here is always an invalid lang_id.
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return nil, &InvalidLanguageError{Lang: lang}
			}
			return nil, fmt.Errorf("insert product data: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &Product{ID: productID}, nil
}
