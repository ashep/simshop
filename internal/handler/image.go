package handler

import (
	"net/http"
	"path/filepath"
)

func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	productID := filepath.Base(r.PathValue("product_id"))
	fileName := filepath.Base(r.PathValue("file_name"))

	if productID == "" || productID == "." || fileName == "" || fileName == "." {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, filepath.Join(h.dataDir, "products", productID, "images", fileName))
}
