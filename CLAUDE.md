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

When adding a field to an existing struct or method, read the current state of the file first and preserve all
existing fields and logic — including any that were added by the user outside of the tracked plan. Never use
`replace_all` to propagate a new field into test bodies without first auditing every match: a "missing X" test
must not have X added to its body.

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

### Geo detector

`internal/geo.Detector` caches country lookups in memory with a 1-hour TTL. Only successful lookups (non-empty
country string) are written to the cache. A service error (non-200 or network failure) returns `""` and is **not**
cached — the next call for the same IP will retry the service. This prevents a transient error from silencing retries
for an hour.

The `CF-IPCountry` header is validated as a strict 2-letter ISO alpha-2 code by `isAlpha2` before being accepted.
Any other value (e.g., multi-character strings, empty) is ignored and falls through to IP-based detection. This
prevents header spoofing from influencing pricing decisions with garbage values.

`clientIP` uses `X-Forwarded-For` only when the `CF-IPCountry` header is absent. In production (behind Cloudflare)
the XFF path is never reached; without Cloudflare, XFF is caller-controlled and can be spoofed.

The `lookup` response body is capped at 16 bytes via `io.LimitReader` — a country code is at most 2 characters;
the extra headroom protects against runaway responses from the geo service.

Request processing middleware chain (innermost to outermost): `content-type → openapi validation → handler`.

Routes are registered with Go 1.22+ stdlib pattern syntax: `"METHOD /path"` (e.g., `"GET /products"`).

Handlers use `BadRequestError`, `NotFoundError`, and `BadGatewayError` (defined in `internal/handler/handler.go`) to map
domain errors to HTTP responses via `h.writeError(w, err)`. Always populate the `Reason string` field with a
human-readable message (e.g., `&BadRequestError{Reason: "invalid language code"}`). The reason is written directly into
the JSON response body as `{"error": "<reason>"}`, so it is client-visible. `BadGatewayError` maps to HTTP 502 and is
used when an upstream service (e.g., Nova Poshta API) returns an error.

Domain errors are defined in the service package. The handler maps them to HTTP error types using `errors.Is`.

Always use the most semantically appropriate HTTP status code. Examples: 404 Not Found for missing resource — not a
generic 400 Bad Request.

### Attribute prices (`attr_prices`)

`attr_prices` in `product.yaml` holds per-attribute, per-value, per-country add-on prices. The structure is:
`attr_key → value_key → country_key → float64`. The `"default"` country key is required and used as a fallback
when the user's country is absent. `product.Product.AttrPrices` stores the raw map; `product.ProductDetail.AttrPrices`
holds the resolved form (`attr_key → value_key → float64`) built by `ServeProductContent` using the same country
resolution logic as the base prices. `AttrValue` no longer carries `add_price` — it has only `title`.

### Price field naming

`product.yaml` uses `prices:` as the YAML key (a map of country codes to `{currency, value}`). The `Product` struct
field is `Prices` with `yaml:"prices"`. In the API response (`GetProductResponse`), the resolved single-country price
is serialised as `"price"` — the `ProductDetail.Prices` field carries `json:"price"`. The YAML key and JSON key are
intentionally different.

### Attribute descriptions (`AttrLang.Description`)

`AttrLang` carries an optional `description` field (`yaml:"description"`, `json:"description,omitempty"`). It is a
per-language, per-attribute free-text string (supports Markdown). No country resolution is needed — it is copied
directly into `ProductDetail.Attrs` alongside `title` and `values`. The `AttrLang` OpenAPI schema declares it as
`type: string` + `nullable: true`. Omitted from JSON when empty.

### Attribute values order (`attr_values_order`)

`attr_values_order` in `product.yaml` holds the preferred display order for each attribute's values. The structure is:
`attr_key → []value_key` (ordered list). It is optional — attributes without an entry retain their natural map
iteration order. `product.Product.AttrValuesOrder` stores the raw map (`yaml:"attr_values_order"`).
`product.ProductDetail.AttrValuesOrder` (`json:"attr_values_order,omitempty"`) is a top-level field in the response,
at the same level as `attr_prices` and `attr_images`. It is populated by `ServeProductContent` by assigning
`p.AttrValuesOrder` directly when non-empty. No country resolution or loader rewriting is needed. The
`GetProductResponse` OpenAPI schema declares `attr_values_order` as `type: object, nullable: true` with
`additionalProperties: type: array, items: string`.

### Attribute images (`attr_images`)

`attr_images` in `product.yaml` holds per-attribute, per-value image filenames. The structure is:
`attr_key → value_key → filename` (string). No country resolution is applied — each value maps to exactly one image
filename. `product.Product.AttrImages` stores bare filenames (e.g. `"thumb.png"`), validated at startup by
`validate()` using the same `os.Stat` pattern as `images[]`. `product.ProductDetail.AttrImages` holds the rewritten
form (`attr_key → value_key → URL path`) built by `ServeProductContent`: bare filenames become
`/images/{id}/{filename}`. Path rewriting happens in the handler, not the loader — do not rewrite in the loader.
Unlike `attr_prices`, no fallback to `"default"` is needed because there is no country dimension.

### CORS middleware

`handler.CORSMiddleware()` returns a middleware that unconditionally sets `Access-Control-Allow-Origin: *` for all
requests. OPTIONS preflight requests (detected by `Access-Control-Request-Method` header) are intercepted and responded
to with 204 before calling `next`. There is no per-origin configuration — all origins are always allowed.

Explicit `OPTIONS` routes must be registered alongside each `GET` route in `app.go` because Go 1.22+ stdlib mux does
not implicitly handle OPTIONS for registered methods.

### Rate limit middleware

`handler.RateLimitMiddleware(rpm int)` returns a middleware that limits each client IP to `rpm` requests per minute
per endpoint. Excess requests receive HTTP 429 with a `Retry-After` header and JSON error body. The rate limiter uses
a sliding window approach: each client IP gets one entry per request, and subsequent requests within `window =
60s / rpm` are rejected. A background goroutine sweeps expired entries every `window` seconds. The `rateLimitClientIP`
function extracts the client IP, preferring `X-Forwarded-For` (set by Cloudflare) over `RemoteAddr`.

The `POST /orders` endpoint is rate-limited via `cfg.RateLimit` (`rate_limit` in config YAML). A value of 0 uses
the default of 1 RPM. A negative value disables rate limiting entirely. Functional tests set `cfg.RateLimit = -1`
to disable rate limiting so multiple requests can be made synchronously from the same IP without hitting the limit.

### Circular import: handler ↔ app

`internal/app` imports `internal/handler`; therefore `internal/handler` must never import `internal/app`. When the
handler or service needs config values from the `app.Config` struct, pass the scalar values at construction time
instead of passing the config struct.

The same rule applies to other internal packages (e.g. `internal/geo`): define a local interface in `handler` (e.g.
`geoDetector`) satisfied by the concrete type, and wire it in `internal/app`. This avoids coupling handler to
implementation packages.

## Implementing features

When asked to implement a new feature, use `api/` package content as additional context to understand the requirements.

### Image path transformation

Image paths are bare filenames in YAML (e.g. `"thumb.jpg"`) and must be rewritten to URL-ready paths
(e.g. `"/images/{id}/thumb.jpg"`) before returning to the client. This transformation must happen in **two** places:

1. **Loader** (`loadProduct`): after `validate`, rewrites paths at startup so `catalog.Products` contains downloadable
   URLs. Validation runs against bare filenames first (so `os.Stat` works).

2. **`ServeProductContent` handler**: reads `product.yaml` directly at request time (bypassing the loader), so it must
   apply the same transformation inline after YAML parsing, before building `ProductDetail`.

Any new handler that reads `product.yaml` directly must apply this transformation — do not assume the loader has
already rewritten the paths.

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

Price selection uses `h.geo.Detect(r)` to get the country code, then looks up `p.Price[country]`; falls back to
`p.Price["default"]` if the country key is absent. `ProductDetail.Price` is a single `PriceItem`, not a map.

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

### Orders route

`POST /orders` is served by `handler.CreateOrder`. It reads the product YAML from disk (same pattern as
`ServeProductContent`), validates required fields (`product_id`, `lang`, `first_name`, `last_name`, `phone`,
`email`, `country`, `city`, `address`), resolves the base price from `p.Prices[req.Country]` with `default`
fallback, and walks attributes (sorted by attribute key) to build a `[]order.Attr` of rendered title pairs and
per-attribute add-on prices in cents (also resolved by `req.Country` with `default` fallback). The total order
price is the base plus the sum of `Attr.Price`. The handler then delegates to `h.orders.Submit(ctx, order.Order)`.
Returns 201 with `{"payment_url": "https://foo.bar"}` (stub) on success, 400 for missing/invalid fields, 404 for
unknown product or path traversal, 502 if the order service returns an error. The `orderService` interface is defined
in `internal/handler/order.go`. `NewHandler` accepts `orders orderService` after `np novaPoshtaClient`.

`country` is request-supplied at order time, not geo-detected. The same value drives both price resolution and
the `orders.country` column, so the stored country always matches the price tier the customer was quoted; geo
detection (`h.geo`) is still wired into the handler for `ServeProductContent` (browsing) but is not consulted
by `CreateOrder`. Tests that need to exercise the `default` price fallback should send a country with no
matching `prices.<country>` key (e.g., `"us"` against a UA-only catalog), not an empty string — empty values
are now rejected with 400.

`CreateOrder` validates `req.Country` against `shop.Shop.Countries` (loaded from `shop.yaml`) by calling
`h.shop.Get(ctx)` and a map lookup (`_, ok := s.Countries[req.Country]`). This check runs before the product
YAML is read so disallowed countries fail fast. A country not in the allow-list returns HTTP 400 with
`{"error": "invalid country"}`. The allowed-countries map is independent of `prices.<country>` keys: the price
for a permitted country can still fall back to `prices.default`. Use a country present in the shop allow-list
but absent from the product's `prices` map to exercise the price `default` fallback path. The `default` key in
`prices` is **not** an allowed country — it is a price-resolution fallback only.

Prices crossing the domain boundary are integer minor units (cents) — never float — to match the DB column type
and avoid float drift. The handler does the conversion: `int(math.Round(value * 100))` for both the base price and
each attribute add-on. Round once per amount; do not multiply totals because the rounding bias would compound.

### Nova Poshta routes

`GET /nova-poshta/cities?q=<query>` and `GET /nova-poshta/branches?city_ref=<ref>&q=<query>` proxy to the
Nova Poshta JSON API v2 (`POST https://api.novaposhta.ua/v2.0/json/`). The client lives in
`internal/novaposhta/client.go`. The API key is read from `config.yaml` under `nova_poshta.api_key`.
`nova_poshta.service_url` is empty in production (defaults to the real NP URL) and set to a test server URL
in functional tests via `testapp.New` options. Both `q` and `city_ref` (branches only) are required — missing
either returns 400. NP API failure returns 502 via `BadGatewayError`.

### Monobank acquiring client (`internal/monobank`)

`internal/monobank` is a thin client for the Monobank acquiring API. `APIError` is the shared error type returned
whenever the API responds with an application-level error or a non-2xx HTTP status; callers use `errors.As` to
extract `ErrCode`/`ErrText` for logging without leaking detail in user-facing responses.

`MapCurrency(code string) (int, error)` converts an ISO-4217 alpha-3 currency code (case-insensitive) to the
ISO-4217 numeric code expected by the Monobank API (UAH=980, USD=840, EUR=978). Unknown codes return
`*APIError{ErrCode: "unsupported_currency"}`. The `ErrText` field holds the original (uppercased) input for
forensic logging.

### Orders persistence (`internal/orderdb`)

`orderdb.Writer` implements `order.Writer`. It owns a `*pgxpool.Pool` (concrete, not an interface) and writes
each order in a single transaction:

1. `INSERT INTO orders (...) ... RETURNING id` — captures the DB-generated `uuidv7` id.
2. For each `order.Attr`, `INSERT INTO order_attrs (order_id, attr_name, attr_value, attr_price)`.
3. `INSERT INTO order_history (order_id, status) VALUES ($1, 'new')` — one initial history row per created
   order. `status` is the `order_status` enum and is NOT NULL; the literal `'new'` matches the orders table
   default for newly created orders. `note` is left NULL; `id` and `created_at` are filled by column defaults.
   The history insert is a side effect of order creation, not driven by any field in `order.Order`; do not add
   a history field to the public `Order` struct.
4. `Commit`. The deferred `Rollback` is a no-op after a successful commit (`pgx.ErrTxClosed`, ignored).

This is transactional because a half-written order — header without attrs, or attrs/history without header —
would be worse than a clean failure: the customer's record would be ambiguous and reconciliation harder.
Atomicity is the whole reason for the `order_attrs` and initial `order_history` writes living in the same
transaction as `orders`.

The wired pool is created in `internal/app/app.go` via `pgxpool.New(rt.Ctx, cfg.Database.DSN)` after
`dbmigrator.RunPostgres` has applied the embedded migrations from `internal/sql/`.

The DB schema columns drive the field mapping. `order.Order` carries only what the writer must INSERT — `id`,
`status`, `created_at`, and `updated_at` come from PostgreSQL defaults and are absent from the struct.

- `Order.Attrs` is `[]order.Attr`. Each `Attr{Name, Value, Price}` becomes one row in `order_attrs`, with `Price`
  being the per-attribute add-on amount in minor units (cents). `Order.Price` is the total — base + sum of
  `Attr.Price` — so it can be read directly without joining.
- `Attr.Name` and `Attr.Value` store **rendered titles** in the customer's language (e.g. `"Display color"` →
  `"Red"`), not raw attribute/value IDs. This makes orders self-documenting and immutable to later product YAML
  renames; it is the reason `handler.CreateOrder` resolves `attrLang.Title` and `attrVal.Title` before appending
  to the slice.
- `MiddleName == ""` and `CustomerNote == ""` are converted to SQL NULL via the `nullIfEmpty` helper so the
  nullable columns store NULL rather than an empty string.

#### No unit tests in `internal/orderdb`

The transactional writer needs `*pgxpool.Pool.Begin → pgx.Tx`. `pgx.Tx` is a wide interface (Exec, Query,
QueryRow, Commit, Rollback, CopyFrom, SendBatch, Prepare, …) that is impractical to stub by hand, and no mock
package (e.g. `pgxmock`) is vendored. Rather than introduce a leaky narrow interface or a heavy dep just for
unit coverage, the writer is exercised end-to-end by `tests/api/order/post_test.go` against the real Postgres
instance from `docker-compose.tests.yaml`. Don't reintroduce a stub-based unit test for this package without
also providing a real `pgx.Tx` implementation.

### API key middleware (`handler.APIKeyMiddleware`)

`handler.APIKeyMiddleware(apiKey string)` returns a bearer-token middleware backed by `cfg.Server.APIKey`. It
parses the `Authorization` header, requires the `Bearer ` scheme, and compares the supplied token to the
configured key with `crypto/subtle.ConstantTimeCompare` to defeat timing attacks. The middleware emits two
distinct error reasons, both as HTTP 401: `"missing or invalid authorization header"` when the header is absent
or does not start with `Bearer ` (or carries an empty token), and `"invalid api key"` when the header is
well-formed but the token does not match. Both produce HTTP 401 via `UnauthorizedError`. Passing an empty
`apiKey` to the constructor is a programmer error — the middleware would accept any empty bearer. Only register
the protected route in `app.go` when `cfg.Server.APIKey != ""`; do not push that check inside the middleware.

### Orders read path (`internal/order` + `internal/orderdb`)

`order.Record` is the read-side struct returned by `GET /orders`. It carries the DB-generated columns
(`ID`, `Status`, `CreatedAt`, `UpdatedAt`) alongside the persisted order fields, plus inline `Attrs []Attr`
and `History []HistoryEntry`. `order.Record` is intentionally separate from `order.Order` (the write-only
struct used by `CreateOrder`) — the two have different lifetimes and different fields, so unifying them would
muddy both. Nullable text columns (`MiddleName`, `AdminNote`, `CustomerNote`, `HistoryEntry.Note`) are typed as
`*string` so that SQL NULL becomes JSON omission via `json:",omitempty"` rather than serialising as an empty
string.

`orderdb.Reader.List(ctx)` issues exactly three queries (`orders`, `order_attrs`, `order_history`) and stitches
the result in Go, keyed by `order_id`, to avoid the N+1 pattern that a per-order fan-out would create. Nullable
text columns are scanned into `pgtype.Text` and converted to `*string` (NULL → nil) before being assigned to the
`Record`. The reader has no unit tests for the same reason `orderdb.Writer` does not (see "No unit tests in
`internal/orderdb`" above): `pgx.Tx`/`pgxpool.Pool` is too wide to stub by hand and no `pgxmock`-style dep is
vendored. Functional tests in `tests/api/order/get_test.go` cover the read path end-to-end against the real
Postgres container.

### Conditional route registration

Routes that depend on operator-supplied secrets (currently only `GET /orders`, gated by `cfg.Server.APIKey`)
are registered **only when their secret is configured**. The conditional registration block lives in
`internal/app/app.go`; the handler itself does not check for an empty key. Because `POST /orders` is registered
unconditionally on the same path, an unregistered `GET /orders` produces HTTP 405 Method Not Allowed (not 404)
— Go 1.22+ stdlib mux returns 405 when a path is registered for some methods but not the requested one. This is
acceptable: the user-facing requirement is that the endpoint be unavailable when the key is unset, and 405 fits
that. Do not add a runtime "401 / 404 if key empty" branch inside the handler — the conditional registration
in `app.go` is the correct shape, and duplicating the check inside the handler would create two sources of
truth.

### `uuidv7` and PostgreSQL 18

The `orders` and `order_history` tables use `id uuid PRIMARY KEY DEFAULT uuidv7()`. `uuidv7()` is a built-in
function in PostgreSQL 18+ — there is no separate `uuidv7` type. Earlier PG versions need an extension
(`pg_uuidv7`) or a custom `CREATE DOMAIN`. The test container in `docker-compose.tests.yaml` pins
`postgres:18-alpine` to satisfy this requirement.

### Functional tests: postgres backend

`docker-compose.tests.yaml` defines a `postgres:18-alpine` service. The `tests` service depends on
`postgres: service_healthy` and exports `APP_DB_DSN=postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable`.
`testapp.New` reads `APP_DB_DSN` (defaulting to the same value) and assigns it to `cfg.Database.DSN`. Tests can
override the DSN via the `opts` callback like any other config field.

Because the postgres container persists across `task go:test:func` runs, tests that need clean state must truncate
explicitly. `tests/api/order/post_test.go` uses a `truncateOrders` helper that runs
`TRUNCATE order_attrs, order_history, orders RESTART IDENTITY CASCADE` before each subtest that inspects the
tables. After a destructive test (e.g. forcing a DB error), restore the schema in `t.Cleanup` rather than leaving
the database in a broken state — `TestCreateOrder_DBFailure` does this by renaming `orders` away and back instead
of dropping it.

When the postgres state ends up dirty (e.g. an interrupted test), run `task go:test:func:clean` to wipe and recreate
the container.

`testapp.New` exposes the DSN via `(*App).DSN()` so tests can open their own `pgxpool.Pool` to inspect or mutate
the database.

### `t.Cleanup` runs after `defer`, not before

`t.Cleanup` callbacks fire after the test function returns *and after all deferred statements run*. If a test does
`defer pool.Close()` and then registers a cleanup that uses the pool, the cleanup will silently fail because the
pool is already closed. Either close the pool inside the cleanup callback, or skip the `defer` and let the cleanup
own teardown. This bit `TestCreateOrder_DBFailure`'s table-rename rollback once already.

### Loader: `shop.yaml` shop settings

`{data_dir}/shop.yaml` holds a single `shop:` key mapping to `shop.Shop` (countries, name, title, description).
It is loaded by `loadShop` into `catalog.Shop`. The file is **mandatory** — a missing `shop.yaml` is a fatal
startup error, and `shop.countries` must be a non-empty map. A malformed YAML file is fatal. The wrapper struct
`shopFile` is needed because the YAML root key is `shop:`, not the struct fields directly. Functional and loader
tests must therefore lay down a valid `shop.yaml` (with at least one `countries` entry) in their data dir, or the
app will refuse to boot.

`shop.Countries map[string]*Country` is keyed by lowercase ISO alpha-2 country code. Each `Country` carries
`Name map[string]string` (per-language display name), `Currency map[string]string` (per-language currency
symbol/code), `PhoneCode string` (international dialing prefix, e.g. `"+380"`), and `Flag string` (a glyph,
typically a Unicode flag emoji such as `"🇺🇦"`, used by the frontend country picker). The set of map keys is
the authoritative source for which countries can place orders (see `CreateOrder` validation). It is independent of
any product's `prices` map: a country key can exist in `shop.countries` without appearing in `prices`, in which
case the `prices.default` fallback is used at order time.

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
5. `prices` must contain a `"default"` key.
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
- `testapp.New(t, dataDir, opts ...func(*app.Config))` takes a `dataDir` argument and optional config mutators.
  Pass `func(cfg *app.Config) { cfg.NovaPoshta.ServiceURL = srv.URL }` to override config fields in tests.
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

### `internal/geo` package: country detection

`geo.Detector` resolves a country code from an incoming HTTP request using two strategies:
1. **Header shortcut**: if `CF-IPCountry` is present, return it lowercased immediately (no network call, no cache).
2. **IP lookup**: extract client IP (prefer first `X-Forwarded-For` entry over `RemoteAddr`), check in-memory cache
   (TTL `1h`), and on a miss call `https://ipinfo.io/{ip}/country`. Result is always lowercased and trimmed. On
   service error, returns `""`.

`geo.NewDetector()` is the production constructor. Tests construct `*Detector` directly (same package) to inject a
`httptest.Server` client and URL — the unexported `httpClient` and `serviceURL` fields are intentionally accessible
this way. The `errcheck` linter requires `defer func() { _ = resp.Body.Close() }()` (not bare `defer resp.Body.Close()`).

### `internal/novaposhta` package: Nova Poshta API client

`novaposhta.Client` wraps the Nova Poshta JSON API v2 (`https://api.novaposhta.ua/v2.0/json/`). It exposes:
- `SearchCities(ctx, query)` — calls `Address.searchSettlements`, returns `[]City{Ref, Name}`.
- `SearchBranches(ctx, settlementRef, query)` — calls `Address.getWarehouses` with `SettlementRef` (the ref returned by
  `searchSettlements`), returns `[]Branch{Ref, Name}`. Do NOT use `CityRef` — that is a different ref from the old
  `getCities` API and will return "City not found".

`NewClient(apiKey, serviceURL string)` is the production constructor; pass `""` for `serviceURL` to use the default.
Tests construct `*Client` directly (same package) to inject `httptest.Server` via the unexported `httpClient` and
`serviceURL` fields — same pattern as `geo.Detector`. Response body is capped at 1 MB via `io.LimitReader`.
`success=false` in the API response is treated as an error regardless of HTTP status.
