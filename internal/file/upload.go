package file

import (
	"context"
	"fmt"
)

func (s *Service) Upload(ctx context.Context, req UploadRequest) (*File, error) {
	if !req.IsAdmin {
		var count int
		if err := s.db.QueryRow(ctx,
			"SELECT COUNT(*) FROM files WHERE owner_id = $1 AND deleted_at IS NULL",
			req.OwnerID,
		).Scan(&count); err != nil {
			return nil, fmt.Errorf("count user files: %w", err)
		}
		if count >= s.cfg.MaxNumPerUser {
			return nil, ErrFileLimitReached
		}
	}

	var id string
	if err := s.db.QueryRow(ctx,
		"INSERT INTO files (owner_id, mime_type, size_bytes, data) VALUES ($1, $2, $3, $4) RETURNING id",
		req.OwnerID, req.MimeType, req.Size, req.Data,
	).Scan(&id); err != nil {
		return nil, fmt.Errorf("insert file: %w", err)
	}

	return &File{ID: id}, nil
}
