package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// Responder validates and writes JSON responses against the OpenAPI spec.
type Responder struct {
	router routers.Router
}

// Responder returns a Responder that validates responses against the spec.
func (o *OpenAPI) Responder() *Responder {
	return &Responder{router: o.router}
}

// Write marshals v to JSON, validates the body and status against the spec for
// the request's route, then writes the response. If v is nil the body is empty
// and body validation is skipped. Returns an error and writes 500 if the route
// is not found or the response fails validation.
func (r *Responder) Write(w http.ResponseWriter, req *http.Request, status int, v any) error {
	route, pathParams, err := r.router.FindRoute(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("find route: %w", err)
	}

	reqInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}

	respInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqInput,
		Status:                 status,
		Header:                 http.Header{},
		Options: &openapi3filter.Options{
			AuthenticationFunc:    openapi3filter.NoopAuthenticationFunc,
			IncludeResponseStatus: true,
		},
	}

	var body []byte
	if v != nil {
		body, err = json.Marshal(v)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return fmt.Errorf("marshal response: %w", err)
		}
		respInput.Header.Set("Content-Type", "application/json")
		respInput.Body = io.NopCloser(bytes.NewReader(body))
	}

	if err = openapi3filter.ValidateResponse(req.Context(), respInput); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("validate response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_, _ = w.Write(body)
	}

	return nil
}
