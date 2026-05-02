package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ashep/simshop/internal/order"
)

const (
	maxStatusUpdateNoteLen           = 500
	maxStatusUpdateTrackingNumberLen = 64
)

// allowedOperatorTargets enumerates the status values an operator may set via
// PATCH /orders/{id}/status. Kept in sync with allowedOperatorTransitions in
// internal/order/lifecycle.go (this is the request-layer accept-set; the
// lifecycle rule decides whether a given current→target pair is legal).
var allowedOperatorTargets = map[string]bool{
	"processing":       true,
	"shipped":          true,
	"delivered":        true,
	"refund_requested": true,
	"returned":         true,
	"refunded":         true,
}

type orderStatusUpdateRequest struct {
	Status         string `json:"status"`
	TrackingNumber string `json:"tracking_number"`
	Note           string `json:"note"`
}

// UpdateOrderStatus applies an operator-driven status transition to an order.
// Authentication is enforced by APIKeyMiddleware in front of this handler.
func (h *Handler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req orderStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, &BadRequestError{Reason: "invalid request body"})
		return
	}
	if !allowedOperatorTargets[req.Status] {
		h.writeError(w, &BadRequestError{Reason: "invalid status"})
		return
	}
	if len(req.Note) > maxStatusUpdateNoteLen {
		h.writeError(w, &BadRequestError{Reason: "note too long"})
		return
	}
	if len(req.TrackingNumber) > maxStatusUpdateTrackingNumberLen {
		h.writeError(w, &BadRequestError{Reason: "tracking_number too long"})
		return
	}
	tracking := strings.TrimSpace(req.TrackingNumber)
	if req.Status == "shipped" && tracking == "" {
		h.writeError(w, &BadRequestError{Reason: "tracking_number required"})
		return
	}
	if req.Status != "shipped" && tracking != "" {
		h.writeError(w, &BadRequestError{Reason: "tracking_number only valid for shipped"})
		return
	}

	_, err := h.orders.UpdateStatus(r.Context(), id, req.Status, req.Note, tracking)
	if errors.Is(err, order.ErrNotFound) {
		h.writeError(w, &NotFoundError{Reason: "order not found"})
		return
	}
	if errors.Is(err, order.ErrTransitionNotAllowed) {
		h.writeError(w, &ConflictError{Reason: "transition not allowed"})
		return
	}
	if err != nil {
		h.l.Error().Err(err).Str("order_id", id).Str("target", req.Status).Msg("update order status failed")
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, &orderStatusResponse{Status: req.Status}); err != nil {
		h.l.Error().Err(err).Msg("write order status response failed")
		h.writeError(w, err)
	}
}
