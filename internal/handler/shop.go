package handler

import (
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/shop"
)

type shopService interface {
	Create(req shop.CreateRequest) (*shop.Shop, error)
}

func (h *Handler) ShopCreate(w http.ResponseWriter, r *http.Request) {
	req := shop.CreateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, errors.New("failed to create shop: no user in request context"))
		return
	}

	sh, err := h.shop.Create(req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.l.Info().Str("shop_id", sh.ID).Str("user_id", user.ID).Msg("shop created")

	if err := h.resp.Write(w, r, http.StatusCreated, &shop.CreateResponse{ID: sh.ID}); err != nil {
		h.l.Error().Err(err).Msg("response write failed")
		return
	}
}
