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
	Get(ctx context.Context, id string) (*product.AdminProduct, error)
	ListByShop(ctx context.Context, shopID string) ([]*product.AdminProduct, error)
	Update(ctx context.Context, id string, req product.UpdateRequest) error
	SetPrices(ctx context.Context, id string, prices map[string]int) error
	GetPrice(ctx context.Context, id string, countryID string) (*product.PriceResult, error)
	SetFiles(ctx context.Context, id string, req product.SetFilesRequest) error
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	req := product.CreateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

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
		case errors.As(err, new(*product.InvalidLanguageError)):
			h.writeError(w, &BadRequestError{Reason: err.Error()})
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

func (h *Handler) ListShopProducts(w http.ResponseWriter, r *http.Request) {
	shopID := r.PathValue("id")

	sh, err := h.shop.Get(r.Context(), shopID)
	if errors.Is(err, shop.ErrShopNotFound) {
		h.writeError(w, &NotFoundError{Reason: "shop not found"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	products, err := h.prod.ListByShop(r.Context(), shopID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	var body any
	if user != nil && (user.IsAdmin() || user.ID == sh.OwnerID) {
		body = products
	} else {
		pub := make([]product.PublicProduct, len(products))
		for i, p := range products {
			pub[i] = p.PublicProduct
		}
		body = pub
	}

	if err := h.resp.Write(w, r, http.StatusOK, body); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	p, err := h.prod.Get(r.Context(), id)
	if errors.Is(err, product.ErrProductNotFound) {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	var body any
	if user != nil && (user.IsAdmin() || user.ID == p.ShopOwnerID) {
		body = p
	} else {
		body = p.PublicProduct
	}

	if err := h.resp.Write(w, r, http.StatusOK, body); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	req := product.UpdateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	id := r.PathValue("id")

	if !user.IsAdmin() {
		p, err := h.prod.Get(r.Context(), id)
		if errors.Is(err, product.ErrProductNotFound) {
			h.writeError(w, &NotFoundError{Reason: "product not found"})
			return
		} else if err != nil {
			h.writeError(w, err)
			return
		}
		if p.ShopOwnerID != user.ID {
			h.writeError(w, &PermissionDeniedError{})
			return
		}
	}

	if err := h.prod.Update(r.Context(), id, req); err != nil {
		switch {
		case errors.Is(err, product.ErrProductNotFound):
			h.writeError(w, &NotFoundError{Reason: "product not found"})
		case errors.Is(err, product.ErrMissingTitle):
			h.writeError(w, &BadRequestError{Reason: "at least one title is required"})
		case errors.As(err, new(*product.InvalidLanguageError)):
			h.writeError(w, &BadRequestError{Reason: err.Error()})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("product_id", id).Str("user_id", user.ID).Msg("product updated")

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) SetProductPrices(w http.ResponseWriter, r *http.Request) {
	req := product.SetPricesRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	id := r.PathValue("id")

	if !user.IsAdmin() {
		p, err := h.prod.Get(r.Context(), id)
		if errors.Is(err, product.ErrProductNotFound) {
			h.writeError(w, &NotFoundError{Reason: "product not found"})
			return
		} else if err != nil {
			h.writeError(w, err)
			return
		}
		if p.ShopOwnerID != user.ID {
			h.writeError(w, &PermissionDeniedError{})
			return
		}
	}

	if err := h.prod.SetPrices(r.Context(), id, req.Prices); err != nil {
		switch {
		case errors.Is(err, product.ErrProductNotFound):
			h.writeError(w, &NotFoundError{Reason: "product not found"})
		case errors.As(err, new(*product.InvalidCountryError)):
			h.writeError(w, &BadRequestError{Reason: err.Error()})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("product_id", id).Str("user_id", user.ID).Msg("product prices updated")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) SetProductFiles(w http.ResponseWriter, r *http.Request) {
	req := product.SetFilesRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}

	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	id := r.PathValue("id")

	if !user.IsAdmin() {
		p, err := h.prod.Get(r.Context(), id)
		if errors.Is(err, product.ErrProductNotFound) {
			h.writeError(w, &NotFoundError{Reason: "product not found"})
			return
		} else if err != nil {
			h.writeError(w, err)
			return
		}
		if p.ShopOwnerID != user.ID {
			h.writeError(w, &PermissionDeniedError{})
			return
		}
	}

	req.IsAdmin = user.IsAdmin()

	if err := h.prod.SetFiles(r.Context(), id, req); err != nil {
		switch {
		case errors.Is(err, product.ErrProductNotFound):
			h.writeError(w, &NotFoundError{Reason: "product not found"})
		case errors.Is(err, product.ErrFileNotFound):
			h.writeError(w, &NotFoundError{Reason: "file not found"})
		case errors.Is(err, product.ErrFileOwnerMismatch):
			h.writeError(w, &PermissionDeniedError{})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("product_id", id).Str("user_id", user.ID).Msg("product files updated")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetProductPrice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	country := r.URL.Query().Get("country")
	if country == "" {
		country = "DEFAULT"
	}

	result, err := h.prod.GetPrice(r.Context(), id, country)
	if errors.Is(err, product.ErrProductNotFound) {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
		return
	} else if err != nil {
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, result); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
