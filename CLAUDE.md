# Project Rules

## Meta

After completing any task, reflect on what was learned. If a decision, pattern, or constraint is non-obvious and likely
to recur, add it here under the appropriate section. Do not record things derivable from reading the code. **This is
mandatory** — update this file as the final step of every task before responding with a summary.

After implementing or changing a feature, update `README.md` if business logic worth describing changed (new entities,
concepts, endpoints, behavior). Wrap `README.md` lines at 120 characters.

Never run `git commit` or `git push` unless the user explicitly asks. When dispatching subagents, include "do not
commit" at the top of every subagent prompt.

When adding a field to an existing struct or method, read the current state of the file first and preserve all
existing fields and logic — including any added by the user outside of the tracked plan. Never use `replace_all` to
propagate a new field into test bodies without auditing every match: a "missing X" test must not have X added to it.

The `Write` tool requires the target file to have been `Read` at least once in the same session before overwriting.
When writing many files in one pass, `Read` each target first.

`data/` is a **symlink** pointing to `../websites/craft.d5y.xyz/data` — a separate git repository. Files under
`data/` (e.g. email templates, YAML) must be committed in that repo, not in `simshop`. `git add data/...` in the
`simshop` repo will fail with "beyond a symbolic link". Use `git -C /Users/ashep/src/my/websites/craft.d5y.xyz add ...`
instead.

`vendor/` is gitignored in this repo — it is regenerated locally via `go mod vendor` and never committed.
Only `go.mod` and `go.sum` are staged when adding/updating modules. `go mod tidy` will strip any module not reached
from a real import; to land a dep before its consumer exists, run `go get` (which records it as `// indirect` in
`go.mod`) and **skip `go mod tidy`** until the importer is added in a follow-up task. `go mod vendor` is also a no-op
for a module nothing imports, so the `vendor/` tree only gets populated once a `.go` file actually imports it.

## Project overview

`simshop` is a Go HTTP API service (`github.com/ashep/simshop`). Stack: filesystem catalog (products from YAML at
startup, no product DB), PostgreSQL for orders, zerolog, the `go-app` framework (`github.com/ashep/go-app`), and an
OpenAPI spec in `api/` for request/response validation.

Key directories: `internal/` (app, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product,
shop, sql), `api/`, `tests/` (build tag `functest`), `vendor/`. Config from `config.yml` + env; data from `data_dir`
(default `./data`). When implementing a new feature, use `api/` as additional context.

## Conventions

### Errors and status codes

Handlers map domain errors to HTTP via `h.writeError(w, err)`. Five typed errors in `internal/handler/handler.go`:
`BadRequestError` (400), `NotFoundError` (404), `BadGatewayError` (502, upstream failures), `UnauthorizedError` (401),
`ConflictError` (409, illegal state transition). Each has a `Reason string` field — always populate it; it's written
client-visibly as `{"error": "<reason>"}`.

Domain errors are defined in service packages; the handler maps them via `errors.Is`. Always use the most semantically
appropriate status (e.g. 404 for missing resource, not 400).

### OpenAPI

The validator (`kin-openapi`) runs in OpenAPI 3.0 compatibility mode:

- Don't use 3.1 array-type syntax (`type: ["string", "null"]`); use `type: string` + `nullable: true`.
- For UUID path params, use `type: string` + `format: uuid`, never `type: uuid` (panics at startup).
- For CSV-encoded query arrays (`?status=a,b`), use `in: query` + `style: form` + `explode: false` + `schema.type:
  array` with item-level `enum`. The validator decodes the CSV and rejects unknown values with 400 before the
  handler runs, so handlers should not redeclare the enum in Go.

### HTTP middleware

Routes use Go 1.22+ stdlib pattern syntax (`"GET /products"`). Middleware chain (innermost → outermost):
`content-type → openapi validation → handler`.

- **CORS** (`handler.CORSMiddleware`): unconditionally sets `Access-Control-Allow-Origin: *`. OPTIONS preflight (via
  `Access-Control-Request-Method`) is intercepted with 204. **Explicit `OPTIONS` routes must be registered alongside
  each `GET` route in `app.go`** — Go 1.22+ stdlib mux does not implicitly handle OPTIONS.
- **Rate limit** (`handler.RateLimitMiddleware(rpm)`): per-IP sliding window `60s/rpm`, 429 + `Retry-After`. Client IP
  from first `X-Forwarded-For` (Cloudflare) falling back to `RemoteAddr`. `cfg.RateLimit` (`rate_limit`): positive →
  RPM ceiling; **`0` or negative → disabled** (functests use `-1`).
- **API key** (`handler.APIKeyMiddleware`): requires `Authorization: Bearer <token>`, compared with
  `crypto/subtle.ConstantTimeCompare`. Two distinct 401 reasons (`"missing or invalid authorization header"`,
  `"invalid api key"`) written via the local `writeUnauthorized` helper, not `writeError`. Empty `apiKey` is a
  programmer error — only register protected routes in `app.go` when `cfg.Server.APIKey != ""`; do not push the empty
  check into the middleware.

### Money

Prices crossing the domain boundary are integer minor units (cents) — never float. Conversion:
`int(math.Round(value * 100))` for both base price and each attribute add-on. **Round once per amount; do not multiply
totals**, or rounding bias compounds.

### Country codes

Lowercase ISO 3166-1 alpha-2 throughout. Two distinct sets:

- **Allow-list** — `shop.Countries` (from `shop.yaml`). Authoritative for who can place orders. Validated by
  `CreateOrder` before reading product YAML.
- **Price keys** — `prices.<country>` in each `product.yaml`. Independent of allow-list. The literal `"default"` key
  is required and is **not** an allowed country — only a price-resolution fallback.

A country can be allowed without a per-product price; it falls back to `prices.default`. To exercise the fallback in a
test, pass an allowed country with no matching `prices` entry.

### Image path transformation

Image paths in YAML are bare filenames; clients receive `/images/{id}/<filename>`. Rewriting must happen in **two**
places:

1. **Loader** (`loadProduct`): rewrites paths after `validate` (validation needs bare filenames for `os.Stat`).
2. **`ServeProductContent` handler**: reads `product.yaml` directly at request time (bypasses the loader), so it
   applies the same transformation inline before building `ProductDetail`.

Same rule for `attr_images`. Any new handler that reads `product.yaml` directly must apply this transformation.

### Customer language on orders

`orders.lang` carries the customer's checkout language, populated from `req.Lang` on `POST /orders`. The customer
email notifier reads it to resolve `emails/{status}/{lang}.md`. Tests seeding rows directly into `orders` must
include a non-empty `lang` value (the column is `NOT NULL`).

## Packages

### internal/handler

`internal/app` imports `internal/handler`; therefore **`internal/handler` must never import `internal/app`**. When the
handler needs config, pass scalar values at construction time. Same rule for other internal packages
(`internal/geo`, `internal/monobank`, `internal/novaposhta`, `internal/order`): define a local interface in `handler`
(e.g. `geoDetector`, `monobankClient`, `monobankVerifier`, `novaPoshtaClient`, `orderService`) satisfied by the
concrete type, and wire it in `internal/app`.

`NewHandler` trims the trailing slash from `publicURL` once and stores `webhookURL = publicURL + "/monobank/webhook"`
on the handler — the Monobank webhook URL is **always derived**, never read from config.

`orderService` has six methods: `Submit`, `AttachInvoice`, `List`, `GetStatus`, `UpdateStatus`, `RecordInvoiceEvent`.

Do **not** add delegating methods on `order.Service` for every `Reader` method added to the `Reader` interface. Only
proxy methods that correspond to the `orderService` interface consumed by `internal/handler`. Other consumers (e.g.
background workers) receive `order.Reader` directly — a `Service` proxy is dead code and violates YAGNI.

`order.Service` holds an optional `Notifier` (5th arg to `NewService`; pass `nil` to disable). Dispatch pattern:
`Notify` is called **only after a successful commit**, guarded by `if s.n != nil`. `RecordInvoiceEvent` dispatches
only when the underlying `InvoiceEventWriter` returns a non-empty `newStatus` (i.e. a forward transition occurred).
`app.go` wires `*telegram.Notifier` when both `cfg.Telegram.Token` and `cfg.Telegram.ChatID` are set; passes `nil`
(notifier disabled) otherwise.

### internal/geo

`Detector.Detect(r)` resolves a country code in two steps:

1. **Header shortcut**: `CF-IPCountry`, validated as exactly two ASCII letters by `isAlpha2`. Anything else falls
   through. Prevents header spoofing.
2. **IP lookup**: client IP → in-memory cache (TTL 1h) → `https://ipinfo.io/{ip}/country` on miss. Lowercased and
   trimmed. Body capped at 16 bytes via `io.LimitReader`.

Cache writes only on success. Errors return `""` and are **not** cached, so retries aren't silenced for an hour.
`clientIP` uses `X-Forwarded-For` only when `CF-IPCountry` is absent (XFF is spoofable without Cloudflare).

`NewDetector()` is the production constructor. Tests construct `*Detector` directly to inject `httpClient` and
`serviceURL`. `errcheck` requires `defer func() { _ = resp.Body.Close() }()`.

### internal/novaposhta

`Client` wraps NP JSON API v2 (`POST https://api.novaposhta.ua/v2.0/json/`). Methods: `SearchCities`
(`Address.searchSettlements`), `SearchBranches` (`Address.getWarehouses`), `SearchStreets`
(`Address.searchSettlementStreets`).

**Branches and streets must use `SettlementRef`, not `CityRef`** — the old `getCities` API uses a different ref that
returns "City not found".

`NewClient(apiKey, serviceURL)` is the production constructor (empty `serviceURL` → default). Tests construct
`*Client` directly to inject `httpClient`/`serviceURL`. Body capped at 1 MB. `success=false` is treated as an error
regardless of HTTP status.

### internal/monobank

`Client.CreateInvoice` posts to `/api/merchant/invoice/create`. `NewClient(apiKey, serviceURL)` is the production
constructor (empty `serviceURL` → default); tests inject via `httpClient`/`serviceURL`. Body capped at 1 MB.

`MapCurrency(code)` maps alpha-3 (case-insensitive) to ISO-4217 numeric. Unknown codes return
`*APIError{ErrCode: "unsupported_currency"}` so the handler treats them like any Monobank failure.

**Error policy: generic responses, detailed logs.** All Monobank failures (timeout, non-2xx, parse, app-level error,
unsupported currency, tx2 failure) return HTTP 502 with body `{"error":"bad gateway"}` — never the underlying detail.
The handler logs full detail with structured fields (`order_id`, `monobank_status`, `monobank_err_code`,
`monobank_err_text`, `monobank_body`, plus `invoice_id`/`page_url` where applicable) using `errors.As` to extract
`*APIError`. Both HTTP non-2xx and `errCode`/`errText` body fields produce `*APIError`.

`ParseWebhook(body []byte)` decodes a Monobank invoice-status webhook. It validates `invoiceId`, `status`, and
`reference` are non-empty. `WebhookPayload.RawBody` is a **defensive copy** (`append(json.RawMessage(nil), body...)`),
safe to store as JSONB. `WebhookPayload` has no JSON struct tags (domain type); only the unexported wire struct does.
Empty date strings produce zero `time.Time` (not an error). Dates parsed with `time.RFC3339`.

`Verifier` verifies webhook `X-Sign` (ECDSA-P256 over SHA-256, base64 ASN.1). Pubkey fetched from
`/api/merchant/pubkey` at startup, cached under `sync.RWMutex`. **On verification failure the key is refetched once
and the check retried** — auto-recovers from key rotation. `ErrInvalidSignature` is returned on persistent mismatch
(handler maps to 401). Transport/parse errors during refetch are returned as-is so callers can distinguish bad sig
from upstream failure. Concurrent refetches deduped via `atomic.Bool` (`refetching`); other callers spin on 10 ms
ticks until the flag clears, then re-verify. `NewVerifier(apiKey, serviceURL)` is production; tests construct
directly.

### internal/telegram

`Client.SendMessage` posts to `/bot<token>/sendMessage`. `NewClient(token, serviceURL)` is the production constructor
(empty `serviceURL` → `https://api.telegram.org`); tests construct `*Client` directly to inject `httpClient`/`serviceURL`.
Body capped at 1 MB.

`*APIError` is returned on any non-2xx response. `APIError.RetryAfter` is non-zero only on 429 responses that include
`parameters.retry_after`. Transport errors (timeout, connection refused) are wrapped plain — callers distinguish them
from `*APIError` via `errors.As`.

`Notifier` implements `order.Notifier`. `NewNotifier(client, chatID, reader, log)` returns a stopped notifier; call
`Start()` before the first `Notify` call and `Stop()` on shutdown. `Notify` is non-blocking: if the 256-slot buffer is
full or the channel is closed (post-Stop race), the event is dropped with a Warn log and the call returns immediately.
`Stop()` closes the channel and waits up to 5s for the worker to drain. `messageSender` is a local interface satisfied
by `*Client`; tests use `fakeSender`. `sleepFn` is injected in tests to eliminate real sleeps. Retry policy: up to 3
attempts; 4xx (except 429) → permanent, no retry; 429 → honor `APIError.RetryAfter` clamped to `[1s, 30s]`; other
errors → `backoffSchedule[attempt-1]` (1s, 2s, 4s). `formatMessage` uses U+2014 em-dash (`—`), uppercases currency and
country, omits optional lines (`Customer note:`, `Status note:`) when empty.

### internal/resend

`Client.SendEmail` POSTs to `{serviceURL}/emails`. `NewClient(apiKey, serviceURL)` is the production constructor
(empty `serviceURL` → `https://api.resend.com`); tests construct directly to inject `httpClient`/`serviceURL`. Body
capped at 1 MB.

`*APIError` is returned on any non-2xx response. `APIError.RetryAfter` is non-zero only on 429 responses that include
a `Retry-After` header. Transport errors (timeout, connection refused) are wrapped plain — callers distinguish them
from `*APIError` via `errors.As`.

`Notifier` implements `order.Notifier`. Status filter at the top of `handle()`: only `paid`, `shipped`, `delivered`,
`refund_requested`, `refunded` are dispatched; anything else (especially `new`, `awaiting_payment`,
`payment_processing`, `payment_hold`, `cancelled`, `processing`, `returned`) is silently dropped before any DB read
or template lookup. `NewNotifier(client, from, orderURL, reader, products, shop, templates, log)` returns a stopped
notifier; call `Start()` before the first `Notify` and `Stop()` on shutdown. `Notify` is non-blocking: full buffer
(256) or post-Stop closed channel → drop with Warn log. Retry policy mirrors the Telegram notifier exactly: up to 3
attempts; 4xx (except 429) → permanent, no retry; 429 → honor `APIError.RetryAfter` clamped `[1s, 30s]`; other
errors → `backoffSchedule[attempt-1]` (1s, 2s).

`TemplateStore` (built by `LoadTemplates(dir)`) holds parsed templates per `(status, lang)`. `Render(status, lang,
data)` returns `(subject, html, text, error)`. Markdown body is rendered to HTML by `goldmark` AFTER `text/template`
substitution so customer names containing markdown-reserved characters aren't mangled. Lang fallback: missing
`(status, lang)` falls back to `(status, "en")`; missing both is an error. The plain-text body returned alongside
HTML is the post-template Markdown (good enough for Resend's deliverability tracker — the customer reads the HTML
view).

Templates live at `{data_dir}/emails/{status}/{lang}.md` with a YAML frontmatter block whose `subject` field is
required and non-empty. `internal/loader` parses the directory; `internal/app.validateEmailTemplates` enforces that
`en.md` exists for every notify-on status when `cfg.Resend.APIKey != ""`, aggregating all missing paths into a
single error. The loader itself stays policy-free.

`shop.Service.Name(lang string) string` was added to satisfy the notifier's `shopLookup` interface (returns the
shop name in the requested language with alphabetical-fallback to whichever language is defined). The notifier
relies on this fallback rather than performing its own en-fallback dance.

### internal/order

`MultiNotifier` (in `internal/order/notifier.go`) is a tiny fanout combinator over `[]Notifier`. `app.Run` wraps the
enabled notifiers in it only when 2+ are configured; with one it passes the child directly, with zero it passes nil.
A panic in any child is recovered silently and does not skip later children — children are expected to log
internally if they care.

`ErrTransitionNotAllowed` is returned by `Service.UpdateStatus` and `OperatorWriter.UpdateStatusByOperator` when the
target status is not a legal next step from the current status. The handler maps it to 409.

`OperatorWriter.UpdateStatusByOperator` returns `(applied bool, err error)`. `applied=false` with `nil` error means
the order was already at the target status under the SELECT FOR UPDATE lock (concurrent same-target write); the
caller must NOT dispatch a notification in that case. `Service.NewService` now takes `ow OperatorWriter` as the fifth
positional argument (before the notifier); pass `nil` only in tests that do not exercise `UpdateStatus`.

### internal/orderdb

`Writer` and `Reader` own a `*pgxpool.Pool` (concrete, not interface). The pool is created in `app.Run` after
`dbmigrator.RunPostgres` applies migrations from `internal/sql/`.

`Writer.Write` (called as `Submit` through `order.Service`) is **transactional**: order header + order_attrs +
initial `order_history (status='new')` in one tx. The deferred `Rollback` after `Commit` is a no-op.

Empty `MiddleName`/`CustomerNote` are converted to SQL NULL via `nullIfEmpty` so nullable columns store NULL.

`Attr.Name` and `Attr.Value` store **rendered titles** in the customer's language (e.g. `"Display color"` → `"Red"`),
not raw IDs. Orders are self-documenting and immutable to later YAML renames; this is why `handler.CreateOrder`
resolves `attrLang.Title` and `attrVal.Title` before appending.

`order.Order` (write side) carries only INSERT fields; `order.Record` (read side, from `GET /orders`) adds DB-generated
columns and inline `Attrs`/`History`/`Invoices`. The two are intentionally separate. Nullable text columns are
`*string` (with `json:",omitempty"`) so SQL NULL becomes JSON omission; read-side scans use `pgtype.Text` and convert.

`Writer.UpdateStatusByOperator` writes `tracking_number` write-once via `CASE WHEN $3 = '' THEN tracking_number ELSE
$3 END` — passing an empty string preserves whatever is already stored. The handler enforces "tracking required iff
`shipped`" so this SQL guard is a defense-in-depth, not the policy.

#### `RecordInvoiceEvent` and the invoice timeline

`RecordInvoiceEvent(ctx, evt)` is the single entry point for provider invoice events. One transaction:

1. `SELECT … FOR UPDATE` on `orders` — serializes concurrent webhooks for the same order. Missing → `order.ErrNotFound`.
2. `INSERT invoice_history (...) ON CONFLICT (invoice_id, provider, status, event_at) DO NOTHING` — idempotent on the
   provider's dedupe key. Duplicate webhook silently no-ops.
3. `SELECT status, note FROM invoice_history WHERE order_id = … ORDER BY event_at DESC, created_at DESC LIMIT 1` —
   re-derives current invoice status from the timeline (not just the row we may have inserted). This gives
   out-of-order webhook tolerance.
4. Map `invoice_status → order_status` via `order.InvoiceStatusToOrderStatus`. Apply iff
   `order.ShouldApplyInvoiceTransition(current, candidate)` returns true; the apply does
   `UPDATE orders.status` + `INSERT order_history` carrying the latest event's note.

`evt.Payload` must be non-empty (sanity check); `RecordInvoiceEvent` errors before opening the tx otherwise.

`order_history.payload` was removed — payloads live exclusively on `invoice_history.payload` (audit trail). To inspect
the verbatim provider payload that triggered an order_status change, query `invoice_history` ordered by `event_at`.

The `invoice_status` enum: `processing, hold, paid, failed, reversed`. Monobank webhook statuses map via
`monobankStatusToInvoiceStatus` (`success → paid`, `failure|expired → failed`, `created → no row` informational).

#### No unit tests in `internal/orderdb`

`pgx.Tx` is a wide interface impractical to stub by hand, and no mock package is vendored. Both `Writer` and `Reader`
are exercised end-to-end by `tests/api/order/` against the real PG instance from `docker-compose.tests.yaml`. Don't
reintroduce a stub-based unit test without a real `pgx.Tx`.

### internal/loader

A missing `data_dir` or `products/` subdir is **not** an error (empty catalog). Malformed YAML or validation failure
is **fatal at startup**. Validation runs inside `loadProduct` immediately after parsing. Product subdirs without
`product.yaml` are silently skipped.

`{data_dir}/shop.yaml` is **mandatory**: missing → fatal startup error, and `shop.countries` must be non-empty.
Functional and loader tests must lay down a valid `shop.yaml` with at least one `countries` entry.

`{data_dir}/products/products.yaml` is a flat list of `product.Item` entries (id, title, description), loaded into
`catalog.ProductItems`. A missing file → empty slice. Separate from per-directory `loadProducts`; both coexist in
`Catalog`. `joinProductImages` then sets each `Item.Image` to the first preview URL of the matching `Product`
(already rewritten), or leaves it nil.

Product validation in `validate` (fatal at startup):

1. `name` non-empty.
2. `description` non-empty.
3. Language sets of `name` and `description` must be identical.
4. Every spec entry must cover exactly the languages in `name`.
5. `prices` must contain `"default"`.
6. Every attr entry must cover exactly the languages in `name`, with ≥ 1 value per language.
7. Every image `preview`/`full` and every `attr_images` filename must exist on disk relative to
   `{productDir}/images/`.

`loadProducts` is no longer wired into any handler — it exists solely to enforce YAML integrity at startup.

## Routes

### `GET /products`, `GET /products/{id}/{lang}`

`ListProducts` returns `products/products.yaml` metadata as `[{id, title, description, image?}]`. `image` is the URL of
the first preview from the matching `product.yaml`; omitted when nil. Missing `products.yaml` → `[]`. `product.NewService`
normalises a nil slice to `[]*Item{}` (required by the OpenAPI response validator).

`ServeProductContent` reads `{data_dir}/products/{id}/product.yaml` directly and returns a lang-filtered
`ProductDetail`. `id` and `lang` are validated with `if v != filepath.Base(v) || v == "" || v == "."`. Missing
directory/file → 404; missing language key in `p.Name` → 404.

Price selection: `h.geo.Detect(r)` → country, then `p.Prices[country]` with `p.Prices["default"]` fallback.
`ProductDetail.Prices` is a single `PriceItem`, not a map.

**Price field naming gotcha:** YAML key is `prices:`, Go field is `Product.Prices`, but the API response serialises
the resolved single-country price as `"price"` — `ProductDetail.Prices` carries `json:"price"`. The YAML key and JSON
key are intentionally different.

### `GET /pages`, `GET /pages/{id}/{lang}`

`ListPages` returns `pages/pages.yaml` entries as `[{id, title}]`. Missing → `[]`.

`ServePage` reads `{data_dir}/pages/{id}/{lang}.md` as `text/plain; charset=utf-8`. Same path-reject pattern as
`ServeProductContent`. **Does NOT go through OpenAPI response middleware** (plain text, not JSON), but is declared in
the spec for documentation. Missing → 404.

### `GET /shop`

Returns `shop.yaml` content (`*shop.Shop`) as JSON.

### `POST /orders`, `GET /orders`

`CreateOrder` validates the request, resolves product/price, then runs a **two-phase committed flow**:

1. **tx1 (`Submit`)** — `INSERT orders (status='new')` + `order_attrs` + `order_history (new)`. DB is the source of
   truth before Monobank knows about the order.
2. **Monobank call** — `monobank.CreateInvoice`. On failure the order stays `new`; caller gets 502. Orphan state
   (Monobank invoice without our row, or vice versa) is reconciled out of band — manually for now.
3. **tx2 (`AttachInvoice`)** — `INSERT order_invoices` + `UPDATE orders SET status='awaiting_payment'` + `INSERT
   order_history (awaiting_payment)`, in one transaction.

Required fields: `product_id, lang, first_name, last_name, phone, email, country, city, address`. Returns
`201 {"payment_url": ...}`; 400 on missing/invalid; 404 on unknown product or path traversal; 502 on tx1/Monobank/tx2
failure (always `{"error":"bad gateway"}`).

`country` is **request-supplied, not geo-detected**. Drives both price resolution and the `orders.country` column, so
the stored country always matches the price tier the customer was quoted. Geo (`h.geo`) is wired for
`ServeProductContent` only. To exercise the `default` fallback in tests, pass an allowed country with no matching
`prices.<country>` (empty values are 400-rejected).

`req.Country` is validated against `shop.Countries` **before reading product YAML** (`_, ok := s.Countries[req.Country]`).
Disallowed → 400 `{"error": "invalid country"}`.

Total = base price (`p.Prices[country]` with `default` fallback) + sum of per-attribute add-ons. Both use
`int(math.Round(value * 100))` (see Money).

The Monobank `merchantPaymInfo.basketOrder` is **mandatory** (`INVALID_MERCHANT_PAYM_INFO` otherwise). One line item
per invoice: `name = "<product title>"` when no attrs, or `name = "<product title> (<Attr1>: <Val1>, <Attr2>:
<Val2>)"` (titles in `req.Lang`, attrs in the same alphabetical order as the persisted `order.Attrs` slice). Other
fields: `qty = 1`, `sum = totalCents`, `code = req.ProductID`, `tax = h.taxIDs` (from `cfg.Monobank.TaxIDs`). Per
Monobank's contract, sum of basket-item `sum` values must equal invoice `amount`.

The optional basket-item `icon` is `<server.public_url>/images/<product_id>/<images[0].preview>` when
`len(p.Images) > 0 && p.Images[0].Preview != ""`. Missing images silently omit `icon`. The icon URL is built from the
bare filename — same path the `GET /images/{product_id}/{file_name}` route serves — so `public_url` must point at this
service.

Other Monobank invoice fields per request: `merchantPaymInfo.reference = orderID`,
`destination = "<shop name in req.Lang>, order <orderID first 13 chars>"` (UUIDv7 timestamp prefix —
`xxxxxxxx-xxxx`),
`redirectUrl = cfg.Monobank.RedirectURL + "?order_id=<orderID>"`.

`order.id` is generated by PostgreSQL `DEFAULT uuidv7()`, surfaced via `RETURNING id`.

`order_invoices` has **no unique constraint on `order_id`** (room for re-issued invoices). PK is `(id, provider)`
where `id TEXT` is the **provider's** invoice id — no separate `invoice_id` column. A test mocking a provider must
return distinct ids per call when seeding multiple orders, or the second `AttachInvoice` violates the PK and the
handler returns 502; see the counter-based mock in `tests/api/order/get_test.go`.

`GET /orders` returns every order with attrs/history/invoices, newest first. **Registered only when
`cfg.Server.APIKey != ""`**; the handler does not check the key. Because `POST /orders` is registered unconditionally
on the same path, an unregistered `GET /orders` produces HTTP 405 (Go 1.22+ stdlib mux returns 405 when a path is
registered for some methods but not the requested one). Acceptable. Do not push the empty-key check into the handler;
the conditional registration in `app.go` is the single source of truth.

### `GET /orders/{id}`

`GetOrderStatus` returns `{"status": "<order_status>"}` for one order by UUIDv7. The path param is validated by the
OpenAPI request middleware (`format: uuid`) — malformed → 400 before the handler. Valid UUID not in DB → 404
`{"error": "order not found"}`. No API key (public endpoint). Maps `order.ErrNotFound` to `&NotFoundError{...}`.

### `POST /monobank/webhook`

`MonobankWebhook` processes invoice-status webhooks. Authenticated by `X-Sign`; no API key, no CORS, no OpenAPI
middleware, no rate limiter — Monobank is the sole caller. Reads up to 1 MB, verifies via `h.mbVerifier`, parses with
`monobank.ParseWebhook`, then maps `payload.Status` → `invoice_status` via `monobankStatusToInvoiceStatus`
(`created` is informational and short-circuits to 200 with no DB write). Builds an `order.InvoiceEvent{...,
Payload: payload.RawBody, EventAt: payload.ModifiedDate}` and calls `orders.RecordInvoiceEvent`. The service /
`internal/orderdb` does the rest.

**Response policy: status code only, empty body** via `w.WriteHeader(...)`. Never calls `h.writeError`. Codes are
driven by retryability: 200 for processed/idempotent/informational/unknown-reference (permanent — Monobank stops
retrying); 401 for bad signature; 400 for malformed body; **500 for transient DB or verifier-transport errors so
Monobank retries**. This is the only handler that deliberately returns 500 as a retry signal rather than an error.

`event_at` comes from Monobank's `modifiedDate` (not our `CURRENT_TIMESTAMP`) — gives order-independence: out-of-order
webhooks resolve correctly because "latest event" is decided by the provider's authoritative timestamp. Lets
retry-after-failure work: `processing@t1 → failure@t2 → processing@t3 → success@t4` correctly drives to `paid`.

The lifecycle rule is `order.ShouldApplyInvoiceTransition(current, candidate)` in `internal/order/lifecycle.go`.
Summary: invoice events freely drive the pre-paid cluster {`new`, `awaiting_payment`, `payment_processing`,
`payment_hold`, `cancelled`}; `cancelled` is re-enterable on retry; `paid` is stable against payment_* and `cancelled`
(only `refunded` moves it forward); fulfillment states (`processing`, `shipped`, `delivered`, `refund_requested`,
`returned`) are operator-owned and ignore invoice events except for `refunded`, which always wins.

### `GET /nova-poshta/cities`, `/branches`, `/streets`

Three proxy routes:

- `GET /nova-poshta/cities?q=<query>` → `Address.searchSettlements`
- `GET /nova-poshta/branches?city_ref=<ref>&q=<query>` → `Address.getWarehouses`
- `GET /nova-poshta/streets?city_ref=<ref>&q=<query>` → `Address.searchSettlementStreets`

`q` is required everywhere; `city_ref` is required for branches and streets — missing → 400. NP failure → 502 via
`BadGatewayError`.

### `GET /images/{product_id}/{file_name}`

`handler.ServeImage` reads from `{data_dir}/products/{product_id}/images/{file_name}` via `http.ServeFile`.
`filepath.Base` is applied to both path values before joining (path-traversal guard).

Functest: only the image file under `{data_dir}/products/{id}/images/` is required to serve it. A `product.yaml` is
needed only if the product must also appear in `catalog.Products` — the loader silently skips directories without it.

## Configuration

Config keys in `internal/app/config.go`. Required vs optional:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, **`Server.PublicURL`** — required; empty → startup error.
- **`Server.PublicURL`** is the public https base URL of this service. Used to derive the Monobank webhook URL
  (`<PublicURL>/monobank/webhook`, sent as `webHookUrl` on every `CreateInvoice`) and the basket-item icon
  (`<PublicURL>/images/<product_id>/<preview>`). Trailing slash trimmed once in `NewHandler`. **There is no separate
  `monobank.webhook_url` config** — the webhook target is always derived.
- **`Monobank.ServiceURL`**, **`NovaPoshta.ServiceURL`**, **`Telegram.ServiceURL`** — optional; empty → real upstream.
  Tests inject `httptest.Server.URL` via `testapp.New` opts.
- **`Telegram.Token`** + **`Telegram.ChatID`** — both optional; both empty → notifier disabled; exactly one set →
  startup error `"telegram: token and chat_id must be set together"`; both set → notifier enabled. `defer tn.Stop()`
  must be registered **after** `defer db.Close()` so LIFO order drains pending events while the pool is still open.
- **`Monobank.TaxIDs`** — list of merchant tax-registration IDs (required when fiscalization is enabled). Wired into
  `NewHandler` as `taxIDs []int` and emitted on every basket item.
- **`Server.APIKey`** — optional; empty disables `GET /orders` via conditional registration.
- **`RateLimit`** — positive → RPM/IP; `0`/negative → disabled (functests use `-1`).
- **`DataDir`** — default `./data`.
- **`Resend.APIKey`** — optional; empty disables the customer email notifier.
- **`Mail.From`** — sender address, required when `Resend.APIKey` is set; empty + `Resend.APIKey` non-empty →
  startup error.
- **`Mail.OrderURL`** — customer-facing URL pattern with literal `{id}`, required when `Resend.APIKey` is set;
  empty + `Resend.APIKey` non-empty → startup error.
- **`Resend.ServiceURL`** — optional; empty → `https://api.resend.com`. Tests inject via `httptest.Server.URL`.

`testapp.New` defaults `Monobank.APIKey="test-key"`, `Monobank.RedirectURL="https://test.example/thanks"`,
`Server.PublicURL="https://test.example"` so plain construction works in tests. It also starts a built-in
`httptest.Server` registered as `cfg.Monobank.ServiceURL` that serves `/api/merchant/pubkey` so `Verifier.Fetch`
succeeds at startup. Tests overriding `cfg.Monobank.ServiceURL` get their own stub; the built-in is unused but still
shut down via `t.Cleanup`.

`uuidv7()` is a built-in PostgreSQL 18 function (no extension); `orders` and `order_history` use
`id uuid PRIMARY KEY DEFAULT uuidv7()`. The test container in `docker-compose.tests.yaml` pins `postgres:18-alpine`.

## Tests

### Running

```
task go:test:unit -- [FLAGS]
task go:test:func -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags. In git worktrees where `.ci/` is not initialized, run `go test` directly:
`go test -run TestFoo -v ./internal/...` or `go test -tags=functest ./tests/...`.

When postgres state ends up dirty, run `task go:test:func:clean` to wipe and recreate.

### Unit tests

- Before implementing any feature or fix, invoke `superpowers:test-driven-development`.
- Before claiming work is done, invoke `superpowers:verification-before-completion` and run **both**
  `task go:test:unit -- ./...` and `task go:test:func -- -v`. Both suites must pass.
- After all tests pass, run `task go:golangci-lint`. All lint must pass before the task is complete.
- Do not respond with a summary before tests pass.
- Group related tests under one parent: `TestFoo(main *testing.T)` + `main.Run("CaseName", ...)`. Never write
  separate top-level `TestFoo_CaseName` functions.
- When adding a new file to a package that already has `_test.go` companions (e.g. `internal/handler/`), write the
  unit test file in the same task. Don't wait to be reminded.
- When a handler response declares `id` with `format: uuid`, mock return values must be valid UUID strings
  (e.g. `"018f4e3a-0000-7000-8000-000000000099"`) — the OpenAPI response validator rejects non-UUIDs, the handler
  returns 500, and your real assertion is masked.

### Functional tests

`docker-compose.tests.yaml` defines `postgres:18-alpine`. The `tests` service depends on `postgres: service_healthy`
and exports `APP_DB_DSN=postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable`. `testapp.New` reads
`APP_DB_DSN` and assigns it to `cfg.Database.DSN`; tests can override via the `opts` callback. `(*App).DSN()` exposes
the DSN so tests can open their own pool.

The postgres container persists across `task go:test:func` runs — tests needing clean state must truncate explicitly.
`tests/api/order/post_test.go` has a `truncateOrders` helper (`TRUNCATE order_attrs, order_history, orders RESTART
IDENTITY CASCADE`). After a destructive test (e.g. forcing a DB error), restore the schema in `t.Cleanup` — don't
leave the DB broken. `TestCreateOrder_DBFailure` does this via `RENAME` rather than `DROP`.

**`t.Cleanup` runs after `defer`, not before.** If a test does `defer pool.Close()` and then registers a cleanup using
the pool, the cleanup silently fails. Either close the pool inside the cleanup, or skip the `defer` and let the
cleanup own teardown.

Test fixture helpers — **loader unit tests (`internal/loader/`)**:

- `makeProductDir(t, dataDir, id, yaml, extraFiles)` — creates `products/{id}/product.yaml` plus extra files.
- `makeProductsFile(t, dataDir, content)` — writes `products/products.yaml`.

**API functional tests (`tests/api/product/`)**:

- `makeDataDir(t, productsYAML, productYAMLs)` — temp data dir with `products/products.yaml` (if non-empty) and
  per-product YAMLs as `map[string]string`.
- `testapp.New(t, dataDir, opts ...func(*app.Config))` — config mutators override fields, e.g.
  `func(cfg *app.Config) { cfg.NovaPoshta.ServiceURL = srv.URL }`. **`testapp.New` does not start the app**; tests
  must call `a.Start()` before issuing requests, or every HTTP call fails with `connect: connection refused`.
- Subtests needing a fully separate empty catalog start their own `testapp` — safe because each binds a random port.

API test orchestration:

- All API subtests within a `TestFoo` share one `testapp` started in the parent function. Per-subtest instances
  panic on port conflict under `main.Parallel()`.
- At most one top-level `TestFoo` per package may call `main.Parallel()` if each starts its own `testapp`. New API
  test functions starting their own app must NOT call `main.Parallel()` unless the existing parallel tests are
  refactored to share one app.
- Tests issuing many synchronous requests must set `cfg.RateLimit = -1` to disable rate limiting on `POST /orders`.

### Shared helpers location

Keep shared test helpers in `handler_test.go`, not per-feature test files, so they survive feature removal.

### Shared ECDSA test key in `tests/api/order/`

A single `testECDSAKey *ecdsa.PrivateKey` is generated in `init()`. Helpers: `pubKeyPayload(t)` returns the PEM-encoded
public key wrapped in the Monobank pubkey JSON payload; `signWebhookBody(t, body)` computes the ECDSA-SHA256 signature
in the format `POST /monobank/webhook` expects. All Monobank stub servers in this package must use `pubKeyPayload` for
`/api/merchant/pubkey`. No test should generate its own ECDSA key — use the shared helpers.
