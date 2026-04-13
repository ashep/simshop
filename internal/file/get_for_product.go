package file

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

		diskPath := filepath.Join(s.publicDir, "files", fi.ID, fi.Name)
		if _, statErr := os.Stat(diskPath); statErr != nil {
			if !errors.Is(statErr, fs.ErrNotExist) {
				return nil, fmt.Errorf("stat file: %w", statErr)
			}
			dirPath := filepath.Join(s.publicDir, "files", fi.ID)
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				return nil, fmt.Errorf("create file dir: %w", err)
			}
			if err := os.WriteFile(diskPath, data, 0644); err != nil {
				return nil, fmt.Errorf("write file to disk: %w", err)
			}
		}

		fi.Path = "/files/" + fi.ID + "/" + fi.Name
		result = append(result, fi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file rows: %w", err)
	}

	return result, nil
}
