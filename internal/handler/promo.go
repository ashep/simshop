package handler

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ashep/simshop/internal/product"
	"gopkg.in/yaml.v3"
)

type promoCodeResponse struct {
	Code            string `json:"code"`
	DiscountPercent int    `json:"discount_percent"`
}

// CheckPromoCode reports whether a promo code is valid for a product so the
// storefront can preview the discount. Unknown products and unknown codes get
// the same 404, and the product's code list is never exposed.
func (h *Handler) CheckPromoCode(w http.ResponseWriter, r *http.Request) {
	notFound := &NotFoundError{Reason: "promo code not found"}

	id := r.PathValue("id")
	if id != filepath.Base(id) || id == "" || id == "." {
		h.writeError(w, notFound)
		return
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "products", id, "product.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		h.writeError(w, notFound)
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		h.writeError(w, err)
		return
	}

	code, percent, ok := p.LookupPromo(r.PathValue("code"))
	if !ok {
		h.writeError(w, notFound)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, &promoCodeResponse{Code: code, DiscountPercent: percent}); err != nil {
		h.l.Error().Err(err).Msg("write promo code response failed")
	}
}
