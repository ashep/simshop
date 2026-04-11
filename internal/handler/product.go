package handler

import (
	"context"
	"net/http"

	"github.com/ashep/simshop/internal/product"
)

type productService interface {
	Create(ctx context.Context, req product.CreateRequest) (*product.Product, error)
}

func (h *Handler) ProductCreate(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
