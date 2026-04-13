package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/product"
)

type productService interface {
	Get(ctx context.Context, id string) (*product.Product, error)
	List(ctx context.Context) ([]*product.Product, error)
	GetPrice(ctx context.Context, id string, countryID string) (*product.PriceResult, error)
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

	if err := h.resp.Write(w, r, http.StatusOK, p); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
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

	result.CountryID = country

	if err := h.resp.Write(w, r, http.StatusOK, result); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
