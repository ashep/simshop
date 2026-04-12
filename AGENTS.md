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

- **PostgreSQL** as the database (via `pgx/v5`)
- **zerolog** for structured logging
- **go-app** framework (`github.com/ashep/go-app`) for app lifecycle/config
- **OpenAPI** spec in `api/` for request validation

Key directories:

- `internal/` — application code (app, auth, handler, product, shop, sql, etc.)
- `api/` — OpenAPI spec files
- `tests/` — functional tests (build tag: `functest`)
- `vendor/` — vendored dependencies

Config is loaded from `config.yml` and environment variables. The database DSN is set under `database.dsn`.

Do not run `docker compose` directly — the `task` commands manage containers automatically.

## Architecture

Request processing middleware chain (innermost to outermost): `content-type → auth → openapi validation → handler`.

Each request struct exposes a `Trim()` method that strips leading/trailing whitespace from all string fields (including map keys and values). Handlers call `req.Trim()` immediately after `h.unmarshal()`, before any validation or service call.

Endpoints that are public but return extra fields for authenticated admins use `optionalAuthMw` (from `auth.OptionalMiddleware`). This middleware sets the user in context if a valid API key is present, but does not reject unauthenticated requests. Example: `GET /shops/{id}` uses `optionalAuthMw(openapiMw(hdl.GetShop))`.

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

## Implementing features

When asked to implement a new feature, use `api/` and `internal/sql/` package content as additional context to
understand the requirements.

### Database migrations

**Modify existing migration files in place** — never create a new numbered migration file to alter an existing schema. If the schema needs a change (rename a table, add a column), edit the existing `xxx_init.up.sql` and its matching `xxx_init.down.sql` directly. Creating a separate `002_rename.up.sql` is wrong.

Whenever an `xxx-up` migration is created or modified, always update the corresponding `xxx-down` migration to drop
all objects added by the up migration, in reverse dependency order (child tables before parent tables). Never leave the
down migration out of sync with the up migration.

### Partial upsert with nullable columns

When a table has a NOT NULL column (e.g., `name`) and an optional column (e.g., `description`), and the update API allows updating each independently, use a two-pass approach:

**Pass 1** — Upsert rows that include a `name`. Use `ON CONFLICT DO UPDATE SET name = excluded.name, description = COALESCE(excluded.description, table.description)` so that an omitted description preserves the existing value.

**Pass 2** — For description-only entries (no `name` provided for that language), use a plain `UPDATE` and check `tag.RowsAffected() == 0`. A zero count means no row exists for that language yet, so return the appropriate domain error (e.g., `ErrInvalidLanguage`) rather than silently doing nothing. A bare INSERT would violate the NOT NULL constraint on `name`.

### Language validation

Any create or update operation that accepts language-keyed data (e.g., a `names map[string]string` field) must handle
the case where the caller supplies an unknown language code. In the service layer, a PostgreSQL FK violation on
`lang_id` (error code `23503`) must be caught and returned as `ErrInvalidLanguage`. The handler must map `ErrInvalidLanguage` to `&BadRequestError{Reason: "invalid language code"}`.

### EN title requirement

Any entity that carries language-keyed titles (e.g., `titles map[string]string`) must always include an `EN` entry on
creation **and on update**. Enforce this at three layers:

1. **OpenAPI spec** — add `required: [EN]` under the `titles` object in the create request schema.
2. **Service layer** — check `req.Titles["EN"] == ""` at the top of `Create` and return `ErrMissingEnTitle`.
3. **Handler** — map `ErrMissingEnTitle` to `&BadRequestError{Reason: "EN title is required"}`.

Language codes are stored and compared as **uppercase** (e.g., `"EN"`, `"UK"`). Use uppercase in all spec `required`
lists, service checks, and test fixtures. Using lowercase (e.g., `"en"`) will cause response-validation failures
because the DB always returns uppercase keys.

### Country price fallback query pattern

When resolving a price with a fallback to `DEFAULT`, use a single query with `ANY($2)` containing
`[countryID, "DEFAULT"]` and an `ORDER BY CASE WHEN country_id = $3 THEN 0 ELSE 1 END LIMIT 1` to prefer the
exact-country match. `pgx.ErrNoRows` (no price rows at all, or no DEFAULT) returns a zero-value result struct
`&PriceResult{CountryID: "DEFAULT", Value: 0}` — never a domain error, since zero price is a valid state.

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

### Admin vs public response shaping

When an endpoint is publicly accessible but returns additional fields for admin callers (or also shop owners), use
`auth.GetUserFromContext` to check for admin status in the handler, then encode either the admin struct (e.g.,
`*shop.AdminShop`) or just the public embedded struct (e.g., `sh.Shop`). Declare the local variable as `any` so both
types satisfy it without a cast. The service always returns the full admin struct; the handler decides what to
serialise.

When shop-owner access must also be checked (e.g., `GET /products/{id}`), include the owner_id in the admin struct
with `json:"-"` (so it is never serialised) and compare `user.ID == p.ShopOwnerID` alongside `user.IsAdmin()`. The
service fetches owner_id from the DB in the same query that fetches the resource.

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
- Do not consider a task complete until tests pass. Do not respond with a summary of changes before running tests.
- Group related tests under a single parent function `TestFoo(main *testing.T)` and use `main.Run("CaseName", ...)` for
  sub-tests. Never write separate top-level functions like `TestFoo_CaseName`.

### Seeder (`tests/pkg/seeder`)

- Create the seeder at the top of the parent test function, before any `main.Run(...)` calls.
- All auxiliary DB queries (reads and writes) belong in the seeder, not inline in the test body.

### OpenAPI spec

The OpenAPI validator (`kin-openapi`) operates in OpenAPI 3.0 compatibility mode. Do **not** use OpenAPI 3.1 array type syntax (`type: ["string", "null"]`) — it will cause `unsupported 'type' value "null"` at startup. Use the 3.0 style instead: `type: string` + `nullable: true`.

For path parameters that hold UUID values, always write `type: string` + `format: uuid`, never `type: uuid`. Using `type: uuid` causes an `unsupported 'type' value "uuid"` panic at startup.

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

