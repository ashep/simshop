package file

import "errors"

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
