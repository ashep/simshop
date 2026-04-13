package file

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) Upload(ctx context.Context, req UploadRequest) (*FileInfo, error) {
	if !req.IsAdmin {
		var count int
		if err := s.db.QueryRow(ctx,
			"SELECT COUNT(*) FROM files WHERE owner_id = $1 AND deleted_at IS NULL",
			req.OwnerID,
		).Scan(&count); err != nil {
			return nil, fmt.Errorf("count user files: %w", err)
		}
		if count >= s.maxNumPerUser {
			return nil, ErrFileLimitReached
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var id string
	var createdAt, updatedAt time.Time
	if err := tx.QueryRow(ctx,
		"INSERT INTO files (owner_id, name, mime_type, size_bytes, data) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at",
		req.OwnerID, req.Name, req.MimeType, req.Size, req.Data,
	).Scan(&id, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("insert file: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	path, err := s.materialize(id, req.Name, req.Data)
	if err != nil {
		return nil, fmt.Errorf("materialize file %s: %w", id, err)
	}

	return &FileInfo{
		ID:        id,
		Name:      req.Name,
		MimeType:  req.MimeType,
		SizeBytes: req.Size,
		Path:      path,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
