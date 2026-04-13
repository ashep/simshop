package openapi

import (
	"bytes"
	"io"
	"mime"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// Middleware returns an HTTP middleware that validates every incoming request
// against the spec. Routes not defined in the spec pass through without validation.
func (o *OpenAPI) Middleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := o.router.FindRoute(r)
			if err != nil {
				// Route not in spec — pass through.
				next(w, r)
				return
			}

			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			isMultipart := mt == "multipart/form-data"

			const maxBodyBytes = 1 << 20 // 1 MiB
			var body []byte
			if r.Body != nil && !isMultipart {
				r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
				body, err = io.ReadAll(r.Body)
				if err != nil {
					writeRawError(w, "failed to read request body")
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}

			input := &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options: &openapi3filter.Options{
					AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
					ExcludeRequestBody: isMultipart,
				},
			}

			if err = openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				writeValidationError(w, err)
				return
			}

			// Restore body for the handler.
			if body != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			next(w, r)
		}
	}
}
