package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/ashep/simshop/internal/stats"
)

const defaultStatsDays = 7

type statsService interface {
	Get(ctx context.Context, days int) (*stats.Report, error)
}

// SetStats wires the visit-stats service. It is optional: GET /stats is only
// registered when analytics are configured.
func (h *Handler) SetStats(s statsService) {
	h.stats = s
}

// ServeStats returns the visit report for the last ?days= days (7, 14 or 30;
// the value is validated by the OpenAPI middleware).
func (h *Handler) ServeStats(w http.ResponseWriter, r *http.Request) {
	days := defaultStatsDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			h.writeError(w, &BadRequestError{Reason: "invalid days"})
			return
		}
		days = n
	}

	report, err := h.stats.Get(r.Context(), days)
	if err != nil {
		if errors.Is(err, stats.ErrUpstream) {
			// The message carries the wrapped Cloudflare detail; the client gets a generic 502.
			h.l.Error().Err(err).Msg("stats upstream failed")
			h.writeError(w, &BadGatewayError{})
			return
		}
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, report); err != nil {
		h.l.Error().Err(err).Msg("write stats response failed")
		h.writeError(w, err)
	}
}
