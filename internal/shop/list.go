package shop

import (
	"context"
	"fmt"
)

func (s *Service) List(ctx context.Context) ([]Shop, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, sn.lang_id, sn.name
		FROM shops s
		LEFT JOIN shop_names sn ON sn.shop_id = s.id
		ORDER BY s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query shops: %w", err)
	}
	defer rows.Close()

	var result []Shop
	byID := make(map[string]int)

	for rows.Next() {
		var id string
		var langID, name *string
		if err := rows.Scan(&id, &langID, &name); err != nil {
			return nil, fmt.Errorf("scan shop row: %w", err)
		}

		idx, exists := byID[id]
		if !exists {
			result = append(result, Shop{ID: id, Names: map[string]string{}})
			idx = len(result) - 1
			byID[id] = idx
		}

		if langID != nil && name != nil {
			result[idx].Names[*langID] = *name
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shop rows: %w", err)
	}

	if result == nil {
		result = []Shop{}
	}

	return result, nil
}
