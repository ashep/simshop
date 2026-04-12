package product

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) ListByShop(ctx context.Context, shopID string) ([]*AdminProduct, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.created_at, p.updated_at, s.owner_id, pd.lang_id, pd.title, pd.description
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		LEFT JOIN product_data pd ON pd.product_id = p.id
		WHERE p.shop_id = $1 AND p.deleted_at IS NULL
		ORDER BY p.created_at DESC
	`, shopID)
	if err != nil {
		return nil, fmt.Errorf("query products by shop: %w", err)
	}
	defer rows.Close()

	index := map[string]*AdminProduct{}
	var order []string

	for rows.Next() {
		var id string
		var createdAt, updatedAt time.Time
		var ownerID string
		var langID, title, description *string

		if err := rows.Scan(&id, &createdAt, &updatedAt, &ownerID, &langID, &title, &description); err != nil {
			return nil, fmt.Errorf("scan product row: %w", err)
		}

		if _, exists := index[id]; !exists {
			index[id] = &AdminProduct{
				PublicProduct: PublicProduct{
					ID:      id,
					Content: map[string]ContentItem{},
				},
				ShopOwnerID: ownerID,
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			}
			order = append(order, id)
		}

		if langID != nil && title != nil && description != nil {
			index[id].Content[*langID] = ContentItem{
				Title:       *title,
				Description: *description,
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product rows: %w", err)
	}

	result := make([]*AdminProduct, 0, len(order))
	for _, id := range order {
		result = append(result, index[id])
	}

	return result, nil
}
