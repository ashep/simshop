package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/shop"
)

type shopService interface {
	Create(ctx context.Context, req shop.CreateRequest) (*shop.Shop, error)
}

func (h *Handler) CreateShop(w http.ResponseWriter, r *http.Request) {
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

	if !user.IsAdmin() {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	sh, err := h.shop.Create(r.Context(), req)
	if errors.Is(err, shop.ErrShopAlreadyExists) {
		h.writeError(w, &ConflictError{Reason: "shop already exists"})
		return
	} else if errors.Is(err, shop.ErrInvalidLanguage) {
		h.writeError(w, &BadRequestError{Reason: "invalid language code"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	h.l.Info().Str("shop_id", sh.ID).Str("user_id", user.ID).Msg("shop created")

	w.WriteHeader(http.StatusCreated)
}
