package handler

import (
	"context"
	"net/http"

	"github.com/ashep/simshop/internal/shop"
)

type shopService interface {
	Get(ctx context.Context) (*shop.Shop, error)
}

func (h *Handler) ServeShop(w http.ResponseWriter, r *http.Request) {
	s, err := h.shop.Get(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if err := h.resp.Write(w, r, http.StatusOK, s); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
