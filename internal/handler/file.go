package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/product"
)

type fileService interface {
	GetForProduct(ctx context.Context, productID string) ([]file.FileInfo, error)
}

func (h *Handler) GetProductFiles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	_, err := h.prod.Get(r.Context(), id)
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

	if err := h.resp.Write(w, r, http.StatusOK, files); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
