# Project Rules for Claude

## Meta

After completing any task, reflect on what was learned. If a decision, pattern, or constraint is non-obvious and likely
to recur, add it to this file under the appropriate section. Do not record things already derivable from reading the
code.

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

### Language validation

Any create or update operation that accepts language-keyed data (e.g., a `names map[string]string` field) must handle
the case where the caller supplies an unknown language code. In the service layer, a PostgreSQL FK violation on
`lang_id` (error code `23503`) must be caught and returned as `ErrInvalidLanguage`. The handler must map `ErrInvalidLanguage` to `&BadRequestError{Reason: "invalid language code"}`.

## Tests

### Unit tests

Placed alongside source code in each package. Run with:

```
task go:test:unit -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g., `-run TestName`, `-v`).

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
- Before claiming any work is done, invoke the `superpowers:verification-before-completion` skill and run the relevant
  tests.
- Do not consider a task complete until tests pass. Do not respond with a summary of changes before running tests.
- Group related tests under a single parent function `TestFoo(main *testing.T)` and use `main.Run("CaseName", ...)` for
  sub-tests. Never write separate top-level functions like `TestFoo_CaseName`.

### Seeder (`tests/pkg/seeder`)

- Create the seeder at the top of the parent test function, before any `main.Run(...)` calls.
- All auxiliary DB queries (reads and writes) belong in the seeder, not inline in the test body.

### API functional tests (`tests/api/`)

- All API subtests within a `TestFoo` share one `testapp` instance started in the parent function. Starting a separate
  instance per subtest panics on port conflict when subtests run in parallel.

