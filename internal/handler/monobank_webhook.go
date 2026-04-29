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

// monobankStatusToInvoiceStatus maps a Monobank invoice webhook status onto
// the persisted invoice_status enum. ok=false means the status is purely
// informational (`created`) or unrecognized — callers no-op.
func monobankStatusToInvoiceStatus(s string) (status string, ok bool) {
	switch s {
	case "processing":
		return order.InvoiceStatusProcessing, true
	case "hold":
		return order.InvoiceStatusHold, true
	case "success":
		return order.InvoiceStatusPaid, true
	case "failure", "expired":
		return order.InvoiceStatusFailed, true
	case "reversed":
		return order.InvoiceStatusReversed, true
	}
	return "", false
}

// buildWebhookNote renders a short, human-readable summary of a webhook
// payload for storage in invoice_history.note (and propagated into
// order_history.note when the order's status moves forward).
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

	invStatus, ok := monobankStatusToInvoiceStatus(payload.Status)
	if !ok {
		h.l.Info().
			Str("reference", payload.Reference).
			Str("monobank_status", payload.Status).
			Msg("monobank webhook: informational, no transition")
		w.WriteHeader(http.StatusOK)
		return
	}

	evt := order.InvoiceEvent{
		OrderID:   payload.Reference,
		InvoiceID: payload.InvoiceID,
		Provider:  "monobank",
		Status:    invStatus,
		Note:      buildWebhookNote(payload),
		Payload:   payload.RawBody,
		EventAt:   payload.ModifiedDate,
	}
	if err := h.orders.RecordInvoiceEvent(r.Context(), evt); err != nil {
		if stderrors.Is(err, order.ErrNotFound) {
			h.l.Warn().Str("reference", payload.Reference).Msg("monobank webhook for unknown order")
			w.WriteHeader(http.StatusOK)
			return
		}
		h.l.Error().Err(err).
			Str("reference", payload.Reference).
			Str("invoice_status", invStatus).
			Msg("monobank webhook: record invoice event failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.l.Info().
		Str("reference", payload.Reference).
		Str("invoice_status", invStatus).
		Time("event_at", payload.ModifiedDate).
		Msg("monobank webhook: event recorded")
	w.WriteHeader(http.StatusOK)
}
