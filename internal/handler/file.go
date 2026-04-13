package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/file"
)

type fileService interface {
	Upload(ctx context.Context, req file.UploadRequest) (*file.File, error)
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
	defer f.Close()

	if fh.Size > maxSize {
		h.writeError(w, &BadRequestError{Reason: "file too large"})
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
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

	if err := h.resp.Write(w, r, http.StatusCreated, &file.UploadResponse{ID: result.ID}); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
