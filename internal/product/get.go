package product

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) Get(ctx context.Context, id string) (*AdminProduct, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.created_at, p.updated_at, s.owner_id, pd.lang_id, pd.title, pd.description
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		LEFT JOIN product_data pd ON pd.product_id = p.id
		WHERE p.id = $1 AND p.deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query product: %w", err)
	}
	defer rows.Close()

	var p *AdminProduct

	for rows.Next() {
		var createdAt, updatedAt time.Time
		var ownerID string
		var langID, title, description *string

		if err := rows.Scan(&createdAt, &updatedAt, &ownerID, &langID, &title, &description); err != nil {
			return nil, fmt.Errorf("scan product row: %w", err)
		}

		if p == nil {
			p = &AdminProduct{
				PublicProduct: PublicProduct{
					ID:      id,
					Content: map[string]ContentItem{},
				},
				ShopOwnerID: ownerID,
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			}
		}

		if langID != nil && title != nil && description != nil {
			p.Content[*langID] = ContentItem{
				Title:       *title,
				Description: *description,
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product rows: %w", err)
	}

	if p == nil {
		return nil, ErrProductNotFound
	}

	return p, nil
}
