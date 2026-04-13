package handler

import (
	"context"
	"net/http"

	"github.com/ashep/simshop/internal/product"
)

type productService interface {
	List(ctx context.Context) ([]*product.Product, error)
}

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.prod.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if err := h.resp.Write(w, r, http.StatusOK, products); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
