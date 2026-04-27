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

`simshop` is a Go HTTP API service (`github.com/ashep/simshop`). Stack: filesystem catalog (products loaded from YAML
at startup, no product database), PostgreSQL for orders, zerolog for structured logging, the `go-app` framework
(`github.com/ashep/go-app`) for app lifecycle/config, and an OpenAPI spec in `api/` for request/response validation.

Key directories: `internal/` (app, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product,
shop, sql), `api/` (OpenAPI spec), `tests/` (functional tests, build tag `functest`), `vendor/` (vendored deps). Config
is loaded from `config.yml` and environment variables; data is read from `data_dir` (default `./data`). When asked to
implement a new feature, use `api/` as additional context.

## Conventions

### Errors and status codes

Handlers map domain errors to HTTP responses via `h.writeError(w, err)`. The handler defines four typed errors in
`internal/handler/handler.go`: `BadRequestError` (400), `NotFoundError` (404), `BadGatewayError` (502), and
`UnauthorizedError` (401). Each carries a `Reason string` field — always populate it with a human-readable message
(e.g. `&BadRequestError{Reason: "invalid language code"}`). The reason is written directly into the JSON body as
`{"error": "<reason>"}`, so it is client-visible. `BadGatewayError` is used when an upstream service (Nova Poshta,
Monobank) fails.

Domain errors are defined in the service package; the handler maps them to HTTP error types using `errors.Is`.

Always use the most semantically appropriate HTTP status code. Examples: 404 Not Found for a missing resource — not a
generic 400 Bad Request.

### OpenAPI

The OpenAPI validator (`kin-openapi`) runs in OpenAPI 3.0 compatibility mode. Do **not** use OpenAPI 3.1 array type
syntax (`type: ["string", "null"]`) — it causes `unsupported 'type' value "null"` at startup. Use 3.0 style:
`type: string` + `nullable: true`.

For path parameters that hold UUID values, always write `type: string` + `format: uuid`, never `type: uuid`. Using
`type: uuid` causes an `unsupported 'type' value "uuid"` panic at startup.

### HTTP middleware

Routes are registered with Go 1.22+ stdlib pattern syntax: `"METHOD /path"` (e.g. `"GET /products"`). The middleware
chain wrapping a handler, innermost to outermost, is: `content-type → openapi validation → handler`.

CORS is provided by `handler.CORSMiddleware()`, which unconditionally sets `Access-Control-Allow-Origin: *`. OPTIONS
preflight requests (detected by `Access-Control-Request-Method`) are intercepted and answered with 204 before `next`
is called. There is no per-origin configuration. **Explicit `OPTIONS` routes must be registered alongside each `GET`
route in `app.go`** — Go 1.22+ stdlib mux does not implicitly handle OPTIONS for registered methods.

Rate limiting is provided by `handler.RateLimitMiddleware(rpm int)`: per-client-IP sliding window of `60s/rpm`,
sweeping expired entries in a background goroutine. Excess requests get HTTP 429 with `Retry-After`. Client IP comes
from the first `X-Forwarded-For` entry (Cloudflare) falling back to `RemoteAddr`.

The `POST /orders` endpoint is rate-limited via `cfg.RateLimit` (`rate_limit` in YAML). A **positive** value sets
the per-IP RPM ceiling; **`0` or any negative value disables rate limiting entirely** (functional tests use `-1` to
allow synchronous bursts from the same IP).

API key auth is provided by `handler.APIKeyMiddleware(apiKey string)`: requires `Authorization: Bearer <token>` and
compares with `crypto/subtle.ConstantTimeCompare` to defeat timing attacks. Two distinct 401 reasons are emitted —
`"missing or invalid authorization header"` (header absent or wrong scheme) and `"invalid api key"` (token mismatch);
both are written directly via the local `writeUnauthorized` helper, not via `writeError(&UnauthorizedError{...})`.
Passing an empty `apiKey` to the constructor is a programmer error — the middleware would accept any empty bearer.
Only register the protected route in `app.go` when `cfg.Server.APIKey != ""`; do not push that check into the
middleware.

### Money values

Prices crossing the domain boundary are integer minor units (cents) — never float — to match the DB column type and
avoid float drift. The handler does the conversion: `int(math.Round(value * 100))` for both the base price and each
attribute add-on. **Round once per amount; do not multiply totals**, because the rounding bias would compound.

### Country codes

Country codes are lowercase ISO 3166-1 alpha-2 throughout. Two distinct sets are involved:

- **Allow-list** — `shop.Countries` (loaded from `shop.yaml`). Authoritative for which countries can place orders.
  Validated by `CreateOrder` before the product YAML is read.
- **Price keys** — `prices.<country>` in each `product.yaml`. Independent of the allow-list. The literal `"default"`
  key is required and is **not** an allowed country — it is only a price-resolution fallback.

A country may exist in `shop.Countries` without appearing in any product's `prices` map; the price falls back to
`prices.default`. To exercise the fallback in a test, pass an allowed country with no matching `prices` entry.

### Image path transformation

Image paths in YAML are bare filenames (`"thumb.jpg"`) and must be rewritten to URL-ready paths
(`"/images/{id}/thumb.jpg"`) before returning to the client. This must happen in **two** places:

1. **Loader** (`loadProduct`): rewrites paths after `validate` so `catalog.Products` has downloadable URLs.
   Validation runs against the bare filenames first (so `os.Stat` works).
2. **`ServeProductContent` handler**: reads `product.yaml` directly at request time (bypassing the loader), so it must
   apply the same transformation inline after YAML parsing, before building `ProductDetail`.

Any new handler that reads `product.yaml` directly must apply this transformation — do not assume the loader has
already rewritten the paths.

The same rule applies to `attr_images`: the loader stores bare filenames; `ServeProductContent` rewrites to
`/images/{id}/{filename}` when building `ProductDetail.AttrImages`. Path rewriting happens in the handler, not the
loader.

### File writes

The `Write` tool requires that the target file has been `Read` at least once in the same session before overwriting.
When writing many files in a single pass, `Read` each target first — even for files that will be completely replaced.

## Packages

### internal/handler

`internal/app` imports `internal/handler`; therefore **`internal/handler` must never import `internal/app`**. When the
handler or service needs config values from `app.Config`, pass scalar values at construction time instead of passing
the config struct.

The same rule applies to other internal packages (e.g. `internal/geo`, `internal/monobank`, `internal/novaposhta`,
`internal/order`): define a local interface in `handler` (e.g. `geoDetector`, `monobankClient`, `novaPoshtaClient`,
`orderService`) satisfied by the concrete type, and wire it in `internal/app`. This keeps handler decoupled from
implementation packages.

`NewHandler` parameter order (see `internal/handler/handler.go`):
`prod, pages, shopSvc, np, mb, mbVerifier, orders, geo, resp, dataDir, redirectURL, webhookURL, taxIDs, l`. The
trailing scalars (`dataDir`, `redirectURL`, `webhookURL`, `taxIDs`) are read out of `cfg` in `app.Run` and passed in
flat — handler must not import `internal/app` to read them.

`monobankVerifier` is a local interface in `handler.go` with one method `Verify(ctx, body, sigB64) error`. It is
satisfied by `*monobank.Verifier` and is used by the webhook handler to authenticate incoming Monobank callbacks.

The `orderService` interface in `order.go` has five methods: `Submit`, `AttachInvoice`, `List`, `GetStatus`,
`ApplyPaymentEvent`. The last two are needed by the webhook handler to query current order state and record payment
events.

### internal/geo

`geo.Detector.Detect(r)` resolves a country code in two steps:

1. **Header shortcut**: `CF-IPCountry` is validated as exactly two ASCII letters by `isAlpha2`. Any other value
   (multi-character, empty, garbage) is ignored and falls through to IP lookup. This prevents header spoofing from
   poisoning pricing decisions with garbage values.
2. **IP lookup**: extract client IP, check the in-memory cache (TTL 1h), and on a miss call
   `https://ipinfo.io/{ip}/country`. Result is lowercased and trimmed.

Cache writes only occur on success (non-empty country). A service error (non-200 or network failure) returns `""` and
is **not** cached, so the next call retries — a transient error must not silence retries for an hour. The lookup body
is capped at 16 bytes via `io.LimitReader` (a country code is at most 2 chars; the headroom protects against runaway
responses).

`clientIP` uses `X-Forwarded-For` only when `CF-IPCountry` is absent. In production behind Cloudflare the XFF path is
never reached; without Cloudflare, XFF is caller-controlled and can be spoofed.

`geo.NewDetector()` is the production constructor. Tests construct `*Detector` directly (same package) to inject a
`httptest.Server` URL via the unexported `httpClient` and `serviceURL` fields. The `errcheck` linter requires
`defer func() { _ = resp.Body.Close() }()` (not bare `defer resp.Body.Close()`).

### internal/novaposhta

`novaposhta.Client` wraps the Nova Poshta JSON API v2 (`POST https://api.novaposhta.ua/v2.0/json/`). Methods:
`SearchCities` (`Address.searchSettlements`), `SearchBranches` (`Address.getWarehouses`), `SearchStreets`
(`Address.searchSettlementStreets`).

**Branches and streets must use `SettlementRef`, not `CityRef`** — the `Ref` returned by `searchSettlements` is a
settlement ref, and the old `getCities` API uses a different ref that returns "City not found".

`NewClient(apiKey, serviceURL)` is the production constructor; pass `""` for `serviceURL` to use the default. Tests
construct `*Client` directly (same package) to inject `httptest.Server` via the unexported `httpClient` and
`serviceURL` fields — same pattern as `geo.Detector`. Response body capped at 1 MB via `io.LimitReader`.
`success=false` in the response is treated as an error regardless of HTTP status.

### internal/monobank

`monobank.Client` wraps the Monobank acquiring API. `CreateInvoice(ctx, CreateInvoiceRequest)` posts to
`/api/merchant/invoice/create`. `NewClient(apiKey, serviceURL)` is the production constructor; pass `""` for
`serviceURL` to use the default. Tests construct `*Client` directly to inject a `httptest.Server` via the unexported
`httpClient` and `serviceURL` fields — same pattern as `geo.Detector` and `novaposhta.Client`. Response body capped
at 1 MB.

`monobank.MapCurrency(code)` maps alpha-3 currency codes (case-insensitive) to ISO-4217 numeric. Unknown codes return
`*monobank.APIError{ErrCode: "unsupported_currency"}` so the handler can treat them like any other Monobank failure.

**Error policy: generic responses, detailed logs.** All Monobank-related failures (HTTP timeout, non-2xx, parse error,
application-level error, currency-map miss, tx2 failure) return HTTP 502 with body `{"error":"bad gateway"}` — never
the underlying detail. The handler logs the full detail with structured fields (`order_id`, `monobank_status`,
`monobank_err_code`, `monobank_err_text`, `monobank_body`, and where applicable `invoice_id`, `page_url`) using
`errors.As(err, &apiErr)` to extract `*monobank.APIError`. HTTP non-2xx and `errCode`/`errText` body fields both
produce `*monobank.APIError`.

`monobank.ParseWebhook(body []byte)` decodes a Monobank invoice-status webhook delivery. It validates that
`invoiceId`, `status`, and `reference` are non-empty and returns a `*WebhookPayload`. `WebhookPayload.RawBody` is a
**defensive copy** of the input bytes (via `append(json.RawMessage(nil), body...)`), safe to store as JSONB without
re-encoding even if the caller reuses the buffer. `WebhookPayload` carries no JSON struct tags — it is a domain
type; only the unexported `webhookBody` wire struct carries tags. Empty date strings produce zero `time.Time` (not an
error), tolerating future Monobank contract changes. Dates are parsed with `time.RFC3339`.

`monobank.Verifier` verifies Monobank webhook `X-Sign` headers. The merchant ECDSA-P256 public key is fetched from
`/api/merchant/pubkey` via `Fetch(ctx)` at startup and cached under `sync.RWMutex`. `Verify(ctx, body, sigB64)`
hashes the body with SHA-256 and verifies the base64-encoded ASN.1 ECDSA signature. On a verification failure the
key is refetched once and the check is retried — this auto-recovers from Monobank key rotation without operator
intervention. `ErrInvalidSignature` is the sentinel returned on persistent mismatch; callers map it to HTTP 401. A
transport or parse error during the lazy refetch is returned as-is (not collapsed to `ErrInvalidSignature`) so the
caller can distinguish a bad signature from an upstream failure. Concurrent refetches are deduped via an
`atomic.Bool` (`refetching`): only the first failing caller fetches; others spin on 10 ms ticks until the flag
clears, then re-verify against the freshly cached key. `NewVerifier(apiKey, serviceURL)` is the production
constructor (pass `""` for `serviceURL` to use the default). Tests construct `*Verifier` directly to inject an
`httptest.Server` URL, mirroring the `Client` / `geo.Detector` pattern.

### internal/orderdb

`orderdb.Writer` and `orderdb.Reader` own a `*pgxpool.Pool` (concrete, not an interface). The pool is created in
`internal/app/app.go` via `pgxpool.New(rt.Ctx, cfg.Database.DSN)` after `dbmigrator.RunPostgres` has applied the
embedded migrations from `internal/sql/`.

`Writer.Write` (called as `Submit` through `order.Service`) is **transactional**: order header + order_attrs + initial
`order_history (status='new')` all in one tx. A half-written order — header without attrs, or attrs/history without
header — would leave the customer's record ambiguous and reconciliation harder; atomicity is the whole reason these
inserts share a transaction. The deferred `Rollback` after a successful `Commit` is a no-op (`pgx.ErrTxClosed`,
ignored).

`MiddleName == ""` and `CustomerNote == ""` are converted to SQL NULL via the `nullIfEmpty` helper so the nullable
columns store NULL rather than the empty string.

`Attr.Name` and `Attr.Value` store **rendered titles** in the customer's language (e.g. `"Display color"` → `"Red"`),
not raw attribute/value IDs. This makes orders self-documenting and immutable to later product YAML renames; it is
the reason `handler.CreateOrder` resolves `attrLang.Title` and `attrVal.Title` before appending to the slice.

`order.Order` (write side) carries only what the writer must INSERT. `order.Record` (read side, returned by
`GET /orders`) adds the DB-generated columns (`ID`, `Status`, `CreatedAt`, `UpdatedAt`) and inline `Attrs`,
`History`, `Invoices`. The two are intentionally separate — different lifetimes, different fields, unifying them
would muddy both. Nullable text columns (`MiddleName`, `AdminNote`, `CustomerNote`, `HistoryEntry.Note`) are
`*string` so SQL NULL becomes JSON omission via `json:",omitempty"` rather than serialising as an empty string;
read-side scans use `pgtype.Text` and convert to `*string`.

#### No unit tests in `internal/orderdb`

The transactional writer needs `*pgxpool.Pool.Begin → pgx.Tx`. `pgx.Tx` is a wide interface (Exec, Query, QueryRow,
Commit, Rollback, CopyFrom, SendBatch, Prepare, …) that is impractical to stub by hand, and no mock package
(e.g. `pgxmock`) is vendored. Rather than introduce a leaky narrow interface or a heavy dep just for unit coverage,
both `Writer` and `Reader` are exercised end-to-end by `tests/api/order/{post,get}_test.go` against the real
PostgreSQL instance from `docker-compose.tests.yaml`. Don't reintroduce a stub-based unit test for this package
without also providing a real `pgx.Tx` implementation.

See also: **POST /orders, GET /orders** under Routes for the two-phase Monobank flow.

### internal/loader

A missing `data_dir` or a missing `products/` subdirectory is **not** an error — they yield an empty catalog. A
malformed YAML file or a validation error is **fatal** at startup. Validation runs inside `loadProduct` immediately
after YAML parsing. Product subdirectories that lack `product.yaml` are silently skipped, so extra directories can
coexist in the `products/` tree.

`{data_dir}/shop.yaml` is **mandatory**: a missing file is a fatal startup error, and `shop.countries` must be a
non-empty map. The wrapper struct `shopFile` is needed because the YAML root key is `shop:`, not the struct fields
directly. Functional and loader tests must therefore lay down a valid `shop.yaml` with at least one `countries`
entry, or the app refuses to boot.

`{data_dir}/products/products.yaml` is a flat list of `product.Item` entries (id, title, description) loaded by
`loadProductsList` into `catalog.ProductItems`. A missing file returns an empty slice — not an error. This is
separate from per-directory loading by `loadProducts`; `ProductItems` and `Products` coexist in `Catalog`.
`joinProductImages` then sets each `Item.Image` to the first preview URL of the matching `Product` (already
rewritten to `/images/{id}/{file}`), or leaves it nil.

Product validation lives in `internal/loader/loader.go:validate`. Rules are fatal at startup:

1. `name` must be non-empty.
2. `description` must be non-empty.
3. Language sets of `name` and `description` must be identical.
4. Every spec entry must cover exactly the languages in `name`.
5. `prices` must contain a `"default"` key.
6. Every attr entry must cover exactly the languages in `name`, and each language entry must have ≥ 1 value.
7. Every image `preview` and `full` path must exist on disk relative to `{productDir}/images/` (`os.Stat`,
   `errors.Is(err, fs.ErrNotExist)`). Same check is applied to every `attr_images` filename.

The `loadProducts` per-directory loader is no longer wired into any handler — it exists solely to enforce product
YAML integrity at startup.

## Routes

### `GET /products`, `GET /products/{id}/{lang}`

`ListProducts` returns the metadata loaded from `products/products.yaml` as a JSON array of
`{id, title, description, image?}`. The optional `image` is the URL of the first preview from the matching
`product.yaml`; `image` is omitted when nil. A missing `products.yaml` returns `[]`, not an error. The route goes
through the OpenAPI response middleware. `product.NewService` normalises a nil slice to `[]*Item{}` so the response
is a valid empty array (required by the OpenAPI response validator).

`ServeProductContent` reads `{data_dir}/products/{id}/product.yaml` directly at request time and returns a
lang-filtered `ProductDetail`. Both `id` and `lang` are validated with the reject pattern
`if v != filepath.Base(v) || v == "" || v == "."`. Missing product directory or `product.yaml` → 404; missing
language key in `p.Name` → 404.

Price selection uses `h.geo.Detect(r)` to get the country code, then looks up `p.Prices[country]` with
`p.Prices["default"]` as fallback. `ProductDetail.Prices` is a single `PriceItem`, not a map.

**Price field naming gotcha:** the YAML key is `prices:` and the Go field is `Product.Prices`, but the API response
serialises the resolved single-country price as `"price"` — `ProductDetail.Prices` carries the JSON tag
`json:"price"`.
The YAML key and JSON key are intentionally different.

### `GET /pages`, `GET /pages/{id}/{lang}`

`ListPages` returns the entries from `pages/pages.yaml` as a JSON array of `{id, title}`. Missing file → `[]`. Goes
through the OpenAPI response middleware.

`ServePage` reads `{data_dir}/pages/{id}/{lang}.md` and returns it as `text/plain; charset=utf-8`. Both path values
use the same reject pattern as `ServeProductContent`. **The route does NOT go through the OpenAPI response
middleware** (plain text, not JSON), but is declared in the spec for documentation. Missing file → 404.

### `GET /shop`

Returns the `shop.yaml` content (`*shop.Shop`) as JSON via the OpenAPI response middleware.

### `POST /orders`, `GET /orders`

`CreateOrder` validates the request, resolves the product and price, then runs a **two-phase committed flow**:

1. **tx1 (`Submit`)** — `INSERT orders (status='new')` + `order_attrs` + initial `order_history (new)`. The DB is
   the source of truth before Monobank knows about the order.
2. **Monobank call** — `monobank.CreateInvoice(...)`. On failure the order stays in `status='new'`; the caller gets
   HTTP 502 with body `{"error":"bad gateway"}`. Any orphan state (Monobank invoice without our row, or vice versa)
   is reconciled out of band — manually for now.
3. **tx2 (`AttachInvoice`)** — `INSERT order_invoices` + `UPDATE orders SET status='awaiting_payment'` +
   `INSERT order_history (awaiting_payment)`, all in one transaction.

Required request fields: `product_id, lang, first_name, last_name, phone, email, country, city, address`. Returns
201 `{"payment_url": "<monobank-page-url>"}` on success; 400 on missing/invalid fields; 404 on unknown product or
path traversal; 502 on tx1 failure, Monobank failure, or tx2 failure (always with body `{"error":"bad gateway"}`).

`country` is **request-supplied at order time, not geo-detected**. The same value drives both price resolution and
the `orders.country` column, so the stored country always matches the price tier the customer was quoted. Geo
detection (`h.geo`) is wired into the handler for `ServeProductContent` (browsing) but is not consulted by
`CreateOrder`. To exercise the `default` price fallback in tests, send an allowed country with no matching
`prices.<country>` key (empty values are rejected with 400).

`req.Country` is validated against `shop.Shop.Countries` (loaded from `shop.yaml`) by
`_, ok := s.Countries[req.Country]` **before the product YAML is read** so disallowed countries fail fast. A country
not in the allow-list returns 400 with `{"error": "invalid country"}`.

Total price = base price (`p.Prices[country]` with `default` fallback) + sum of per-attribute add-ons. Both base
and add-on use `int(math.Round(value * 100))` to convert to cents (see Conventions / Money values).

The Monobank `merchantPaymInfo.basketOrder` is **mandatory** — Monobank rejects invoices without it as
`INVALID_MERCHANT_PAYM_INFO` (required for fiscalization). The handler sends a single line item per invoice with
`name = p.Name[req.Lang]` (product title in customer's language), `qty = 1`, `sum = totalCents`,
`code = req.ProductID`, and `tax = h.taxIDs` (loaded from `cfg.Monobank.TaxIDs`). `Code` and `Tax` are also required
when fiscalization is enabled on the merchant account. Per Monobank's contract, the sum of all basket-item `sum`
values must equal the invoice `amount`; with one line item this is trivially satisfied.

Other Monobank invoice fields built per request: `merchantPaymInfo.reference = orderID`,
`destination = "<shop name in req.Lang>, order <orderID first 8 chars>"` (e.g. `"Acme, order 018f4e3a"`),
`redirectUrl = cfg.Monobank.RedirectURL + "?order_id=<orderID>"`.

`order.id` is generated by PostgreSQL `DEFAULT uuidv7()` and surfaced via `RETURNING id`; `Submit` returns this id
so the handler can pass it to Monobank as `merchantPaymInfo.reference` and to `AttachInvoice`.

The `order_invoices` table has **no unique constraint on `order_id`**, leaving room for re-issued invoices in a
future iteration. Its primary key is `(id, provider)` where `id TEXT` is the **provider's** invoice id (e.g. the
Monobank `invoiceId`) — there is no separate `invoice_id` column. `order.Invoice.ID` carries that value. The
composite PK means a test mocking a provider must return distinct ids per call when seeding multiple orders,
otherwise the second `AttachInvoice` insert violates the PK and the handler returns 502; see the counter-based mock
in `tests/api/order/get_test.go`.

`GET /orders` returns every persisted order with its attrs, history, and invoices — newest first. **It is registered
only when `cfg.Server.APIKey != ""`**; the handler itself does not check for an empty key. Because `POST /orders`
is registered unconditionally on the same path, an unregistered `GET /orders` produces HTTP 405 Method Not Allowed
(not 404) — Go 1.22+ stdlib mux returns 405 when a path is registered for some methods but not the requested one.
This is acceptable: the user-facing requirement is that the endpoint be unavailable when the key is unset, and 405
fits that. Do not add a runtime "401 / 404 if key empty" branch inside the handler — the conditional registration
in `app.go` is the single source of truth.

See also: **internal/orderdb** under Packages for the transactional writer, Reader fan-out, and no-unit-tests
rationale.

### `POST /monobank/webhook`

`MonobankWebhook` processes Monobank invoice-status webhook deliveries. Authenticated by `X-Sign` (ECDSA over the
body); no API key, no CORS, no OpenAPI middleware, no rate limiter — Monobank is the sole caller. Reads up to 1 MB,
verifies via `h.mbVerifier`, parses with `monobank.ParseWebhook`, looks up the order by `payload.Reference`, computes
the target `order_status` via `monobankStatusToOrderStatus`, and applies the transition only when
`shouldApply(current, target)` — equal rank or backwards is a silent no-op. On forward transition,
`order.Service.ApplyPaymentEvent` runs `UPDATE orders.status` + `INSERT order_history` with the verbatim payload in
one tx. Response codes: 200 for processed/idempotent/informational/unknown reference (permanent — Monobank stops
retrying); 401 for bad signature; 400 for malformed body; 500 for transient DB errors so Monobank retries.

State-machine ranks live in `orderStatusRank` in `internal/handler/monobank_webhook.go`. `cancelled` and fulfillment
`processing` deliberately share rank 5 — a late `failure` after the operator has begun fulfillment is informational
only and must not downgrade the order.

### `GET /nova-poshta/cities`, `/branches`, `/streets`

Three proxy routes over the Nova Poshta JSON API v2:

- `GET /nova-poshta/cities?q=<query>` → `Address.searchSettlements`
- `GET /nova-poshta/branches?city_ref=<ref>&q=<query>` → `Address.getWarehouses`
- `GET /nova-poshta/streets?city_ref=<ref>&q=<query>` → `Address.searchSettlementStreets`

`q` is required everywhere; `city_ref` is required for branches and streets — missing returns 400. NP API failure
returns 502 via `BadGatewayError`. The API key is read from `config.yaml` under `nova_poshta.api_key`.
`nova_poshta.service_url` is empty in production (uses the real NP URL) and set to a `httptest.Server` URL in
functional tests via `testapp.New` opts.

See also: **internal/novaposhta** under Packages for the SettlementRef-vs-CityRef gotcha.

### `GET /images/{product_id}/{file_name}`

Served by `handler.ServeImage`. Reads from `{data_dir}/products/{product_id}/images/{file_name}` using
`http.ServeFile`. `filepath.Base` is applied to both path values before joining to prevent path traversal.
`NewHandler` takes `dataDir string` (positional) so the handler can resolve image paths without importing
`internal/app`.

Functional tests for image serving must create a complete valid product directory (including `product.yaml`)
alongside the `images/` subdir if the test needs the product to appear in `catalog.Products`. If only the image file
itself is needed (file living under `{data_dir}/products/{id}/images/`), no `product.yaml` is required — the loader
silently skips directories that lack one.

## Configuration

Config keys live under `internal/app/config.go`. Notable required vs optional rules:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, and **`Monobank.WebhookURL`** are required — `app.Run` returns an
  error before the migrator if any is empty.
- **`Monobank.ServiceURL`** is optional; empty falls back to `https://api.monobank.ua/`. Tests inject a
  `httptest.Server.URL` via `testapp.New` opts.
- **`Monobank.TaxIDs`** is the list of merchant tax-registration IDs from the Monobank business cabinet (required
  when fiscalization is enabled). Wired into `NewHandler` as `taxIDs []int` and emitted on every basket item.
- **`Server.APIKey`** is optional; empty disables `GET /orders` via conditional route registration (see Routes).
- **`NovaPoshta.ServiceURL`** is optional; empty falls back to the production NP URL.
- **`RateLimit`** (top-level `rate_limit`): positive → that many RPM per IP; `0` or negative → rate limiting
  disabled (functional tests use `-1`).
- **`DataDir`** (top-level `data_dir`): default `./data`.

`testapp.New` defaults `Monobank.APIKey="test-key"`, `Monobank.RedirectURL="https://test.example/thanks"`, and
`Monobank.WebhookURL="https://test.example/monobank/webhook"` so plain construction works in tests. It also starts a
built-in `httptest.Server` (registered as `cfg.Monobank.ServiceURL`) that serves `/api/merchant/pubkey` so
`Verifier.Fetch` succeeds at startup without hitting real Monobank. Tests that need custom Monobank behaviour
override `cfg.Monobank.ServiceURL` via opts; the built-in stub is then unused but still shut down via `t.Cleanup`.

`uuidv7()` is a built-in PostgreSQL 18 function; the `orders` and `order_history` tables use
`id uuid PRIMARY KEY DEFAULT uuidv7()`. There is no separate `uuidv7` type. Earlier PG versions need an extension
(`pg_uuidv7`) or a custom `CREATE DOMAIN`. The test container in `docker-compose.tests.yaml` pins `postgres:18-alpine`
to satisfy this.

## Tests

### Running

Unit tests live alongside source code in each package. Run:

```
task go:test:unit -- [FLAGS]
```

Functional tests live in `tests/`, all files tagged `//go:build functest`. Run:

```
task go:test:func -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags (e.g. `-run TestName`, `-v`). In git worktrees where `.ci/` is not
initialized, run `go test` directly: `go test -run TestFoo -v ./internal/...` or
`go test -tags=functest ./tests/...`.

When the postgres state ends up dirty (e.g. an interrupted test), run `task go:test:func:clean` to wipe and recreate
the container.

### Unit tests

- Before implementing any feature or fix, invoke the `superpowers:test-driven-development` skill.
- Before claiming any work is done, invoke the `superpowers:verification-before-completion` skill and run **both**
  `task go:test:unit -- ./...` and `task go:test:func -- -v`. Both suites must pass — running only one is not enough.
- After all changes are made and tests pass, run `task go:golangci-lint`. All lint checks must pass before the task
  is considered complete.
- Do not consider a task complete until tests pass. Do not respond with a summary of changes before running tests.
- Group related tests under a single parent function `TestFoo(main *testing.T)` and use `main.Run("CaseName", ...)`
  for sub-tests. Never write separate top-level functions like `TestFoo_CaseName`.
- When adding a new file to a package that already has `_test.go` companions alongside source files (e.g.
  `internal/handler/`), write the unit test file as part of the same task. Do not wait to be reminded.
- When a handler response schema declares `id` with `format: uuid`, mock return values in handler unit tests must
  use valid UUID strings (e.g. `"018f4e3a-0000-7000-8000-000000000099"`). The OpenAPI response validator
  (`resp.Write`) rejects non-UUID strings and the handler returns HTTP 500, masking the actual assertion under test.

### Functional tests

`docker-compose.tests.yaml` defines a `postgres:18-alpine` service. The `tests` service depends on
`postgres: service_healthy` and exports
`APP_DB_DSN=postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable`. `testapp.New` reads `APP_DB_DSN`
(defaulting to the same value) and assigns it to `cfg.Database.DSN`. Tests can override the DSN via the `opts`
callback like any other config field. `(*App).DSN()` exposes the DSN so tests can open their own `pgxpool.Pool` to
inspect or mutate the database.

The postgres container persists across `task go:test:func` runs, so tests that need clean state must truncate
explicitly. `tests/api/order/post_test.go` uses a `truncateOrders` helper that runs
`TRUNCATE order_attrs, order_history, orders RESTART IDENTITY CASCADE` before each subtest that inspects the tables.
After a destructive test (e.g. forcing a DB error), restore the schema in `t.Cleanup` rather than leaving the
database in a broken state — `TestCreateOrder_DBFailure` does this by renaming `orders` away and back instead of
dropping it.

**`t.Cleanup` runs after `defer`, not before.** `t.Cleanup` callbacks fire after the test function returns *and after
all deferred statements run*. If a test does `defer pool.Close()` and then registers a cleanup that uses the pool,
the cleanup will silently fail because the pool is already closed. Either close the pool inside the cleanup callback,
or skip the `defer` and let the cleanup own teardown. This bit `TestCreateOrder_DBFailure`'s table-rename rollback
once already.

Functional tests use YAML fixture files. **Loader unit tests (`internal/loader/`):**

- `makeProductDir(t, dataDir, id, yaml, extraFiles)` creates `{dataDir}/products/{id}/product.yaml` and any extra
  files specified as `map[string][]byte` of relative path → content.
- `makeProductsFile(t, dataDir, content)` creates `{dataDir}/products/products.yaml` with the given content.

**API functional tests (`tests/api/product/`):**

- `makeDataDir(t, productsYAML, productYAMLs)` creates a temp data dir, writes `products/products.yaml` (if
  non-empty), and writes per-product `product.yaml` files given as `map[string]string` of `{id: yaml-content}`.
- `testapp.New(t, dataDir, opts ...func(*app.Config))` takes a `dataDir` and optional config mutators. Pass
  `func(cfg *app.Config) { cfg.NovaPoshta.ServiceURL = srv.URL }` to override config fields in tests.
- Subtests that need a completely separate empty catalog start their own `testapp` inside the subtest body — safe
  because each app binds to a random port.

API test orchestration:

- All API subtests within a `TestFoo` share one `testapp` instance started in the parent function. Starting a
  separate instance per subtest panics on port conflict when subtests run in parallel.
- At most one top-level `TestFoo` function per package may call `main.Parallel()` if each starts its own `testapp`.
  New API test functions that start their own app must NOT call `main.Parallel()` unless existing parallel tests are
  refactored to share a single app.
- Tests that need to issue many requests synchronously must set `cfg.RateLimit = -1` to disable rate limiting on
  `POST /orders`.

### Shared helpers location

Keep shared test helpers in `handler_test.go` (not in per-feature test files) so they survive feature removal.

### Shared ECDSA test key in `tests/api/order/`

The package has a single `testECDSAKey *ecdsa.PrivateKey` generated in `init()`. Two helpers derived from it:
`pubKeyPayload(t)` returns the PEM-encoded public key wrapped in the Monobank pubkey JSON payload, and
`signWebhookBody(t, body)` computes the ECDSA-SHA256 signature in the format `POST /monobank/webhook` expects.
All Monobank stub servers in the package's functional tests use `pubKeyPayload` for `/api/merchant/pubkey`. No test
function should generate its own ECDSA key — use the shared helpers instead.

### Webhook handler response policy

`MonobankWebhook` never calls `h.writeError` and never writes a JSON body. Monobank does not read error bodies, so
the response is **status code only, empty body** via plain `w.WriteHeader(...)`. The status codes are semantically
driven by retryability: permanent conditions (bad sig, malformed body, unknown reference, informational status,
idempotent/backwards transition) return 200 or 4xx so Monobank stops retrying. Transient conditions (verifier
transport error, DB error) return 500 so Monobank retries later. This is the only handler in the codebase that
deliberately returns 500 as a retry signal rather than as an error indicator.
