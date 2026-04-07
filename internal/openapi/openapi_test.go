package openapi_test

import (
	"testing"
	"testing/fstest"

	"github.com/ashep/simshop/internal/openapi"
)

const testSpec = `
openapi: "3.0.3"
info:
  title: Test API
  version: "0.1.0"
paths:
  /product:
    post:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - name
              properties:
                id:
                  type: string
                  format: uuid
                name:
                  type: string
                  minLength: 1
      responses:
        "200":
          description: OK
        "201":
          description: Created
          content:
            application/json:
              schema:
                type: object
                required:
                  - id
                  - name
                properties:
                  id:
                    type: string
                    format: uuid
                  name:
                    type: string
`

func buildOpenAPI(t *testing.T) *openapi.OpenAPI {
	t.Helper()
	specFS := fstest.MapFS{"root.yaml": {Data: []byte(testSpec)}}
	oas, err := openapi.New(specFS)
	if err != nil {
		t.Fatalf("openapi.New: %v", err)
	}
	return oas
}
