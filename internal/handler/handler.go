package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ashep/simshop/internal/openapi"
	"github.com/rs/zerolog"
)

type BadRequestError struct{}

func (e *BadRequestError) Error() string {
	return "bad request"
}

type Handler struct {
	shop shopService
	prod productSservice
	resp *openapi.Responder
	l    zerolog.Logger
}

func NewHandler(shop shopService, prod productSservice, resp *openapi.Responder, l zerolog.Logger) *Handler {
	return &Handler{
		shop: shop,
		prod: prod,
		resp: resp,
		l:    l,
	}
}

func (h *Handler) unmarshal(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	if err := d.Decode(v); err != nil {
		h.l.Warn().Err(err).Msg("request body could not be decoded")
		return &BadRequestError{}
	}
	return nil
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

	h.l.Error().Err(err).Send()
	w.WriteHeader(http.StatusInternalServerError)
}
