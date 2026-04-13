package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ashep/simshop/internal/openapi"
	"github.com/rs/zerolog"
)

type BadRequestError struct {
	Reason string
}

func (e *BadRequestError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "bad request"
}

type NotFoundError struct {
	Reason string
}

func (e *NotFoundError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "not found"
}

type Handler struct {
	prod productService
	prop propertyService
	file fileService
	resp *openapi.Responder
	l    zerolog.Logger
}

func NewHandler(
	prod productService,
	prop propertyService,
	file fileService,
	resp *openapi.Responder,
	l zerolog.Logger,
) *Handler {
	return &Handler{
		prod: prod,
		prop: prop,
		file: file,
		resp: resp,
		l:    l,
	}
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	if tErr, ok := errors.AsType[*BadRequestError](err); tErr != nil && ok {
		w.WriteHeader(http.StatusBadRequest)
		if _, wErr := fmt.Fprintf(w, `{"error": %q}`, tErr.Error()); wErr != nil {
			h.l.Warn().Err(wErr).Msg("error response write failed")
		}
		return
	}

	if tErr, ok := errors.AsType[*NotFoundError](err); tErr != nil && ok {
		w.WriteHeader(http.StatusNotFound)
		if _, wErr := fmt.Fprintf(w, `{"error": %q}`, tErr.Error()); wErr != nil {
			h.l.Warn().Err(wErr).Msg("error response write failed")
		}
		return
	}

	h.l.Error().Err(err).Send()
	w.WriteHeader(http.StatusInternalServerError)
}
