package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/product"
)

type fileService interface {
	Upload(ctx context.Context, req file.UploadRequest) (*file.FileInfo, error)
	GetForProduct(ctx context.Context, productID string) ([]file.FileInfo, error)
}

func (h *Handler) GetProductFiles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	p, err := h.prod.Get(r.Context(), id)
	if errors.Is(err, product.ErrProductNotFound) {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	files, err := h.file.GetForProduct(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	var body any
	if user != nil && (user.IsAdmin() || user.ID == p.ShopOwnerID) {
		items := make([]file.AdminFileItem, len(files))
		for i, f := range files {
			items[i] = file.AdminFileItem{
				PublicFileItem: file.PublicFileItem{
					ID:        f.ID,
					Name:      f.Name,
					MimeType:  f.MimeType,
					SizeBytes: f.SizeBytes,
					Path:      f.Path,
				},
				CreatedAt: f.CreatedAt,
				UpdatedAt: f.UpdatedAt,
			}
		}
		body = items
	} else {
		items := make([]file.PublicFileItem, len(files))
		for i, f := range files {
			items[i] = file.PublicFileItem{
				ID:        f.ID,
				Name:      f.Name,
				MimeType:  f.MimeType,
				SizeBytes: f.SizeBytes,
				Path:      f.Path,
			}
		}
		body = items
	}

	if err := h.resp.Write(w, r, http.StatusOK, body); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	maxSize := int64(h.fileMaxSize)
	// Limit the total request body. The +1024 accounts for multipart boundary/header overhead
	// so a file at exactly maxSize bytes is not rejected by the reader limit.
	r.Body = http.MaxBytesReader(w, r.Body, maxSize+1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			h.writeError(w, &BadRequestError{Reason: "file too large"})
		} else {
			h.writeError(w, &BadRequestError{Reason: "failed to parse multipart form"})
		}
		return
	}

	f, fh, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, &BadRequestError{Reason: "file field is required"})
		return
	}
	defer f.Close() //nolint:errcheck

	if fh.Size > maxSize {
		h.writeError(w, &BadRequestError{Reason: "file too large"})
		return
	}

	name := filepath.Base(strings.TrimSpace(r.FormValue("name")))
	if name == "" || name == "." {
		h.writeError(w, &BadRequestError{Reason: "name field is required"})
		return
	}

	// Read first 512 bytes for MIME sniffing.
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		h.writeError(w, err)
		return
	}
	mimeType := http.DetectContentType(buf[:n])

	allowed := false
	for _, t := range h.fileAllowedMTs {
		if t == mimeType {
			allowed = true
			break
		}
	}
	if !allowed {
		h.writeError(w, &BadRequestError{Reason: "unsupported file type"})
		return
	}

	// Seek back to start, then read full content.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		h.writeError(w, err)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		h.writeError(w, err)
		return
	}

	result, err := h.file.Upload(r.Context(), file.UploadRequest{
		OwnerID:  user.ID,
		Name:     name,
		MimeType: mimeType,
		Size:     len(data),
		Data:     data,
		IsAdmin:  user.IsAdmin(),
	})
	if err != nil {
		if errors.Is(err, file.ErrFileLimitReached) {
			h.writeError(w, &ConflictError{Reason: "file limit reached"})
		} else {
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("file_id", result.ID).Str("user_id", user.ID).Msg("file uploaded")

	if err := h.resp.Write(w, r, http.StatusCreated, &file.UploadResponse{
		ID:        result.ID,
		Name:      result.Name,
		MimeType:  result.MimeType,
		SizeBytes: result.SizeBytes,
		Path:      result.Path,
		CreatedAt: result.CreatedAt,
		UpdatedAt: result.UpdatedAt,
	}); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
