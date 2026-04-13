package handler

import (
	"context"
	"net/http"

	"github.com/ashep/simshop/internal/property"
)

type propertyService interface {
	List(ctx context.Context) ([]property.Property, error)
}

func (h *Handler) ListProperties(w http.ResponseWriter, r *http.Request) {
	props, err := h.prop.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if err := h.resp.Write(w, r, http.StatusOK, props); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
