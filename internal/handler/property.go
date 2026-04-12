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
	List(ctx context.Context) ([]property.Property, error)
	Update(ctx context.Context, id string, req property.UpdateRequest) error
}

func (h *Handler) ListProperties(w http.ResponseWriter, r *http.Request) {
	props, err := h.prop.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	if err := h.resp.Write(w, r, http.StatusOK, props); err != nil {
		h.l.Error().Err(err).Msg("response validation failed")
	}
}

func (h *Handler) CreateProperty(w http.ResponseWriter, r *http.Request) {
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
	req.Trim()

	p, err := h.prop.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, property.ErrMissingEnTitle):
			h.writeError(w, &BadRequestError{Reason: "EN title is required"})
		case errors.Is(err, property.ErrInvalidLanguage):
			h.writeError(w, &BadRequestError{Reason: "invalid language code"})
		case errors.Is(err, property.ErrDuplicateTitle):
			h.writeError(w, &ConflictError{Reason: "title is already taken"})
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

func (h *Handler) UpdateProperty(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil || !user.IsAdmin() {
		h.writeError(w, &PermissionDeniedError{})
		return
	}

	id := r.PathValue("id")

	req := property.UpdateRequest{}
	if err := h.unmarshal(r.Body, &req); err != nil {
		h.writeError(w, err)
		return
	}
	req.Trim()

	if err := h.prop.Update(r.Context(), id, req); err != nil {
		switch {
		case errors.Is(err, property.ErrMissingEnTitle):
			h.writeError(w, &BadRequestError{Reason: "EN title is required"})
		case errors.Is(err, property.ErrPropertyNotFound):
			h.writeError(w, &NotFoundError{Reason: "property not found"})
		case errors.Is(err, property.ErrInvalidLanguage):
			h.writeError(w, &BadRequestError{Reason: "invalid language code"})
		default:
			h.writeError(w, err)
		}
		return
	}

	h.l.Info().Str("property_id", id).Str("user_id", user.ID).Msg("property updated")

	w.WriteHeader(http.StatusOK)
}
