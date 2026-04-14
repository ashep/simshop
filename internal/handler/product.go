package handler

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ashep/simshop/internal/product"
	"gopkg.in/yaml.v3"
)

type productService interface {
	List(ctx context.Context) ([]*product.Item, error)
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

func (h *Handler) ServeProductContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lang := r.PathValue("lang")

	if id != filepath.Base(id) || id == "" || id == "." ||
		lang != filepath.Base(lang) || lang == "" || lang == "." {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "products", id, "product.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
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
	p.ID = id

	if _, ok := p.Name[lang]; !ok {
		h.writeError(w, &NotFoundError{Reason: "language not found"})
		return
	}

	for i, img := range p.Images {
		if img.Preview != "" {
			p.Images[i].Preview = "/images/" + id + "/" + img.Preview
		}
		if img.Full != "" {
			p.Images[i].Full = "/images/" + id + "/" + img.Full
		}
	}

	country := h.geo.Detect(r)
	price, ok := p.Price[country]
	if !ok {
		price = p.Price["default"]
	}

	detail := product.ProductDetail{
		ID:          p.ID,
		Name:        p.Name[lang],
		Description: p.Description[lang],
		Price:       price,
		Images:      p.Images,
	}
	if len(p.Specs) > 0 {
		detail.Specs = make(map[string]product.SpecItem, len(p.Specs))
		for key, spec := range p.Specs {
			detail.Specs[key] = spec[lang]
		}
	}
	if len(p.Attrs) > 0 {
		detail.Attrs = make(map[string]product.AttrLang, len(p.Attrs))
		for key, attr := range p.Attrs {
			detail.Attrs[key] = attr[lang]
		}
	}

	if err := h.resp.Write(w, r, http.StatusOK, detail); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
