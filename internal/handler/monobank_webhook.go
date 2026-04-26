package handler

import (
	stderrors "errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/order"
)

const webhookMaxBodySize = 1 << 20 // 1 MB, mirrors monobank.Client

// monobankStatusToOrderStatus maps a Monobank invoice status to the
// order_status this server should transition to. ok=false means the status
// is informational only (`created`) or not recognized — callers no-op and log.
func monobankStatusToOrderStatus(s string) (status string, ok bool) {
	switch s {
	case "processing":
		return "payment_processing", true
	case "hold":
		return "payment_hold", true
	case "success":
		return "paid", true
	case "failure", "expired":
		return "cancelled", true
	case "reversed":
		return "refunded", true
	}
	return "", false
}

// orderStatusRank assigns a monotonic rank to each order_status so the
// webhook handler can reject backwards transitions. cancelled and the
// fulfillment processing status share rank 5 because they are parallel
// branches off paid/awaiting_payment — a late `failure` after the operator
// has started fulfillment must NOT downgrade the order.
var orderStatusRank = map[string]int{
	"new":                0,
	"awaiting_payment":   1,
	"payment_processing": 2,
	"payment_hold":       3,
	"paid":               4,
	"processing":         5,
	"cancelled":          5,
	"shipped":            6,
	"delivered":          7,
	"refund_requested":   8,
	"returned":           9,
	"refunded":           10,
}

// shouldApply reports whether target is strictly ahead of current in the
// payment lifecycle. Equal rank → idempotent no-op.
func shouldApply(current, target string) bool {
	return orderStatusRank[target] > orderStatusRank[current]
}

// buildWebhookNote renders a short, human-readable summary of a webhook
// payload for storage in order_history.note.
func buildWebhookNote(p *monobank.WebhookPayload) string {
	switch p.Status {
	case "success":
		return fmt.Sprintf("monobank: success, finalAmount=%d", p.FinalAmount)
	case "failure":
		detail := p.ErrCode
		if detail == "" {
			detail = p.FailureReason
		}
		if detail == "" {
			return "monobank: failure"
		}
		return fmt.Sprintf("monobank: failure (%s)", detail)
	default:
		return "monobank: " + p.Status
	}
}

// MonobankWebhook processes Monobank invoice-status webhook deliveries.
// Authenticated via the X-Sign header. Returns 200 on processed, idempotent,
// informational, or unknown-reference outcomes (all permanent — Monobank
// stops retrying); 401 on invalid signature; 400 on malformed body; 500 on
// transient DB errors so Monobank retries.
func (h *Handler) MonobankWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookMaxBodySize))
	if err != nil {
		h.l.Warn().Err(err).Msg("monobank webhook: read body failed")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Sign")
	if vErr := h.mbVerifier.Verify(r.Context(), body, sig); vErr != nil {
		if stderrors.Is(vErr, monobank.ErrInvalidSignature) {
			h.l.Warn().Str("client_ip", rateLimitClientIP(r)).Msg("monobank webhook: invalid signature")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.l.Error().Err(vErr).Msg("monobank webhook: verifier transport error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	payload, err := monobank.ParseWebhook(body)
	if err != nil {
		h.l.Warn().Err(err).Msg("monobank webhook: parse body failed")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	current, err := h.orders.GetStatus(r.Context(), payload.Reference)
	if err != nil {
		if stderrors.Is(err, order.ErrNotFound) {
			h.l.Warn().Str("reference", payload.Reference).Msg("monobank webhook for unknown order")
			w.WriteHeader(http.StatusOK)
			return
		}
		h.l.Error().Err(err).Str("reference", payload.Reference).Msg("monobank webhook: get status failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	target, ok := monobankStatusToOrderStatus(payload.Status)
	if !ok {
		h.l.Info().
			Str("reference", payload.Reference).
			Str("monobank_status", payload.Status).
			Msg("monobank webhook: informational, no transition")
		w.WriteHeader(http.StatusOK)
		return
	}

	if !shouldApply(current, target) {
		h.l.Info().
			Str("reference", payload.Reference).
			Str("current", current).
			Str("target", target).
			Msg("monobank webhook: idempotent or backwards, no transition")
		w.WriteHeader(http.StatusOK)
		return
	}

	note := buildWebhookNote(payload)
	if err := h.orders.ApplyPaymentEvent(r.Context(), payload.Reference, target, note, payload.RawBody); err != nil {
		h.l.Error().Err(err).
			Str("reference", payload.Reference).
			Str("target", target).
			Msg("monobank webhook: apply payment event failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.l.Info().
		Str("reference", payload.Reference).
		Str("from", current).
		Str("to", target).
		Msg("monobank webhook: transition applied")
	w.WriteHeader(http.StatusOK)
}
