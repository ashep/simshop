package handler

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ashep/simshop/internal/page"
)

type pageService interface {
	List(ctx context.Context) ([]*page.Page, error)
}

func (h *Handler) ListPages(w http.ResponseWriter, r *http.Request) {
	pages, err := h.pages.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if err := h.resp.Write(w, r, http.StatusOK, pages); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lang := r.PathValue("lang")

	if id != filepath.Base(id) || id == "" || id == "." ||
		lang != filepath.Base(lang) || lang == "" || lang == "." {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "pages", id, lang+".md"))
	if errors.Is(err, fs.ErrNotExist) {
		h.writeError(w, &NotFoundError{Reason: "page not found"})
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
