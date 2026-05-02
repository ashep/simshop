package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

type BadGatewayError struct {
	Reason string
}

func (e *BadGatewayError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "bad gateway"
}

type UnauthorizedError struct {
	Reason string
}

func (e *UnauthorizedError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "unauthorized"
}

type ConflictError struct {
	Reason string
}

func (e *ConflictError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "conflict"
}

type geoDetector interface {
	Detect(r *http.Request) string
}

type monobankVerifier interface {
	Verify(ctx context.Context, body []byte, signatureB64 string) error
}

type Handler struct {
	prod        productService
	pages       pageService
	shop        shopService
	np          novaPoshtaClient
	monobank    monobankClient
	mbVerifier  monobankVerifier
	orders      orderService
	geo         geoDetector
	resp        *openapi.Responder
	dataDir     string
	redirectURL string
	webhookURL  string
	publicURL   string
	taxIDs      []int
	l           zerolog.Logger
}

func NewHandler(
	prod productService,
	pages pageService,
	shopSvc shopService,
	np novaPoshtaClient,
	mb monobankClient,
	mbVerifier monobankVerifier,
	orders orderService,
	geo geoDetector,
	resp *openapi.Responder,
	dataDir string,
	redirectURL string,
	publicURL string,
	taxIDs []int,
	l zerolog.Logger,
) *Handler {
	publicURL = strings.TrimRight(publicURL, "/")
	return &Handler{
		prod:        prod,
		pages:       pages,
		shop:        shopSvc,
		np:          np,
		monobank:    mb,
		mbVerifier:  mbVerifier,
		orders:      orders,
		geo:         geo,
		resp:        resp,
		dataDir:     dataDir,
		redirectURL: redirectURL,
		webhookURL:  publicURL + "/monobank/webhook",
		publicURL:   publicURL,
		taxIDs:      taxIDs,
		l:           l,
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

	if tErr, ok := errors.AsType[*BadGatewayError](err); tErr != nil && ok {
		w.WriteHeader(http.StatusBadGateway)
		if _, wErr := fmt.Fprintf(w, `{"error": %q}`, tErr.Error()); wErr != nil {
			h.l.Warn().Err(wErr).Msg("error response write failed")
		}
		return
	}

	if tErr, ok := errors.AsType[*UnauthorizedError](err); tErr != nil && ok {
		w.WriteHeader(http.StatusUnauthorized)
		if _, wErr := fmt.Fprintf(w, `{"error": %q}`, tErr.Error()); wErr != nil {
			h.l.Warn().Err(wErr).Msg("error response write failed")
		}
		return
	}

	if tErr, ok := errors.AsType[*ConflictError](err); tErr != nil && ok {
		w.WriteHeader(http.StatusConflict)
		if _, wErr := fmt.Fprintf(w, `{"error": %q}`, tErr.Error()); wErr != nil {
			h.l.Warn().Err(wErr).Msg("error response write failed")
		}
		return
	}

	h.l.Error().Err(err).Send()
	w.WriteHeader(http.StatusInternalServerError)
}
