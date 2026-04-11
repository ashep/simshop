package shop

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) Get(ctx context.Context, id string) (*AdminShop, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.owner_id, s.created_at, s.updated_at, sn.lang_id, sn.name
		FROM shops s
		LEFT JOIN shop_names sn ON sn.shop_id = s.id
		WHERE s.id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query shop: %w", err)
	}
	defer rows.Close()

	var sh *AdminShop

	for rows.Next() {
		var shopID string
		var ownerID *string
		var createdAt, updatedAt time.Time
		var langID, name *string
		if err := rows.Scan(&shopID, &ownerID, &createdAt, &updatedAt, &langID, &name); err != nil {
			return nil, fmt.Errorf("scan shop row: %w", err)
		}

		if sh == nil {
			sh = &AdminShop{
				Shop:      Shop{ID: shopID, Names: map[string]string{}},
				OwnerID:   ownerID,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}
		}

		if langID != nil && name != nil {
			sh.Names[*langID] = *name
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
