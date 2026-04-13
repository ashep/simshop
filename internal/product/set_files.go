package product

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) SetFiles(ctx context.Context, id string, req SetFilesRequest) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify the product exists atomically.
	tag, err := tx.Exec(ctx,
		"UPDATE products SET updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL", id,
	)
	if err != nil {
		return fmt.Errorf("update product timestamp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	if !req.IsAdmin {
		// Fetch the shop owner to validate file ownership.
		var shopOwnerID string
		err = tx.QueryRow(ctx,
			`SELECT s.owner_id FROM products p
			 JOIN shops s ON s.id = p.shop_id
			 WHERE p.id = $1`, id,
		).Scan(&shopOwnerID)
		if err != nil {
			return fmt.Errorf("fetch shop owner: %w", err)
		}

		for _, fileID := range req.FileIDs {
			var fileOwnerID string
			err = tx.QueryRow(ctx,
				"SELECT owner_id FROM files WHERE id = $1 AND deleted_at IS NULL", fileID,
			).Scan(&fileOwnerID)
			if err == pgx.ErrNoRows {
				return ErrFileNotFound
			}
			if err != nil {
				return fmt.Errorf("fetch file owner: %w", err)
			}
			if fileOwnerID != shopOwnerID {
				return ErrFileOwnerMismatch
			}
		}
	} else {
		// Admin: still verify that each file exists.
		for _, fileID := range req.FileIDs {
			var exists bool
			err = tx.QueryRow(ctx,
				"SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND deleted_at IS NULL)", fileID,
			).Scan(&exists)
			if err != nil {
				return fmt.Errorf("check file existence: %w", err)
			}
			if !exists {
				return ErrFileNotFound
			}
		}
	}

	if _, err = tx.Exec(ctx, "DELETE FROM product_files WHERE product_id = $1", id); err != nil {
		return fmt.Errorf("delete product files: %w", err)
	}

	for _, fileID := range req.FileIDs {
		if _, err = tx.Exec(ctx,
			"INSERT INTO product_files (product_id, file_id) VALUES ($1, $2)",
			id, fileID,
		); err != nil {
			return fmt.Errorf("insert product file: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
