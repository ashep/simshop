package property

import (
	"context"
	"fmt"
)

func (s *Service) List(ctx context.Context) ([]Property, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.id, pt.lang_id, pt.title
		FROM properties p
		LEFT JOIN property_titles pt ON pt.property_id = p.id
		ORDER BY p.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query properties: %w", err)
	}
	defer rows.Close()

	var result []Property
	byID := make(map[string]int)

	for rows.Next() {
		var id string
		var langID, title *string
		if err := rows.Scan(&id, &langID, &title); err != nil {
			return nil, fmt.Errorf("scan property row: %w", err)
		}

		idx, exists := byID[id]
		if !exists {
			result = append(result, Property{ID: id, Titles: map[string]string{}})
			idx = len(result) - 1
			byID[id] = idx
		}

		if langID != nil && title != nil {
			result[idx].Titles[*langID] = *title
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate property rows: %w", err)
	}

	if result == nil {
		result = []Property{}
	}

	return result, nil
}
