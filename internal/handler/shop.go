package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/shop"
)

type shopService interface {
	Get(ctx context.Context, id string) (*shop.AdminShop, error)
	Create(ctx context.Context, req shop.CreateRequest) (*shop.Shop, error)
	Update(ctx context.Context, id string, req shop.UpdateRequest) error
	List(ctx context.Context) ([]shop.Shop, error)
}

func (h *Handler) GetShop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	sh, err := h.shop.Get(r.Context(), id)
	if errors.Is(err, shop.ErrShopNotFound) {
		h.writeError(w, &NotFoundError{Reason: "shop not found"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	user := auth.GetUserFromContext(r.Context())
	var body any
	if user != nil && user.IsAdmin() {
		body = sh
	} else {
		body = sh.Shop
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.l.Warn().Err(err).Msg("get shop response write failed")
	}
}

func (h *Handler) ListShops(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, errors.New("failed to list shops: no user in request context"))
		return
	}

	if !user.IsAdmin() {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	shops, err := h.shop.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(shops); err != nil {
		h.l.Warn().Err(err).Msg("list shops response write failed")
	}
}

func (h *Handler) CreateShop(w http.ResponseWriter, r *http.Request) {
	req := shop.CreateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

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
	} else if errors.As(err, new(*shop.InvalidLanguageError)) {
		h.writeError(w, &BadRequestError{Reason: err.Error()})
		return
	} else if errors.Is(err, shop.ErrInvalidOwner) {
		h.writeError(w, &BadRequestError{Reason: "invalid owner id"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	h.l.Info().Str("shop_id", sh.ID).Str("user_id", user.ID).Msg("shop created")

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) UpdateShop(w http.ResponseWriter, r *http.Request) {
	req := shop.UpdateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, errors.New("failed to update shop: no user in request context"))
		return
	}

	id := r.PathValue("id")

	if !user.IsAdmin() {
		sh, err := h.shop.Get(r.Context(), id)
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

	if err := h.shop.Update(r.Context(), id, req); err != nil {
		if errors.Is(err, shop.ErrShopNotFound) {
			h.writeError(w, &NotFoundError{Reason: "shop not found"})
		} else if errors.As(err, new(*shop.InvalidLanguageError)) {
			h.writeError(w, &BadRequestError{Reason: err.Error()})
		} else {
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("shop_id", id).Str("user_id", user.ID).Msg("shop updated")

	w.WriteHeader(http.StatusOK)
}
