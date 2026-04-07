package handler

import (
	"net/http"

	"github.com/ashep/simshop/internal/product"
)

type productSservice interface {
	Create(request product.CreateRequest) (*product.Product, error)
}

func (h *Handler) ProductCreate(w http.ResponseWriter, r *http.Request) {
	req := &product.CreateRequest{}
	if err := h.unmarshal(r.Body, req); err != nil {
		h.writeError(w, err)
		return
	}

	h.l.Info().Msg("handling post product request")

	p, err := h.prod.Create(*req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusCreated, &product.CreateResponse{ID: p.ID, Name: p.Name}); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
		return
	}
}
