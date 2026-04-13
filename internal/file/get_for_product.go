package file

import (
	"context"
	"fmt"
)

func (s *Service) GetForProduct(ctx context.Context, productID string) ([]FileInfo, error) {
	rows, err := s.db.Query(ctx,
		`SELECT f.id, f.name, f.mime_type, f.size_bytes, f.data, f.created_at, f.updated_at
		 FROM product_files pf
		 JOIN files f ON f.id = pf.file_id
		 WHERE pf.product_id = $1 AND f.deleted_at IS NULL
		 ORDER BY f.created_at`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("query product files: %w", err)
	}
	defer rows.Close()

	result := make([]FileInfo, 0)
	for rows.Next() {
		var fi FileInfo
		var data []byte
		if err := rows.Scan(&fi.ID, &fi.Name, &fi.MimeType, &fi.SizeBytes, &data,
			&fi.CreatedAt, &fi.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file row: %w", err)
		}

		path, err := s.materialize(fi.ID, fi.Name, data)
		if err != nil {
			return nil, fmt.Errorf("materialize file %s: %w", fi.ID, err)
		}
		fi.Path = path
		result = append(result, fi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file rows: %w", err)
	}

	return result, nil
}
