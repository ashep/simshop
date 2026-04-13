package file

import (
	"errors"
	"time"
)

var ErrFileLimitReached = errors.New("file limit reached")

type File struct {
	ID string
}

type UploadRequest struct {
	OwnerID  string
	Name     string
	MimeType string
	Size     int
	Data     []byte
	IsAdmin  bool
}

type UploadResponse struct {
	ID string `json:"id"`
}

// FileInfo is the internal record returned by GetForProduct.
type FileInfo struct {
	ID        string
	Name      string
	MimeType  string
	SizeBytes int
	Path      string    // URL-relative path, e.g. /files/{id}/{name}
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PublicFileItem is the JSON shape returned to unauthenticated / non-owner callers.
type PublicFileItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int    `json:"size_bytes"`
	Path      string `json:"path"`
}

// AdminFileItem is the JSON shape returned to admins and shop owners.
type AdminFileItem struct {
	PublicFileItem
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
