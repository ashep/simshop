package handler

import (
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/order"
)

type orderStatusResponse struct {
	Status string `json:"status"`
}

// GetOrderStatus returns the current status of an order. The {id} path
// parameter is validated as a UUID by the OpenAPI request-validation
// middleware before this handler runs.
func (h *Handler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	status, err := h.orders.GetStatus(r.Context(), id)
	if errors.Is(err, order.ErrNotFound) {
		h.writeError(w, &NotFoundError{Reason: "order not found"})
		return
	}
	if err != nil {
		h.l.Error().Err(err).Str("order_id", id).Msg("get order status failed")
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, &orderStatusResponse{Status: status}); err != nil {
		h.l.Error().Err(err).Msg("write order status response failed")
		h.writeError(w, err)
	}
}
