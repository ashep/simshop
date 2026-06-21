package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (h *Handler) ServeAsset(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	base := filepath.Join(h.dataDir, "assets")
	full := filepath.Join(base, rel) // filepath.Join runs Clean, collapsing any ../

	// Containment guard: the resolved path must stay within base.
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}

	// Regular files only — no directory listings.
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, full)
}
