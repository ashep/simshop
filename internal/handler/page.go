package handler

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

func (h *Handler) ListPages(w http.ResponseWriter, r *http.Request) {
	pagesDir := filepath.Join(h.dataDir, "pages")

	entries, err := os.ReadDir(pagesDir)
	if errors.Is(err, fs.ErrNotExist) {
		if wErr := h.resp.Write(w, r, http.StatusOK, []string{}); wErr != nil {
			h.l.Error().Err(wErr).Msg("response validation failed")
		}
		return
	}
	if err != nil {
		h.writeError(w, fmt.Errorf("read pages dir: %w", err))
		return
	}

	ids := []string{}
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}

	if err := h.resp.Write(w, r, http.StatusOK, ids); err != nil {
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
