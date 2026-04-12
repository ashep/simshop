package shop

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) Get(ctx context.Context, id string) (*AdminShop, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.owner_id, s.created_at, s.updated_at, sn.lang_id, sn.title, sn.description
		FROM shops s
		LEFT JOIN shop_data sn ON sn.shop_id = s.id
		WHERE s.id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query shop: %w", err)
	}
	defer rows.Close()

	var sh *AdminShop

	for rows.Next() {
		var shopID, ownerID string
		var createdAt, updatedAt time.Time
		var langID, title, description *string
		if err := rows.Scan(&shopID, &ownerID, &createdAt, &updatedAt, &langID, &title, &description); err != nil {
			return nil, fmt.Errorf("scan shop row: %w", err)
		}

		if sh == nil {
			sh = &AdminShop{
				Shop:      Shop{ID: shopID, Titles: map[string]string{}, Descriptions: map[string]string{}},
				OwnerID:   ownerID,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}
		}

		if langID != nil && title != nil {
			sh.Titles[*langID] = *title
		}
		if langID != nil && description != nil {
			sh.Descriptions[*langID] = *description
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shop rows: %w", err)
	}

	if sh == nil {
		return nil, ErrShopNotFound
	}

	return sh, nil
}
