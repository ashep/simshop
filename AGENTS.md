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

Do not run `docker compose` directly — the `task` commands manage containers automatically.

## Architecture

Request processing middleware chain (innermost to outermost): `content-type → openapi validation → handler`.

Each request struct exposes a `Trim()` method that strips leading/trailing whitespace from all string fields (including map keys and values). Handlers call `req.Trim()` immediately after `h.unmarshal()`, before any validation or service call.

Routes are registered with Go 1.22+ stdlib pattern syntax: `"METHOD /path"` (e.g., `"POST /shops"`).

Handlers use `BadRequestError`, `ConflictError`, and `PermissionDeniedError` (defined in `internal/handler/handler.go`)
to map domain errors to HTTP responses via `h.writeError(w, err)`.

`BadRequestError` and `ConflictError` each have a `Reason string` field. Always populate it with a human-readable
message (e.g., `&BadRequestError{Reason: "invalid language code"}`). The reason is written directly into the JSON
response body as `{"error": "<reason>"}`, so it is client-visible.

Domain errors (e.g., `ErrShopAlreadyExists`) are defined in the service package. The handler maps them to the
appropriate HTTP error type using `errors.Is`.

Always use the most semantically appropriate HTTP status code. Examples: 409 Conflict for duplicate resource, 404 Not
Found for missing resource — not a generic 400 Bad Request.

### OpenAPI middleware and multipart/form-data

The OpenAPI middleware (`internal/openapi/middleware.go`) reads and buffers the request body (up to 1 MiB) to restore
it for the handler after validation. For `multipart/form-data` requests it skips body reading and sets
`ExcludeRequestBody: true` — this is intentional: binary file content cannot be meaningfully validated by kin-openapi,
and the 1 MiB cap is too small for file upload endpoints. The handler applies its own `http.MaxBytesReader` limit.

### File upload handler pattern

File uploads use `multipart/form-data` with field name `file`. Handler flow:
1. Set `http.MaxBytesReader(w, r.Body, maxSize+1024)` before `ParseMultipartForm` to enforce the byte limit at the
   network layer (`+1024` accounts for multipart boundary/header overhead).
2. Call `r.ParseMultipartForm(32 << 20)` — the argument controls the in-memory buffer, not the total size cap.
3. Get the file with `r.FormFile("file")` and immediately check `fh.Size > maxSize` **before** reading any other
   fields (e.g., `name`). This ensures size rejection takes priority over field validation.
4. Detect MIME type from the first 512 bytes using `http.DetectContentType` (ignore the `Content-Type` header and
   filename — both are client-controlled and untrustworthy).
5. Seek back to start with `f.Seek(0, io.SeekStart)` before reading the full file with `io.ReadAll`.

Apply `filepath.Base` to the `name` field immediately after `TrimSpace` to strip any directory components that could
cause path traversal. Check `name == "" || name == "."` (not just `name == ""`) to reject the degenerate base case
`filepath.Base("")` returns `"."`.

### File materialization ordering

In `Upload`, commit the DB transaction **before** calling `materialize`. This is the safe failure direction: if
materialization fails after commit, the row persists in the DB and `GetForProduct` can re-materialize from the stored
`data` column on next read. The reverse order (materialize → commit) risks an orphan file if commit fails after the
disk write.

### Circular import: handler ↔ app

`internal/app` imports `internal/handler`; therefore `internal/handler` must never import `internal/app`. When the
handler or service needs config values from the `app.Config` struct, pass the scalar values (e.g., `maxSize int`,
`allowedTypes []string`) at construction time instead of passing the config struct.

## Implementing features

When asked to implement a new feature, use `api/` and `internal/sql/` package content as additional context to
understand the requirements.

### Database migrations

**Modify existing migration files in place** — never create a new numbered migration file to alter an existing schema. If the schema needs a change (rename a table, add a column), edit the existing `xxx_init.up.sql` and its matching `xxx_init.down.sql` directly. Creating a separate `002_rename.up.sql` is wrong.

Whenever an `xxx-up` migration is created or modified, always update the corresponding `xxx-down` migration to drop
all objects added by the up migration, in reverse dependency order (child tables before parent tables). Never leave the
down migration out of sync with the up migration.

### Empty title/description validation in Create

In `product.Create`, after verifying that every shop language has a corresponding entry in `req.Data`, also check
`c.Title == ""` → return `&MissingTitleError{Lang: lang}` and `c.Description == ""` → return
`&MissingDescriptionError{Lang: lang}`. These typed errors carry the offending language code so the response message
can be specific: `"title is required for the language EN"`. The handler maps both with `errors.As` → `BadRequestError`
(HTTP 400). Use typed errors (not a sentinel like `ErrMissingTitle`) whenever the error must carry context data.

### Partial upsert with nullable columns

When a table has a NOT NULL column (e.g., `name`) and an optional column (e.g., `description`), and the update API allows updating each independently, use a two-pass approach:

**Pass 1** — Upsert rows that include a `name`. Use `ON CONFLICT DO UPDATE SET name = excluded.name, description = COALESCE(excluded.description, table.description)` so that an omitted description preserves the existing value.

**Pass 2** — For description-only entries (no `name` provided for that language), use a plain `UPDATE` and check `tag.RowsAffected() == 0`. A zero count means no row exists for that language yet, so return the appropriate domain error (e.g., `ErrInvalidLanguage`) rather than silently doing nothing. A bare INSERT would violate the NOT NULL constraint on `name`.

### Language validation

Any create or update operation that accepts language-keyed data (e.g., a `names map[string]string` field) must handle
the case where the caller supplies an unknown language code. In the service layer, a PostgreSQL FK violation on
`lang_id` (error code `23503`) must be caught and returned as `ErrInvalidLanguage`. The handler must map `ErrInvalidLanguage` to `&BadRequestError{Reason: "invalid language code"}`.

### Country price fallback query pattern

When resolving a price with a fallback to `DEFAULT`, use a single query with `ANY($2)` containing
`[countryID, "DEFAULT"]` and an `ORDER BY CASE WHEN country_id = $3 THEN 0 ELSE 1 END LIMIT 1` to prefer the
exact-country match. `pgx.ErrNoRows` (no price rows at all, or no DEFAULT) returns a zero-value result struct
`&PriceResult{CountryID: "DEFAULT", Value: 0}` — never a domain error, since zero price is a valid state.

### Price response country_id always echoes the request

The `GetProductPrice` handler sets `result.CountryID = country` after receiving the service result. This means the
response always returns the caller's requested country code, even when the price was sourced from the `DEFAULT` fallback
row. The service layer is intentionally left unchanged — it returns the actual DB `country_id` — and the handler is
responsible for this response-shaping concern.

### Country FK validation on prices

The `product_prices` table has a FK on `country_id`. On INSERT, a PostgreSQL FK violation (error code `23503`) means
the country code is not in the `countries` table. Return a typed error that carries the offending code
(`InvalidCountryError{Country: code}`) so the handler can include it in the 400 response body.

### Full-replace update strategy for content tables

When a content table has **all** non-nullable columns (e.g., `product_data` with both `title NOT NULL` and
`description NOT NULL`) and the update API replaces the entire content set in one call, use a full-replace strategy:
DELETE all existing rows for the parent entity, then INSERT the new set inside the same transaction.

This is simpler and correct when no partial-update edge case exists. Contrast with the two-pass upsert pattern (see
"Partial upsert with nullable columns") which is needed only when some columns are nullable and callers may omit them
on a per-language basis.

### Atomic existence check via UPDATE RowsAffected

To verify a resource exists inside a transaction without a separate `SELECT EXISTS` query (which would introduce a
TOCTOU race), run:

```sql
UPDATE <table> SET updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL
```

Then call `tag.RowsAffected()`. If it returns `0`, no live row matched — return the appropriate domain error (e.g.,
`ErrProductNotFound`). This is the preferred pattern for any update endpoint that must confirm existence atomically
before modifying related rows.

### Product limit enforcement

When a shop has a `max_products` cap, the `Create` method must count non-deleted products (`deleted_at IS NULL`) before
inserting. If `count >= max_products`, return `ErrShopProductLimitReached`. The handler maps this to
`&ConflictError{Reason: "shop product limit reached"}` (HTTP 409). The count query runs before the transaction (after the
shop-existence check) using the existing pool connection, following the same pattern as the pre-transaction language
validation.

### Owner validation

When an `owner_id` FK insert fails with PostgreSQL error code `23503`, the service must return `ErrInvalidOwner`. The handler maps it to `&BadRequestError{Reason: "invalid owner id"}`. Unlike language FK violations (which appear in a separate loop on `shop_names`), the owner FK violation occurs on the `shops` INSERT itself, so the check is on the first `tx.Exec` call.


### File materialization in service layer

When a service method materializes files from the database onto disk, use `errors.Is(statErr, fs.ErrNotExist)` (not
`os.IsNotExist`) to distinguish "file missing" from other `os.Stat` failures (e.g., permission denied). The full
pattern:

```go
if _, statErr := os.Stat(diskPath); statErr != nil {
    if !errors.Is(statErr, fs.ErrNotExist) {
        return nil, fmt.Errorf("stat file: %w", statErr)
    }
    // materialize: MkdirAll + WriteFile
}
```

Silently swallowing non-ENOENT stat errors with the old `os.IsNotExist(err)` guard means broken static asset URLs when
the filesystem is inaccessible — always propagate unexpected errors.

When materialization logic is needed in multiple methods within the same service, extract it into a private helper
method like `s.materialize(id, name, data) (path string, error)`. This method returns the URL-relative path
(e.g., `/files/{id}/{name}`) after writing the file to disk if needed. Reusing the helper prevents duplication and
ensures consistent behavior across `GetForProduct()`, `Upload()`, and similar methods.

### Upload response vs listing response types in file package

The `file` package separates internal and JSON types deliberately:

- `FileInfo` — internal record returned by `Upload()` and `GetForProduct()`; no JSON tags.
- `UploadResponse` — JSON shape for `POST /files` response (all 7 fields including `created_at`/`updated_at`).
- `PublicFileItem` / `AdminFileItem` — JSON shapes for listing endpoints that shape output differently per caller.

The DB `INSERT INTO files ... RETURNING id, created_at, updated_at` is the canonical way to capture the server-assigned
timestamps without a second round-trip. The `Upload()` service method materializes the file to disk *after* the INSERT,
so `path` is derived in the service layer and never stored in the DB.

### Transactional upload with deferred materialization

The `file.Service.Upload()` method must wrap the INSERT and `materialize()` call inside a single DB transaction. If
materialization fails (disk full, permission denied), the transaction is rolled back and the orphaned DB row is
removed. Pre-transaction checks (e.g., file count limit) run on the pool connection before `tx := s.db.Begin()`.

The pattern:
1. Run pre-transaction validation (counts, limits) on `s.db` (not `tx`).
2. `tx, err := s.db.Begin(ctx)` — start transaction.
3. `defer tx.Rollback(ctx) //nolint:errcheck` — safe even after successful commit; pgx ignores the error.
4. Run INSERT with `tx.QueryRow()` and scan result.
5. Call `s.materialize()` — if this fails, the deferred rollback cleans up the INSERT.
6. `tx.Commit(ctx)` — only if both INSERT and materialize succeed.

The `//nolint:errcheck` comment suppresses the linter because rollback-after-commit returns an error that is expected
and harmless.

### SQL migration table ordering

Tables must be defined in dependency order: a table referencing another must come after it. When adding or reordering
tables in `001_init.up.sql`, verify that every `REFERENCES <table>` points to a table declared earlier in the file.
The corresponding `001_init.down.sql` must drop tables in reverse dependency order (children before parents).

## Tests

### Unit tests

Placed alongside source code in each package. Run with:

```
task go:test:unit -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g., `-run TestName`, `-v`).

In git worktrees, `task go:test:unit` may not be available if `.ci/` is not initialized. In that case, use `go test` directly (e.g., `go test -run TestFoo -v ./internal/...`).

### Running functional tests in a worktree

Git worktrees do not have `.ci/` initialized, so `task go:test:func` is unavailable. To run functional tests from a worktree when the `simshop_tests-postgres-1` Docker container is already running (started by the main repo's test infrastructure), attach to its network directly:

```
docker run --rm \
  --network simshop_tests_default \
  -v /path/to/worktree:/src \
  -w /src \
  golang:1.26-alpine3.23 \
  go test -v -tags=functest -mod=mod ./tests/api/shop/
```

Use `-mod=mod` (not `-mod=vendor`) when the worktree's `vendor/modules.txt` is inconsistent with the Go toolchain inside the container.

### Functional tests

Placed in the `tests/` directory. All files use the `//go:build functest` build tag. Run with:

```
task go:test:func -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g., `-run TestName`, `-v`).

Requires PostgreSQL. Use `task go:test:func` — it starts the necessary containers automatically via
`docker-compose.tests.yaml`. To clean up containers afterward: `task go:test:func:clean`.

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

### Seeder (`tests/pkg/seeder`)

- Create the seeder at the top of the parent test function, before any `main.Run(...)` calls.
- All auxiliary DB queries (reads and writes) belong in the seeder, not inline in the test body.

### OpenAPI spec

The OpenAPI validator (`kin-openapi`) operates in OpenAPI 3.0 compatibility mode. Do **not** use OpenAPI 3.1 array type syntax (`type: ["string", "null"]`) — it will cause `unsupported 'type' value "null"` at startup. Use the 3.0 style instead: `type: string` + `nullable: true`.

For path parameters that hold UUID values, always write `type: string` + `format: uuid`, never `type: uuid`. Using `type: uuid` causes an `unsupported 'type' value "uuid"` panic at startup.

### Shared test helpers in handler package

When a test file in `internal/handler/` is deleted, any helpers it defined (e.g., `buildTestResponder`) become
undefined for other test files in the same package. Keep shared test helpers in `handler_test.go`, not in
per-feature test files, so they survive feature removal.

### Handler unit tests: response ID must be a valid UUID

When a handler response schema declares `id` with `format: uuid` (e.g., `CreateProductResponse`, `CreatePropertyResponse`),
mock return values in handler unit tests must use valid UUID strings (e.g., `"018f4e3a-0000-7000-8000-000000000099"`).
The OpenAPI response validator (`resp.Write`) rejects non-UUID strings and the handler returns HTTP 500, masking the
actual assertion under test.

### Multipart handler validation order in unit tests

The `UploadFile` handler validates fields in this order: MaxBytesReader → ParseMultipartForm → file field
present → name field present → explicit size check → MIME type. When writing unit tests for a case that should reach
the size check (e.g., `FileTooLarge_SizeCheck`), the multipart body must include a valid `name` field — otherwise the
name-validation branch fires first and the test asserts the wrong error. Only tests that target the `MaxBytesReader`
path (body too large to even parse) can safely omit `name`.

### Filesystem-catalog refactor: service constructor signatures

After removing the PostgreSQL backend, service constructors take in-memory data instead of a `*pgxpool.Pool`:
- `product.NewService(products []*Product)` — nil is normalized to `[]*Product{}` inside the constructor (required so
  `List` returns an empty JSON array `[]` rather than `null`, which would fail OpenAPI response validation).

### Filesystem-catalog refactor: handler reduction

The read-only refactor removes `ConflictError`, `PermissionDeniedError`, the `shopService` interface, and all write
handlers (`CreateProduct`, `UpdateProduct`, `SetProductPrices`, `SetProductFiles`, `CreateProperty`, `UpdateProperty`,
`CreateShop`, `UpdateShop`, `UploadFile`). The `Handler` struct no longer holds `fileMaxSize`, `fileAllowedMTs`, or
`shop` fields. When rewriting handler_test.go, remove the `Conflict` and `PermissionDenied` test cases that referenced
deleted error types.

The subsequent removal of `/products/{id}`, `/products/{id}/prices`, and `/products/{id}/files` routes further reduced
the handler to only `ListProducts`. This also removed: the `file` package entirely, `product.PriceResult`,
`product.Product.Prices` and `product.Product.Files` fields, price/file loading from the loader, `PublicDir` from the
`Server` config, and the `index` map from `product.Service`. The `loader.Load` signature lost its `zerolog.Logger` and
`publicDir` parameters since no warnings are issued and no disk reads are needed after file loading was removed.

### Filesystem-catalog refactor: loader package

The `internal/loader` package is responsible for reading product directories from `data_dir` at startup and producing
a `Catalog` struct consumed by service constructors. Key design decisions:
- Each product lives in its own subdirectory under `{data_dir}/products/{id}/`. The directory name is the product ID.
  The product is described by `product.yaml` inside that directory.
- A missing `products/` subdirectory is not an error — results in an empty catalog.
- A malformed YAML file or a validation error is fatal (returned as `error` from `Load`).
- Validation runs inside `loadProduct` immediately after YAML parsing; no separate validation step.
- `app.go` must call `loader.Load` before constructing services; the catalog's slices/maps are passed directly
  to `product.NewService`.

### Product data model

The `product.Product` struct mirrors the `product.yaml` schema:
- `Name map[string]string` — lang → display name.
- `Description map[string]string` — lang → long-form description.
- `Specs map[string]map[string]SpecItem` — specKey → lang → `{Title, Value}`.
- `Price map[string]PriceItem` — country code or `"default"` → `{Currency, Value}`.
- `Attrs map[string]map[string]AttrLang` — attrKey → lang → `{Title, Values map[string]AttrValue}`.
  Each `AttrValue` has `{Title, AddPrice}`.
- `Images []ImageItem` — list of `{Preview, Full}` paths relative to the product's `images/` subdirectory.

### Product validation rules (enforced at startup)

Validation lives in `internal/loader/loader.go:validate`. Rules are fatal at startup:
1. `name` must be non-empty.
2. `description` must be non-empty.
3. Language sets of `name` and `description` must be identical.
4. Every spec entry must cover exactly the languages in `name`.
5. `price` must contain a `"default"` key.
6. Every attr entry must cover exactly the languages in `name`, and each language entry must have ≥ 1 value.
7. Every image `preview` and `full` path must exist on disk relative to `{productDir}/images/` (checked with `os.Stat`; uses `errors.Is(err, fs.ErrNotExist)`).

### Filesystem-catalog refactor: Write tool prerequisite

The Write tool requires that the file has been Read at least once in the session before overwriting. When writing many
files in a single pass, Read each target file before calling Write — even for files that will be completely replaced.

### Filesystem-catalog refactor: functional test pattern (no DB)

After removing PostgreSQL, functional tests use YAML fixture files instead of a seeder. Pattern:
- `makeProductDir(t, dataDir, id, yaml, extraFiles)` creates `{dataDir}/products/{id}/product.yaml` and any
  extra files (e.g. images) specified as a `map[string][]byte` of relative path → content pairs.
- `testapp.New(t, dataDir)` replaces `testapp.New(t)` — always takes a `dataDir` argument.
- No seeder, no DB pool, no `testpostgres`. The `docker-compose.tests.yaml` postgres service is removed.
- Subtests that need a completely separate empty catalog start their own `testapp` inside the subtest body
  (not a separate top-level function) — this is safe because each app binds to a random port.

### API functional tests (`tests/api/`)

- All API subtests within a `TestFoo` share one `testapp` instance started in the parent function. Starting a separate
  instance per subtest panics on port conflict when subtests run in parallel.
- At most one top-level `TestFoo` function per package may call `main.Parallel()` if each starts its own `testapp`.
  When multiple parallel top-level tests each start a `testapp`, they all resume concurrently and conflict on port 9000.
  New API test functions that start their own app must NOT call `main.Parallel()` unless the existing parallel test
  functions are refactored to share a single app.
- Resolve shared test fixtures (e.g., `sd.GetAdminUser`) once at the parent `TestFoo` level on `main`, not inside
  sub-test closures. This matches all sibling test files and avoids redundant DB calls per subtest.
- For timestamp fields that must be non-null (e.g., `created_at`, `updated_at`), use both `assert.Contains` (key
  present) and `assert.NotNil` (value non-null). Use `assert.Contains` alone only for nullable fields like `owner_id`.
- When testing response body shape, assert the `names` map value directly (e.g., `names["en"]`) rather than only
  checking the field exists, so the test catches serialization regressions.
- For computed fields with a known format (e.g., `path: "/files/{uuid}/{name}"`), extract and assert the exact format
  rather than just checking non-emptiness. Type-assert dependent values first (e.g., `id, ok := body["id"].(string)`)
  to avoid malformed data slipping through tests.


### Filesystem-catalog refactor: cleanup after backend removal

After removing a database backend (e.g., pgx/v5), run `go mod tidy && go mod vendor` to strip all dead direct and
indirect dependencies in one step. `go mod tidy` removes unused entries from `go.mod`/`go.sum`; `go mod vendor` syncs
`vendor/`. Packages that were only indirect dependencies of the removed direct dep (e.g., pgpassfile, puddle, pgerrcode)
disappear automatically — no manual editing of `go.mod` is needed.

When removing write endpoints from a handler, also delete any helper methods that only served those endpoints (e.g.,
`unmarshal` is only needed for request-body decoding; a read-only handler has no request bodies and should not keep
it). The linter (`unused`) will catch stragglers.

Functional tests that no longer require a database can be run directly with `go test -tags=functest ./tests/...`
without Docker or task infrastructure — useful in worktrees where `.ci/` is not initialized.

### Image path transformation in loader

After `validate`, `loadProduct` rewrites each non-empty `Images[i].Preview` and `Images[i].Full` from a bare filename
(e.g. `"thumb.jpg"`) to a URL-ready path (e.g. `"/images/{id}/thumb.jpg"`). Validation runs against the bare
filenames first (so `os.Stat` works), then the transformation happens so the JSON response contains downloadable
paths without any handler-level mapping.

### Image serving route

`GET /images/{product_id}/{file_name}` is served by `handler.ServeImage`. It reads from
`{data_dir}/products/{product_id}/images/{file_name}` using `http.ServeFile`. The handler applies `filepath.Base`
to both path values before joining to prevent path-value injection traversal. `handler.NewHandler` takes `dataDir
string` as its third parameter (before `zerolog.Logger`) so the handler can resolve image paths without importing
`internal/app`.

Functional tests for image serving live in `tests/api/image/` and must create a complete valid product directory
(including `product.yaml`) alongside the `images/` subdirectory, because the loader rejects product directories
missing `product.yaml` at startup.
