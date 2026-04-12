package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/property"
)

type propertyService interface {
	Create(ctx context.Context, req property.CreateRequest) (*property.Property, error)
}

func (h *Handler) PropertyCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil || !user.IsAdmin() {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	req := property.CreateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}

	p, err := h.prop.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, property.ErrInvalidLanguage):
			h.writeError(w, &BadRequestError{Reason: "invalid language code"})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("property_id", p.ID).Str("user_id", user.ID).Msg("property created")

	if err := h.resp.Write(w, r, http.StatusCreated, &property.CreateResponse{ID: p.ID}); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}
