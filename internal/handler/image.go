package handler

import (
	"net/http"
	"path/filepath"
)

func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("product_id")
	fileName := r.PathValue("file_name")

	if productID != filepath.Base(productID) || productID == "" || productID == "." ||
		fileName != filepath.Base(fileName) || fileName == "" || fileName == "." {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, filepath.Join(h.dataDir, "products", productID, "images", fileName))
}
