package handler

import (
	"context"
	"net/http"

	"github.com/ashep/simshop/internal/novaposhta"
)

type novaPoshtaClient interface {
	SearchCities(ctx context.Context, query string) ([]novaposhta.City, error)
	SearchBranches(ctx context.Context, cityRef, query string) ([]novaposhta.Branch, error)
	SearchStreets(ctx context.Context, cityRef, query string) ([]novaposhta.Street, error)
}

func (h *Handler) SearchNPCities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		h.writeError(w, &BadRequestError{Reason: "q is required"})
		return
	}

	cities, err := h.np.SearchCities(r.Context(), q)
	if err != nil {
		h.l.Error().Err(err).Str("q", q).Msg("search cities failed")
		h.writeError(w, &BadGatewayError{Reason: "nova poshta api error"})
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, cities); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) SearchNPStreets(w http.ResponseWriter, r *http.Request) {
	cityRef := r.URL.Query().Get("city_ref")
	if cityRef == "" {
		h.writeError(w, &BadRequestError{Reason: "city_ref is required"})
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		h.writeError(w, &BadRequestError{Reason: "q is required"})
		return
	}

	streets, err := h.np.SearchStreets(r.Context(), cityRef, q)
	if err != nil {
		h.l.Error().Err(err).Msg("search streets failed")
		h.writeError(w, &BadGatewayError{Reason: "nova poshta api error"})
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, streets); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) SearchNPBranches(w http.ResponseWriter, r *http.Request) {
	cityRef := r.URL.Query().Get("city_ref")
	if cityRef == "" {
		h.writeError(w, &BadRequestError{Reason: "city_ref is required"})
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		h.writeError(w, &BadRequestError{Reason: "q is required"})
		return
	}

	branches, err := h.np.SearchBranches(r.Context(), cityRef, q)
	if err != nil {
		h.l.Error().Err(err).Msg("search branches failed")
		h.writeError(w, &BadGatewayError{Reason: "nova poshta api error"})
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, branches); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
