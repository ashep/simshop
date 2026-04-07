package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

func init() {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC4122))
}

// OpenAPI holds a parsed and validated OpenAPI spec and exposes middleware
// and response helpers that share the same router instance.
type OpenAPI struct {
	router routers.Router
}

// New parses and validates specData, returning an OpenAPI ready to produce
// middleware and responders. Returns an error if the spec is invalid.
func New(specFiles fs.FS) (*OpenAPI, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		return fs.ReadFile(specFiles, url.Path)
	}

	data, err := fs.ReadFile(specFiles, "root.yaml")
	if err != nil {
		return nil, fmt.Errorf("read root.yaml: %w", err)
	}

	spec, err := loader.LoadFromDataWithPath(data, &url.URL{Path: "root.yaml"})
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	if err = spec.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	return &OpenAPI{router: router}, nil
}

type detail struct {
	Field string `json:"field,omitempty"`
	Error string `json:"error"`
}

func writeRawError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	b, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(b)
}

func writeValidationError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	b, _ := json.Marshal(struct {
		Error   string   `json:"error"`
		Details []detail `json:"details"`
	}{
		Error:   "bad request",
		Details: extractDetails(err),
	})
	_, _ = w.Write(b)
}

func extractDetails(err error) []detail {
	var me openapi3.MultiError
	if errors.As(err, &me) {
		details := make([]detail, 0, len(me))
		for _, e := range me {
			details = append(details, extractDetail(e))
		}
		return details
	}
	return []detail{extractDetail(err)}
}

func extractDetail(err error) detail {
	var re *openapi3filter.RequestError
	if !errors.As(err, &re) {
		return detail{Error: "bad request"}
	}

	var se *openapi3.SchemaError
	if errors.As(re.Err, &se) {
		return detail{
			Field: strings.Join(se.JSONPointer(), "."),
			Error: se.Reason,
		}
	}

	if re.Parameter != nil {
		return detail{Field: re.Parameter.Name, Error: re.Reason}
	}

	return detail{Error: re.Reason}
}
