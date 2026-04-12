package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
)

type productService interface {
	Create(ctx context.Context, req product.CreateRequest) (*product.Product, error)
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	req := product.CreateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	if !user.IsAdmin() {
		sh, err := h.shop.Get(r.Context(), req.ShopID)
		if errors.Is(err, shop.ErrShopNotFound) {
			h.writeError(w, &NotFoundError{Reason: "shop not found"})
			return
		} else if err != nil {
			h.writeError(w, err)
			return
		}
		if sh.OwnerID != user.ID {
			h.writeError(w, &PermissionDeniedError{})
			return
		}
	}

	p, err := h.prod.Create(r.Context(), req)
	if err != nil {
		var mce *product.MissingContentError
		switch {
		case errors.Is(err, product.ErrShopNotFound):
			h.writeError(w, &NotFoundError{Reason: "shop not found"})
		case errors.As(err, &mce):
			h.writeError(w, &BadRequestError{Reason: mce.Error()})
		case errors.Is(err, product.ErrMissingDefaultPrice):
			h.writeError(w, &BadRequestError{Reason: "default country price is required"})
		case errors.Is(err, product.ErrInvalidCountry):
			h.writeError(w, &BadRequestError{Reason: "invalid country id"})
		case errors.Is(err, product.ErrInvalidLanguage):
			h.writeError(w, &BadRequestError{Reason: "invalid language code"})
		case errors.Is(err, product.ErrShopProductLimitReached):
			h.writeError(w, &ConflictError{Reason: "shop product limit reached"})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("product_id", p.ID).Str("shop_id", req.ShopID).Str("user_id", user.ID).Msg("product created")

	if err := h.resp.Write(w, r, http.StatusCreated, &product.CreateResponse{ID: p.ID}); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
