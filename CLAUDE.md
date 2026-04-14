# Project Rules

## Meta

After completing any task, reflect on what was learned. If a decision, pattern, or constraint is non-obvious and likely
to recur, add it to this file under the appropriate section. Do not record things already derivable from reading the
code.

**This is mandatory, not optional.** Do not wait for the user to ask. Update this file as the final step of every task,
before responding with a summary.

After implementing a new feature or changing an existing one, update `README.md` if the change affects business logic
worth describing (new entities, new concepts, new endpoints, changed behavior). This is mandatory — do not skip it.
When editing `README.md`, always wrap lines at 120 characters.

Never run `git commit` or `git push` unless the user explicitly asks. When dispatching subagents, include an explicit
"do not commit" instruction at the top of every subagent prompt.

## Project overview

`simshop` is a Go HTTP API service (`github.com/ashep/simshop`). It uses:

- **Filesystem catalog** — products are loaded from YAML files at startup (no database)
- **zerolog** for structured logging
- **go-app** framework (`github.com/ashep/go-app`) for app lifecycle/config
- **OpenAPI** spec in `api/` for request validation

Key directories:

- `internal/` — application code (app, handler, product, loader, openapi)
- `api/` — OpenAPI spec files
- `tests/` — functional tests (build tag: `functest`)
- `vendor/` — vendored dependencies

Config is loaded from `config.yml` and environment variables. Data is read from `data_dir` (default `./data`).

## Architecture

Request processing middleware chain (innermost to outermost): `content-type → openapi validation → handler`.

Routes are registered with Go 1.22+ stdlib pattern syntax: `"METHOD /path"` (e.g., `"GET /products"`).

Handlers use `BadRequestError` (defined in `internal/handler/handler.go`) to map domain errors to HTTP responses via
`h.writeError(w, err)`. Always populate its `Reason string` field with a human-readable message
(e.g., `&BadRequestError{Reason: "invalid language code"}`). The reason is written directly into the JSON response body
as `{"error": "<reason>"}`, so it is client-visible.

Domain errors are defined in the service package. The handler maps them to HTTP error types using `errors.Is`.

Always use the most semantically appropriate HTTP status code. Examples: 404 Not Found for missing resource — not a
generic 400 Bad Request.

### CORS middleware

`handler.CORSMiddleware(allowedOrigins []string)` returns a middleware that sets `Access-Control-Allow-Origin` when the
request `Origin` matches an entry in `allowedOrigins`. A `"*"` entry allows all origins (echoes `*` back). OPTIONS
preflight requests (detected by `Access-Control-Request-Method` header) are intercepted and responded to with 204
before calling `next`. Configure via `server.cors_origins` in `config.yml`.

Explicit `OPTIONS` routes must be registered alongside each `GET` route in `app.go` because Go 1.22+ stdlib mux does
not implicitly handle OPTIONS for registered methods.

### Circular import: handler ↔ app

`internal/app` imports `internal/handler`; therefore `internal/handler` must never import `internal/app`. When the
handler or service needs config values from the `app.Config` struct, pass the scalar values at construction time
instead of passing the config struct.

## Implementing features

When asked to implement a new feature, use `api/` package content as additional context to understand the requirements.

### Image path transformation in loader

After `validate`, `loadProduct` rewrites each non-empty `Images[i].Preview` and `Images[i].Full` from a bare filename
(e.g. `"thumb.jpg"`) to a URL-ready path (e.g. `"/images/{id}/thumb.jpg"`). Validation runs against the bare
filenames first (so `os.Stat` works), then the transformation happens so the JSON response contains downloadable
paths without any handler-level mapping.

### Image serving route

`GET /images/{product_id}/{file_name}` is served by `handler.ServeImage`. It reads from
`{data_dir}/products/{product_id}/images/{file_name}` using `http.ServeFile`. The handler applies `filepath.Base`
to both path values before joining to prevent path traversal. `handler.NewHandler` takes `dataDir string` as its third
parameter (before `zerolog.Logger`) so the handler can resolve image paths without importing `internal/app`.

Functional tests for image serving must create a complete valid product directory (including `product.yaml`) alongside
the `images/` subdirectory if the test needs the product to appear in `catalog.Products`. If only the image file itself
is needed (e.g., the file is in `{data_dir}/products/{id}/images/`), no `product.yaml` is required — the loader
silently skips directories that lack one.

### Products routes

`GET /products` is served by `handler.ListProducts`. Product metadata is loaded at startup from
`{data_dir}/products/products.yaml` (via `loader.loadProductsList`) into `catalog.ProductItems` and wrapped in
`product.NewService`. The handler delegates to `h.products.List()` and returns a JSON array of
`[{id, title, description, image?}]` objects. The optional `image` field is the URL path of the first preview image
from the product's `product.yaml` (`product.Item.Image *string`). It is populated by `joinProductImages` in `Load()`
after both `loadProducts` and `loadProductsList` have run — it looks up the matching `Product` by ID in `Catalog.Products`
and copies the first `Images[0].Preview` path (already rewritten to `/images/{id}/{file}`). `image` is omitted from the
JSON when nil (`json:"image,omitempty"`). A missing `products.yaml` returns an empty array — not an error. The route
goes through the OpenAPI response middleware.

`GET /products/{id}/{lang}` is served by `handler.ServeProductContent`. It reads
`{data_dir}/products/{id}/product.yaml`, parses the full `product.Product` struct, checks that the requested
language exists in `p.Name`, then builds a lang-filtered `product.ProductDetail` (multilingual maps collapsed to
single strings for the requested language) and returns it as JSON. Both path values are validated with the reject
pattern: `if value != filepath.Base(value) || value == "" || value == "."`. The route goes through the OpenAPI
response middleware. Missing product directory or `product.yaml` → 404. Missing language key → 404.

The existing `loadProducts` (per-directory loader) continues to run at startup for validation purposes; its output
(`Catalog.Products`) is no longer wired to any handler — it exists solely to enforce product YAML integrity at
startup. Product subdirectories without `product.yaml` are silently skipped by `loadProducts`.

### Pages routes

`GET /pages` is served by `handler.ListPages`. Page metadata is loaded at startup from
`{data_dir}/pages/pages.yaml` (via `loader.loadPages`) into `catalog.Pages` and wrapped in
`page.NewService`. The handler delegates to `h.pages.List()` and returns a JSON array of
`[{id, title}]` objects. A missing `pages.yaml` returns an empty array — not an error. The route
goes through the OpenAPI response middleware.

`GET /pages/{id}/{lang}` is served by `handler.ServePage`. It reads
`{data_dir}/pages/{id}/{lang}.md` and returns the file content as `text/plain; charset=utf-8`. Both path
values are validated with the reject pattern: `if value != filepath.Base(value) || value == "" || value == "."`.
The route does NOT go through the OpenAPI response middleware (plain text, not JSON), but is declared in the
spec for documentation. Missing file → 404.

### Loader: `products.yaml` product list

`{data_dir}/products/products.yaml` holds a flat list of `product.Item` entries (id, title, description). It is loaded
by `loadProductsList` into `catalog.ProductItems`. A missing file returns an empty slice — not an error. This is a
separate concept from the per-product directory loading done by `loadProducts`; `ProductItems` and `Products` coexist
in `Catalog`.

### Shop route

`GET /shop` is served by `handler.ServeShop`. It delegates to `h.shop.Get(ctx)` and returns the `*shop.Shop` struct as
JSON via `h.resp.Write`. The `shopService` interface is defined in `internal/handler/shop.go`. The route goes through
the OpenAPI response middleware. `NewHandler` accepts `shopSvc shopService` as its third parameter (after `pages`,
before `resp`).

### Loader: `shop.yaml` shop settings

`{data_dir}/shop.yaml` holds a single `shop:` key mapping to `shop.Shop` (name, title, description — each a
`map[string]string`). It is loaded by `loadShop` into `catalog.Shop`. A missing file results in an empty `&shop.Shop{}`
— not an error. A malformed YAML file is fatal. The wrapper struct `shopFile` is needed because the YAML root key is
`shop:`, not the struct fields directly.

### Loader: missing products directory is not an error

A missing `products/` subdirectory under `data_dir` is not an error — results in an empty catalog. A malformed YAML
file or a validation error is fatal. Validation runs inside `loadProduct` immediately after YAML parsing.

Product subdirectories that do not contain a `product.yaml` file are silently skipped by `loadProducts`. This allows
extra directories to coexist in the `products/` tree without causing a startup failure.

### Product validation rules (enforced at startup)

Validation lives in `internal/loader/loader.go:validate`. Rules are fatal at startup:
1. `name` must be non-empty.
2. `description` must be non-empty.
3. Language sets of `name` and `description` must be identical.
4. Every spec entry must cover exactly the languages in `name`.
5. `price` must contain a `"default"` key.
6. Every attr entry must cover exactly the languages in `name`, and each language entry must have ≥ 1 value.
7. Every image `preview` and `full` path must exist on disk relative to `{productDir}/images/` (checked with `os.Stat`;
   uses `errors.Is(err, fs.ErrNotExist)`).

## Tests

### Unit tests

Placed alongside source code in each package. Run with:

```
task go:test:unit -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g., `-run TestName`, `-v`).

In git worktrees, `task go:test:unit` may not be available if `.ci/` is not initialized. In that case, use `go test`
directly (e.g., `go test -run TestFoo -v ./internal/...`).

### Functional tests

Placed in the `tests/` directory. All files use the `//go:build functest` build tag. Run with:

```
task go:test:func -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g., `-run TestName`, `-v`).

In worktrees (where `.ci/` is not initialized), run directly:

```
go test -tags=functest ./tests/...
```

### Rules

- Before implementing any feature or fix, invoke the `superpowers:test-driven-development` skill.
- Before claiming any work is done, invoke the `superpowers:verification-before-completion` skill and run **both**
  `task go:test:unit -- ./...` and `task go:test:func -- -v`. Both suites must pass — running only one is not sufficient.
- After all changes are made and tests pass, run `task go:golangci-lint`. All lint checks must pass before the task is
  considered complete.
- Do not consider a task complete until tests pass. Do not respond with a summary of changes before running tests.
- Group related tests under a single parent function `TestFoo(main *testing.T)` and use `main.Run("CaseName", ...)` for
  sub-tests. Never write separate top-level functions like `TestFoo_CaseName`.
- When adding a new file to a package that already has `_test.go` companions alongside source files (e.g.
  `internal/handler/`), write the unit test file as part of the same task. Do not wait to be reminded.

### OpenAPI spec

The OpenAPI validator (`kin-openapi`) operates in OpenAPI 3.0 compatibility mode. Do **not** use OpenAPI 3.1 array type
syntax (`type: ["string", "null"]`) — it will cause `unsupported 'type' value "null"` at startup. Use the 3.0 style
instead: `type: string` + `nullable: true`.

For path parameters that hold UUID values, always write `type: string` + `format: uuid`, never `type: uuid`. Using
`type: uuid` causes an `unsupported 'type' value "uuid"` panic at startup.

### Shared test helpers in handler package

Keep shared test helpers in `handler_test.go`, not in per-feature test files, so they survive feature removal.

### Handler unit tests: response ID must be a valid UUID

When a handler response schema declares `id` with `format: uuid`, mock return values in handler unit tests must use
valid UUID strings (e.g., `"018f4e3a-0000-7000-8000-000000000099"`). The OpenAPI response validator (`resp.Write`)
rejects non-UUID strings and the handler returns HTTP 500, masking the actual assertion under test.

### service constructor: nil slice normalization

`product.NewService(items []*Item)` normalizes nil to `[]*Item{}` inside the constructor — required so `List`
returns an empty JSON array `[]` rather than `null`, which would fail OpenAPI response validation.

### Functional test pattern

Functional tests use YAML fixture files:

**Loader unit tests (`internal/loader/`):**
- `makeProductDir(t, dataDir, id, yaml, extraFiles)` creates `{dataDir}/products/{id}/product.yaml` and any extra
  files specified as a `map[string][]byte` of relative path → content pairs.
- `makeProductsFile(t, dataDir, content)` creates `{dataDir}/products/products.yaml` with the given content.

**API functional tests (`tests/api/product/`):**
- `makeDataDir(t, productsYAML, productYAMLs)` creates a temp data dir, writes `products/products.yaml` (if non-empty
  string), and writes per-product `product.yaml` files given as `map[string]string` of `{id: yaml-content}`.
- `testapp.New(t, dataDir)` always takes a `dataDir` argument.
- Subtests that need a completely separate empty catalog start their own `testapp` inside the subtest body — safe
  because each app binds to a random port.

### API functional tests (`tests/api/`)

- All API subtests within a `TestFoo` share one `testapp` instance started in the parent function. Starting a separate
  instance per subtest panics on port conflict when subtests run in parallel.
- At most one top-level `TestFoo` function per package may call `main.Parallel()` if each starts its own `testapp`.
  New API test functions that start their own app must NOT call `main.Parallel()` unless existing parallel tests are
  refactored to share a single app.

### Write tool prerequisite

The Write tool requires that the file has been Read at least once in the session before overwriting. When writing many
files in a single pass, Read each target file before calling Write — even for files that will be completely replaced.
