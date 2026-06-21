# Project Rules

## Meta

- After any task, record a non-obvious decision/pattern/constraint likely to recur under the right section here. Don't
  record what's derivable from reading the code. **Mandatory** — final step of every task, before the summary.
- After a feature change, update `README.md` if business logic worth describing changed (entities, concepts, endpoints,
  behavior). Wrap `README.md` at 120 chars.
- Commit messages NEVER mention yourself; they must read as a regular person's work.
- Adding a field to an existing struct/method: read the file first, preserve all existing fields/logic (incl. user
  edits outside the plan). Never `replace_all` a field into test bodies without auditing every match — a "missing X"
  test must not gain X.
- `Write` requires the target be `Read` first this session; `Read` each target before a multi-file write pass.
- `data/` is a **symlink** to `../websites/craft.d5y.xyz/data` (separate git repo). Commit files under it there:
  `git -C /Users/ashep/src/my/websites/craft.d5y.xyz add ...`. `git add data/...` here fails ("beyond a symbolic link").
- `vendor/` is gitignored (regen via `go mod vendor`); only `go.mod`/`go.sum` are staged. `go mod tidy` strips modules
  no real import reaches — to land a dep before its consumer, `go get` it (records `// indirect`) and **skip `go mod
  tidy`** until the importer lands. `go mod vendor` populates `vendor/` only once a `.go` file imports the module.

## Project overview

`simshop` is a Go HTTP API (`github.com/ashep/simshop`). Stack: filesystem catalog (products from YAML at startup, no
product DB), PostgreSQL for orders, zerolog, `go-app` framework (`github.com/ashep/go-app`), OpenAPI spec in `api/` for
request/response validation. Config from `config.yml` + env; data from `data_dir` (default `./data`). Use `api/` as
context for new features.

Dirs: `internal/` (app, cli, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product, resend,
shop, sql, telegram), `api/`, `frontend/` (static storefront, served by Caddy — see Frontend & deployment), `tests/`
(build tag `functest`), `vendor/`. Server entrypoint is repo-root `main.go`; the CLI is a separate binary
(`cmd/simshop/main.go`).

### CLI (`cmd/simshop` + `internal/cli`)

Operator CLI, a **separate binary** from the server (`cmd/simshop/main.go` is thin: builds `cli.NewRootCmd()` +
`Execute()`s). `go install github.com/ashep/simshop/cmd/simshop@latest` yields `simshop`. All logic in `internal/cli`
(cobra; `spf13/cobra` is a direct dep). Pure HTTP client over the API — unit + `httptest` tests only, no functest.

`~/.simshop.yaml` (config): top-level mapping shop-name → `{url, api_key, default?}` (`url`+`api_key` required). Parsed
via `yaml.Node` (not a plain map) to preserve insertion order. Shop precedence: `--shop` flag → `default: true` →
**first in file order**. `Config.Select("")` → default; `Config.Select(name)` → named or error listing all names.
`DefaultConfigPath()` prefers `.yaml` over `.yml`.

Commands: `order list [--status csv]`, `order get <id>`, `order set-status <status> <id> [<id>...]`, `shops`.

- `order list` (GET /orders) with no `--status` defaults to the **active** statuses (`resolveStatusFilter`/
  `activeStatuses` in `order.go`) — full enum minus terminal set `delivered, cancelled, returned, refunded` — sent as
  the server CSV filter. `--status all` (anywhere in the CSV) clears the filter; any other list passes through verbatim.
- `order get` has **no single-record endpoint** — fetches GET /orders and filters client-side.
- `order set-status` takes **status first, then ≥1 ids**, applied to each. Validates `status` against the operator
  allow-list client-side (fast error before any request), but the **server is authority** on legal transitions (409)
  and tracking-iff-`shipped` (400). Each id is independent — a per-id failure is reported and the rest still run; exits
  non-zero with `"N of M orders failed"` if any failed. `--json` emits an **array** of `setStatusResult`
  (`{id, status?, error?}`). `--tracking`/`--note` apply to every id.
- **Short ids** (any unique UUID prefix, e.g. the `id[:13]` from `order list`) accepted by `get`/`set-status` alongside
  full UUIDs. `resolve.go` `matchOrder`: exact-then-unique-prefix (case-insensitive) over the order list — no match →
  "not found", >1 → "ambiguous" listing candidates. `get` matches its already-fetched list; `set-status` calls
  `Client.ResolveOrderID`, which short-circuits a full UUID (`isFullUUID`, canonical 8-4-4-4-12) **without** a list
  fetch and otherwise resolves the prefix before the PATCH (server `format: uuid` rejects short ids).
- `shops` masks the api key as `<hidden>`. Output: `text/tabwriter` table by default; `--json` (persistent flag) emits
  raw JSON. Renderers use `_, _ = fmt.Fprintf(...)` (errcheck). Money via `formatPrice` (minor units → `"19.99 USD"`,
  negative-safe).

## Conventions

### Errors and status codes

Handlers map domain errors via `h.writeError(w, err)`. Five typed errors in `internal/handler/handler.go`:
`BadRequestError` (400), `NotFoundError` (404), `BadGatewayError` (502, upstream), `UnauthorizedError` (401),
`ConflictError` (409, illegal transition). Each has a `Reason string` — always populate; written as
`{"error": "<reason>"}`. Domain errors live in service packages; handler maps via `errors.Is`. Use the most
semantically appropriate status (404 for missing, not 400).

### OpenAPI

Validator (`kin-openapi`) runs in 3.0 compatibility mode:

- No 3.1 array-type (`type: ["string","null"]`); use `type: string` + `nullable: true`.
- UUID path params: `type: string` + `format: uuid`, never `type: uuid` (panics at startup).
- CSV query arrays (`?status=a,b`): `in: query` + `style: form` + `explode: false` + `schema.type: array` with
  item-level `enum`. Validator decodes CSV and rejects unknown values with 400 before the handler — don't redeclare the
  enum in Go.
- `?status=` decodes to `[""]` → fails enum → 400. "No filter" tests must omit the param; `allowEmptyValue: true` does
  NOT rescue this (kin-openapi only checks it when decoded value is nil).
- Full `order_status` lifecycle lives in the `OrderStatus` component schema; `$ref` it from any endpoint surfacing/
  accepting a full-enum status. Operator-only subsets (e.g. `OrderStatusUpdateRequest.status`) keep inline enums (they
  exclude `new`, `awaiting_payment`, …). The Postgres `order_status` enum in `internal/sql/001_init.up.sql` is the
  runtime authority — edit it together with the OpenAPI schema.

### HTTP middleware

Go 1.22+ stdlib patterns (`"GET /products"`). Chain (inner→outer): `content-type → openapi validation → handler`.

- **CORS** (`CORSMiddleware`): unconditionally sets `Access-Control-Allow-Origin: *`; OPTIONS preflight → 204.
  **Explicit `OPTIONS` routes must be registered alongside each `GET` in `app.go`** — stdlib mux has no implicit OPTIONS.
- **Rate limit** (`RateLimitMiddleware(rpm)`): per-IP sliding window `60s/rpm`, 429 + `Retry-After`. IP from first
  `X-Forwarded-For` (Cloudflare) → `RemoteAddr`. `cfg.RateLimit`: positive → RPM; **`0`/negative → disabled** (functests
  use `-1`).
- **API key** (`APIKeyMiddleware`): `Authorization: Bearer <token>`, `crypto/subtle.ConstantTimeCompare`. Two 401
  reasons (`"missing or invalid authorization header"`, `"invalid api key"`) via local `writeUnauthorized`, not
  `writeError`. Empty `apiKey` is a programmer error — only register protected routes when `cfg.Server.APIKey != ""`;
  don't push the empty check into the middleware.

### Money

Integer minor units (cents) across the domain boundary, never float. `int(math.Round(value * 100))` per amount.
**Round once per amount; don't multiply totals** (bias compounds).

### Country codes

Lowercase ISO 3166-1 alpha-2. Two distinct sets:

- **Allow-list** — `shop.Countries` (from `shop.yaml`). Authoritative for who can order; validated by `CreateOrder`
  before reading product YAML.
- **Price keys** — `prices.<country>` per `product.yaml`. Independent of the allow-list. `"default"` is required and is
  **not** an allowed country, only a price fallback.

A country can be allowed without a per-product price (falls back to `prices.default`); test the fallback with an allowed
country lacking a `prices` entry.

### Image path transformation

YAML stores bare filenames; clients receive `/images/{id}/<filename>`. Rewrite in **two** places: (1) loader
`loadProduct`, after `validate` (which needs bare filenames for `os.Stat`); (2) `ServeProductContent`, which reads
`product.yaml` directly. Same for `attr_images`. Any new handler reading `product.yaml` directly must apply this.

### Gallery media: images and video share one list

`Product.Images` (`[]ImageItem`) is a **single ordered media list** of stills and MP4 clips — no separate `videos`
field. Discriminator `ImageItem.Type`: empty/`"image"` = still, `"video"` = MP4. Frontend keys off this, so it **must**
stay on the Go struct (`internal/product/model.go`) and the `ImageItem` schema (`api/root.yaml`). Unified (not split)
because `attr_images`/carousel are index-based into it. Only `joinProductImages` (`internal/loader`) discriminates: it
skips `Type == "video"` when picking the `/products` thumbnail (`Item.Image`) since `.mp4` can't render in `<img>`. Path
transform, `validate`, and `ServeImage` (MIME-agnostic `http.ServeFile`) treat `.mp4` like any file.

### Customer language on orders

`orders.lang` carries the checkout language from `req.Lang` on `POST /orders`; the email notifier resolves
`emails/{status}/{lang}.md` from it. Tests seeding `orders` directly must set a non-empty `lang` (column NOT NULL).

## Packages

### internal/handler

`internal/app` imports `internal/handler`, so **`handler` must never import `app`** (same for `geo`, `monobank`,
`novaposhta`, `order`): define a local interface in `handler` (`geoDetector`, `monobankClient`, `monobankVerifier`,
`novaPoshtaClient`, `orderService`) and wire the concrete type in `app`. Pass config scalars at construction.

`NewHandler` trims `publicURL`'s trailing slash once and stores `webhookURL = publicURL + "/monobank/webhook"` — the
webhook URL is **always derived**, never from config.

`orderService` interface: `Submit`, `AttachInvoice`, `List`, `GetStatus`, `UpdateStatus`, `RecordInvoiceEvent`. Do
**not** add delegating `order.Service` methods for every new `Reader` method — only proxy what `orderService` needs;
other consumers (workers) take `order.Reader` directly, so a `Service` proxy is dead code.

`order.Service` holds an optional `Notifier` (last arg to `NewService`, `nil` disables). `Notify` fires **only after a
successful commit**, guarded by `if s.n != nil`. `RecordInvoiceEvent` dispatches only on a non-empty `newStatus`
(forward transition).

### internal/geo

`Detector.Detect(r)`: (1) header `CF-IPCountry`, validated as exactly two ASCII letters (`isAlpha2`), else falls
through (anti-spoof); (2) client IP → in-memory cache (TTL 1h) → `https://ipinfo.io/{ip}/country` on miss, lowercased/
trimmed, body capped 16 B. Cache writes on success only; errors return `""` uncached. `clientIP` uses `X-Forwarded-For`
only when `CF-IPCountry` is absent (XFF spoofable without Cloudflare). `NewDetector()` is production; tests construct
`*Detector` directly to inject `httpClient`/`serviceURL`. errcheck needs `defer func() { _ = resp.Body.Close() }()`.

### internal/novaposhta

`Client` wraps NP JSON API v2 (`POST https://api.novaposhta.ua/v2.0/json/`): `SearchCities`
(`Address.searchSettlements`), `SearchBranches` (`Address.getWarehouses`), `SearchStreets`
(`Address.searchSettlementStreets`). **Branches/streets use `SettlementRef`, not `CityRef`** (old `getCities` ref →
"City not found"). `NewClient(apiKey, serviceURL)` production (empty → default); tests inject. Body capped 1 MB.
`success=false` is an error regardless of HTTP status.

### internal/monobank

`Client.CreateInvoice` → `/api/merchant/invoice/create`. `NewClient(apiKey, serviceURL)` production (empty → default);
tests inject. Body capped 1 MB. `MapCurrency(code)` maps alpha-3 (case-insensitive) → ISO-4217 numeric; unknown →
`*APIError{ErrCode: "unsupported_currency"}` (handler treats it like any Monobank failure).

**Error policy: generic responses, detailed logs.** All failures (timeout, non-2xx, parse, app error, unsupported
currency, tx2) → 502 `{"error":"bad gateway"}`. Handler logs full detail via `errors.As` on `*APIError` (`order_id`,
`monobank_status`, `monobank_err_code`, `monobank_err_text`, `monobank_body`, `invoice_id`/`page_url`). Both HTTP
non-2xx and `errCode`/`errText` body fields produce `*APIError`.

`ParseWebhook(body)` decodes invoice-status webhooks; validates `invoiceId`/`status`/`reference` non-empty.
`WebhookPayload.RawBody` is a **defensive copy** (safe as JSONB). Domain type has no JSON tags. Empty dates → zero
`time.Time` (RFC3339).

`Verifier` verifies `X-Sign` (ECDSA-P256/SHA-256, base64 ASN.1). Pubkey from `/api/merchant/pubkey` at startup, cached
under `sync.RWMutex`. **On failure the key is refetched once and rechecked** (recovers from rotation);
`ErrInvalidSignature` on persistent mismatch (→ 401); transport/parse errors during refetch returned as-is. Concurrent
refetches deduped via `atomic.Bool`; others spin on 10 ms ticks then re-verify. `NewVerifier(apiKey, serviceURL)`
production; tests construct directly.

### internal/telegram

`Client.SendMessage` → `/bot<token>/sendMessage`. `NewClient(token, serviceURL)` production (empty →
`https://api.telegram.org`); tests inject. Body capped 1 MB. `*APIError` on non-2xx; `RetryAfter` non-zero only on 429
with `parameters.retry_after`; transport errors wrapped plain.

`Notifier` implements `order.Notifier`. `NewNotifier(client, chatID, reader, log)` returns stopped — `Start()` before
first `Notify`, `Stop()` on shutdown. `Notify` non-blocking: full 256-slot buffer or closed channel → drop + Warn.
`Stop()` closes the channel, waits ≤5s to drain. Retry (single bg worker): ≤3 attempts; 4xx (≠429) permanent; 429 →
`RetryAfter` clamped `[1s,30s]`; else `backoffSchedule[attempt-1]` (1s,2s,4s). `formatMessage` uses em-dash, uppercases
currency/country, omits empty optional lines. `messageSender`/`sleepFn` injected for tests.

### internal/resend

`Client.SendEmail` POSTs `{serviceURL}/emails`. `NewClient(apiKey, serviceURL)` production (empty →
`https://api.resend.com`); tests inject. Body capped 1 MB. `*APIError` on non-2xx; `RetryAfter` non-zero only on 429
with `Retry-After`; transport errors wrapped plain.

`Notifier` implements `order.Notifier`. Status filter atop `handle()`: only `paid`, `processing`, `shipped`,
`delivered`, `refund_requested`, `refunded` dispatch; anything else dropped before any DB read or template lookup.
`internal/app/email_validate.go` keeps a parallel `notifyStatuses` slice (same set) for the startup `en.md` check —
**edit both together**. `NewNotifier(client, from, orderURL, reader, products, shop, templates, log)`; lifecycle/
buffer/retry mirror Telegram (≤3; 4xx≠429 permanent; 429 clamped `[1s,30s]`; else 1s,2s).

`TemplateStore` (`LoadTemplates(dir)`) holds templates per `(status, lang)`. `Render(status, lang, data)` →
`(subject, html, text, error)`. Markdown → HTML via `goldmark` **after** `text/template` substitution (so customer
names with markdown chars aren't mangled). Lang fallback `(status, lang)` → `(status, "en")`; missing both is an error.
Plain-text body is the post-template Markdown. Templates at `{data_dir}/emails/{status}/{lang}.md` with YAML
frontmatter; `subject` required non-empty. `internal/loader` parses the dir (policy-free); `app.validateEmailTemplates`
requires `en.md` for every notify-on status when `Resend.APIKey != ""`, aggregating missing paths into one error.
`shop.Service.Name(lang)` returns the shop name with alphabetical fallback; the notifier relies on this.

### internal/order

`MultiNotifier` (`notifier.go`) fans out over `[]Notifier`. `app.Run` wraps enabled notifiers in it only with 2+; with
one passes the child directly, with zero passes nil. A panic in any child is recovered silently. `app.go` wires
`*telegram.Notifier` only when both `Telegram.Token`+`Telegram.ChatID` set.

`ErrTransitionNotAllowed` from `Service.UpdateStatus`/`OperatorWriter.UpdateStatusByOperator` when target isn't legal →
handler 409. `UpdateStatusByOperator` returns `(applied bool, err error)`: `applied=false`+nil = order already at target
under SELECT FOR UPDATE (concurrent same-target write) — caller must NOT notify. `NewService` takes `ow OperatorWriter`
as the **5th positional arg** (before notifier); pass `nil` only in tests not exercising UpdateStatus.

### internal/orderdb

`Writer`/`Reader` own a `*pgxpool.Pool` (concrete). Pool created in `app.Run` after `dbmigrator.RunPostgres` applies
`internal/sql/` migrations.

`Writer.Write` (= `Submit` via `order.Service`) is **transactional**: order header + order_attrs + initial
`order_history (status='new')` in one tx. Empty `MiddleName`/`CustomerNote` → SQL NULL via `nullIfEmpty`.

`Attr.Name`/`Attr.Value` store **rendered titles** in the customer's language, not raw IDs (orders self-documenting,
immutable to YAML renames) — `handler.CreateOrder` resolves `attrLang.Title`/`attrVal.Title` first. `order.Order` (write
side, INSERT fields only) and `order.Record` (read side, adds DB-generated columns + inline `Attrs`/`History`/
`Invoices`) are intentionally separate. Nullable text → `*string` (`json:",omitempty"`); read scans use `pgtype.Text`.

`Writer.UpdateStatusByOperator` writes `tracking_number` write-once via `CASE WHEN $3 = '' THEN tracking_number ELSE $3
END`. Handler enforces "tracking required iff `shipped`"; the SQL guard is defense-in-depth.

**`RecordInvoiceEvent(ctx, evt)`** — single entry point for provider invoice events, one tx: (1) `SELECT … FOR UPDATE`
on `orders` (serializes webhooks), missing → `order.ErrNotFound`; (2) `INSERT invoice_history … ON CONFLICT
(invoice_id, provider, status, event_at) DO NOTHING` (idempotent on the provider dedupe key); (3) re-derive current
status from the timeline (`SELECT … ORDER BY event_at DESC, created_at DESC LIMIT 1`, out-of-order tolerance); (4) map
via `order.InvoiceStatusToOrderStatus`, apply iff `order.ShouldApplyInvoiceTransition(current, candidate)`:
`UPDATE orders.status` + `INSERT order_history` with the latest event's note. `evt.Payload` must be non-empty (errors
before the tx). `order_history.payload` was removed — payloads live only on `invoice_history.payload`. `invoice_status`
enum: `processing, hold, paid, failed, reversed`; Monobank maps via `monobankStatusToInvoiceStatus` (`success → paid`,
`failure|expired → failed`, `created → no row`).

**No unit tests in `internal/orderdb`** — `pgx.Tx` is impractical to stub and no mock is vendored; `Writer`/`Reader` are
exercised by `tests/api/order/` against real PG. Don't reintroduce a stub-based unit test without a real `pgx.Tx`.

### internal/loader

Missing `data_dir`/`products/` → empty catalog (not an error). Malformed YAML or validation failure → **fatal at
startup** (validation runs in `loadProduct` after parsing). Subdirs without `product.yaml` silently skipped.
`{data_dir}/shop.yaml` is **mandatory**: missing → fatal, and `shop.countries` must be non-empty (tests must lay down a
valid `shop.yaml` with ≥1 country). `{data_dir}/products/products.yaml` is a flat list of `product.Item` (id,
categories, title, description) → `catalog.ProductItems` (missing → empty); `joinProductImages` sets each `Item.Image`
to the matching `Product`'s first preview URL (rewritten) or nil. `loadProducts` is wired into no handler — it exists
solely to enforce YAML integrity at startup.

`validate` (fatal at startup): (1) `name` non-empty; (2) `description` non-empty; (3) `name`/`description` language sets
identical; (4) every spec entry covers exactly `name`'s languages; (5) `prices` contains `"default"`; (6) every attr
entry covers exactly `name`'s languages with ≥1 value each; (7) every image `preview`/`full` and `attr_images` filename
exists under `{productDir}/images/`.

## Routes

### `GET /products`, `GET /products/{id}/{lang}`

`ListProducts` returns `products/products.yaml` as `[{id, categories?, title, description, image?}]`: `categories` =
the `Item`'s raw category ids (omitted when empty); `image` = first preview URL of the matching `product.yaml` (omitted
when nil); missing file → `[]`. `product.NewService` normalises nil → `[]*Item{}` (validator needs it).

`ServeProductContent` reads `{data_dir}/products/{id}/product.yaml` directly → lang-filtered `ProductDetail`. `id`/
`lang` validated by `if v != filepath.Base(v) || v == "" || v == "."`. Missing dir/file/lang key → 404. Price:
`h.geo.Detect(r)` → country → `p.Prices[country]` with `p.Prices["default"]` fallback.

- **Categories live only in the listing.** `product.yaml` has no `categories`; `ServeProductContent` cross-references
  them onto `ProductDetail.Categories` by matching `id` against `h.prod.List()`. Guarded by `if h.prod != nil` (always
  wired in prod; tests may omit); a product with no listing entry gets no categories.
- **Price field naming gotcha:** YAML key `prices:`, Go field `Product.Prices`, but the resolved single-country price
  serialises as `"price"` (`ProductDetail.Prices` is a single `PriceItem` with `json:"price"`). Intentional mismatch.

### `GET /shop`

Returns `shop.yaml` (`*shop.Shop`) as JSON: countries allow-list, multilingual `name`/`title`/`description`, ordered
`categories` (id + per-language title), optional per-language `links` (`{title, icon, url}`), and optional
`google_analytics` (`{id}`). The frontend keys footer links and analytics off these.

### `GET /pages`, `GET /pages/{id}/{lang}`

`ListPages` → `pages/pages.yaml` as `[{id, title}]` (missing → `[]`). `ServePage` reads `{data_dir}/pages/{id}/{lang}.md`
as `text/plain; charset=utf-8`; same path-reject pattern. **Skips OpenAPI response middleware** (plain text) but
declared in spec for docs. Missing → 404.

### `GET /product` (singular — social crawler preview)

`ServeProductPreview` server-renders HTML for crawlers. Registered in `app.go` **without** `openapiMw`/`corsMw`; **not**
in `api/root.yaml`. Reads `product.yaml` directly. Lang priority: `?lang=` → alphabetically-first key in `shop.Name` →
first key in product `Name`. Markdown stripped from description for `<meta>`/`og:description` (~200 runes). First
non-video image → `og:image`. Renders OG + Twitter (`summary_large_image`), `<link rel="canonical">`, and a `<meta
http-equiv="refresh">` (bounces humans to the SPA) via `html/template`. **Never returns non-200 to a crawler** —
invalid/unknown ID falls back to shop-level tags. `shopOrigin()` derives the absolute origin for `og:url`/`canonical`
from `h.redirectURL` (scheme+host only), falling back to `publicURL` minus any `/api` suffix; `og:image` uses
`publicURL` directly (correct in both same-origin and split-host deployments).

### `POST /orders`, `GET /orders`

`CreateOrder` validates, resolves product/price, then a **two-phase committed flow**: (1) **tx1 `Submit`** — `INSERT
orders (status='new')` + `order_attrs` + `order_history (new)` (DB is truth before Monobank knows); (2) **Monobank**
`CreateInvoice` — on failure order stays `new`, caller gets 502, orphan reconciled out of band; (3) **tx2
`AttachInvoice`** — `INSERT order_invoices` + `UPDATE orders SET status='awaiting_payment'` + `INSERT order_history
(awaiting_payment)`, one tx.

Required: `product_id, lang, first_name, last_name, phone, email, country, city, address`. Returns `201 {"payment_url":
...}`; 400 invalid; 404 unknown product/path traversal; 502 tx1/Monobank/tx2 (always `{"error":"bad gateway"}`).

`country` is **request-supplied, not geo-detected** — drives price resolution and `orders.country` (stored country
matches the quoted tier). Geo (`h.geo`) is for `ServeProductContent` only. Validated against `shop.Countries` **before
reading product YAML**; disallowed → 400 `{"error": "invalid country"}`. Total = base (`p.Prices[country]`/default) +
per-attr add-ons, each `int(math.Round(value*100))`.

`merchantPaymInfo.basketOrder` is **mandatory** (`INVALID_MERCHANT_PAYM_INFO` otherwise). One line item: `name =
"<title>"` or `"<title> (<Attr1>: <Val1>, …)"` (titles in `req.Lang`, attrs in persisted `order.Attrs` order); `qty=1`,
`sum=totalCents`, `code=req.ProductID`, `tax=h.taxIDs`. Sum of item `sum` must equal invoice `amount`. Optional `icon` =
`<public_url>/images/<product_id>/<images[0].preview>` when present. Other: `reference=orderID`,
`destination="<shop name in req.Lang>, order <orderID first 13 chars>"`, `redirectUrl=Monobank.RedirectURL +
"?order_id=<orderID>"`. `order.id` from PG `DEFAULT uuidv7()` via `RETURNING id`.

`order_invoices` has **no unique constraint on `order_id`** (re-issued invoices). PK is `(id, provider)` where `id TEXT`
is the **provider's** invoice id (no separate `invoice_id` column). A provider mock must return distinct ids per call
when seeding multiple orders, else the 2nd `AttachInvoice` violates the PK → 502 (counter-based mock in
`tests/api/order/get_test.go`).

`GET /orders` returns all orders (attrs/history/invoices, newest first), **registered only when `cfg.Server.APIKey !=
""`** (handler doesn't check the key). `POST /orders` is registered unconditionally on the same path, so an unregistered
`GET` → 405. Conditional registration in `app.go` is the single source of truth. Optional `?status=<csv>` narrows by
current status (validated by OpenAPI).

### `GET /orders/{id}`

`GetOrderStatus` → `{"status": "<order_status>"}` by UUIDv7. Path param validated by OpenAPI (`format: uuid`) →
malformed 400. Valid UUID not in DB → 404 `{"error": "order not found"}`. Public (no API key).

### `POST /monobank/webhook`

`MonobankWebhook` processes invoice-status webhooks. Authed by `X-Sign`; no API key/CORS/OpenAPI/rate-limit (Monobank
sole caller). Reads ≤1 MB, verifies via `h.mbVerifier`, parses with `ParseWebhook`, maps `Status` → `invoice_status`
via `monobankStatusToInvoiceStatus` (`created` → 200, no DB write), builds `order.InvoiceEvent{…, Payload:
payload.RawBody, EventAt: payload.ModifiedDate}`, calls `RecordInvoiceEvent`.

**Response: status code only, empty body** via `w.WriteHeader` (never `writeError`). By retryability: 200 for
processed/idempotent/informational/unknown-reference; 401 bad signature; 400 malformed; **500 for transient DB/
verifier-transport errors so Monobank retries** (the only handler returning 500 as a retry signal). `event_at` =
Monobank's `modifiedDate` (not `CURRENT_TIMESTAMP`) → out-of-order webhooks resolve via the provider's authoritative
timestamp.

Lifecycle: `order.ShouldApplyInvoiceTransition(current, candidate)` (`internal/order/lifecycle.go`). Invoice events
freely drive the pre-paid cluster {`new`, `awaiting_payment`, `payment_processing`, `payment_hold`, `cancelled`};
`cancelled` re-enterable; `paid` stable against payment_*/`cancelled` (only `refunded` advances it); fulfillment states
(`processing`, `shipped`, `delivered`, `refund_requested`, `returned`) are operator-owned and ignore invoice events
except `refunded`, which always wins.

### `GET /nova-poshta/cities`, `/branches`, `/streets`

`cities?q=` → `searchSettlements`; `branches?city_ref=&q=` → `getWarehouses`; `streets?city_ref=&q=` →
`searchSettlementStreets`. `q` required everywhere; `city_ref` required for branches/streets — missing → 400. NP failure
→ 502.

### `GET /images/{product_id}/{file_name}`

`ServeImage` serves `{data_dir}/products/{product_id}/images/{file_name}` via `http.ServeFile`; `filepath.Base` on both
values (traversal guard). Functest: only the image file is needed to serve it; `product.yaml` is needed only for the
product to appear in `catalog.Products`.

### `GET /assets/{path...}`

`ServeAsset` serves `{data_dir}/assets/<path>` via `http.ServeFile`, supporting nested subdirs (trailing `{path...}`
wildcard). Containment guard: `filepath.Join(dataDir, "assets", path)` (runs `Clean`) must stay under the assets base
(`HasPrefix(full, base+sep)`), else 404 — `filepath.Base` is unusable for multi-segment paths. Directory requests and
missing files → 404. CORS-only, no OpenAPI middleware, not in `api/root.yaml`. Backs the frontend's `applyAssets()`
(favicons, logo).

## Frontend & deployment

`frontend/www/` is a static storefront — vanilla HTML/CSS/JS, **no framework, no build step**, served by **Caddy, not
the Go app**. Three HTML entry points: `index.html` (home + Markdown content pages), `product.html` (detail, carousel,
order form), `order-status.html` (polls `GET /orders/{id}`). JS: `js/api.js` (fetch wrappers), `js/i18n.js` (UI strings
— **every key must have both `en` and `uk`**; `main.js` `t()` falls back to the key), `js/main.js` (routing via query
params `?id=`/`?lang=`/`?category=`/`?img=`, rendering, order form, carousel). Markdown via `marked` (CDN).

- API base: `window.API_BASE || <origin>/api`. An optional, unbundled `config.js` (referenced by `index.html`) sets
  `window.API_BASE` for split-host deployments.
- `applyAssets()` injects favicons/footer logo from the backend `/assets/<conventional-filename>`; none are bundled.
- Google Analytics (gtag.js) is injected at runtime only when `GET /shop` returns `google_analytics.id`. Footer links
  come from the `/shop` per-language `links`.

Deployment: `Dockerfile` (Caddy base) copies `app.out` → `/app/app` and `frontend/www/` → Caddy's web root;
`entrypoint.sh` runs both (server on `:9000` + Caddy), exiting when either child exits. `Caddyfile` routes: `/api*`
reverse-proxied to `:9000` with `/api` stripped; `/product` → backend `ServeProductPreview` for bots (User-Agent regex)
vs `product.html` for humans; `/order/status` → `order-status.html`; images/JS/CSS/HTML served from the web root with
per-type cache headers; SPA fallback to `index.html`.

## Configuration

Keys in `internal/app/config.go`:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, **`Server.PublicURL`** — required; empty → startup error.
- **`Server.PublicURL`** — public https base URL. Derives the webhook URL (`<PublicURL>/monobank/webhook`, sent as
  `webHookUrl`) and basket icon (`<PublicURL>/images/<product_id>/<preview>`). Trailing slash trimmed in `NewHandler`.
  **No separate `monobank.webhook_url` config.**
- **`Monobank.ServiceURL`**, **`NovaPoshta.ServiceURL`**, **`Telegram.ServiceURL`**, **`Resend.ServiceURL`** — optional;
  empty → real upstream. Tests inject `httptest.Server.URL` via `testapp.New` opts.
- **`Telegram.Token`** + **`Telegram.ChatID`** — both optional; both empty → disabled; exactly one → startup error
  (`"telegram: token and chat_id must be set together"`); both → enabled. `defer tn.Stop()` registered **after** `defer
  db.Close()` so LIFO drains pending events while the pool is open.
- **`Monobank.TaxIDs`** — merchant tax IDs; wired as `taxIDs []int`, emitted on every basket item.
- **`Server.APIKey`** — optional; empty disables `GET /orders` via conditional registration.
- **`RateLimit`** — positive → RPM/IP; `0`/negative → disabled (functests `-1`).
- **`DataDir`** — default `./data`.
- **`Resend.APIKey`** — optional; empty disables the email notifier.
- **`Mail.From`**, **`Mail.OrderURL`** (literal `{id}` pattern) — required when `Resend.APIKey` set; empty + key set →
  startup error.

`testapp.New` defaults `Monobank.APIKey="test-key"`, `RedirectURL="https://test.example/thanks"`,
`PublicURL="https://test.example"`, and starts a built-in `httptest.Server` as `Monobank.ServiceURL` serving
`/api/merchant/pubkey` so `Verifier.Fetch` succeeds. Tests overriding `Monobank.ServiceURL` get their own stub.

`uuidv7()` is built-in PostgreSQL 18 (no extension); `orders`/`order_history` use `id uuid PRIMARY KEY DEFAULT
uuidv7()`. Test container pins `postgres:18-alpine`.

## Tests

### Running

```
task go:test:unit -- [FLAGS]
task go:test:func -- [FLAGS]
```

`[FLAGS]` are standard `go test` flags. In worktrees without `.ci/`, run directly: `go test -run TestFoo -v
./internal/...` or `go test -tags=functest ./tests/...`. Dirty PG → `task go:test:func:clean`.

### Unit tests

- Before implementing, invoke `superpowers:test-driven-development`.
- Before claiming done, invoke `superpowers:verification-before-completion` and run **both** `task go:test:unit -- ./...`
  and `task go:test:func -- -v` — both must pass. Then `task go:golangci-lint` — all lint must pass. No summary before
  tests pass.
- Group related tests: `TestFoo(main *testing.T)` + `main.Run("Case", ...)`. Never separate `TestFoo_Case` functions.
- New file in a package with `_test.go` companions → write its unit test in the same task.
- Handler responses with `id` `format: uuid`: mock returns must be valid UUIDs (e.g.
  `"018f4e3a-0000-7000-8000-000000000099"`), else the validator → 500 masks your assertion.

### Functional tests

`docker-compose.tests.yaml` runs `postgres:18-alpine`; `tests` depends on `postgres: service_healthy` and exports
`APP_DB_DSN=postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable`. `testapp.New` reads it into
`cfg.Database.DSN` (override via `opts`); `(*App).DSN()` exposes it.

PG persists across runs — truncate explicitly. `tests/api/order/post_test.go` has `truncateOrders` (`TRUNCATE
order_attrs, order_history, orders RESTART IDENTITY CASCADE`). After a destructive test, restore schema in `t.Cleanup`
(use `RENAME`, not `DROP`; see `TestCreateOrder_DBFailure`). **`t.Cleanup` runs after `defer`** — a test that
`defer pool.Close()` then registers a cleanup using the pool silently fails; close the pool inside the cleanup, or let
the cleanup own teardown.

Fixtures — loader unit tests (`internal/loader/`): `makeProductDir(t, dataDir, id, yaml, extraFiles)`,
`makeProductsFile(t, dataDir, content)`. API functional tests (`tests/api/product/`): `makeDataDir(t, productsYAML,
productYAMLs)`; `testapp.New(t, dataDir, opts...)` (mutators override fields). **`testapp.New` does not start the app** —
call `a.Start()` before requests. Subtests needing a separate empty catalog start their own `testapp` (random port).

Orchestration: all subtests in a `TestFoo` share one `testapp` (per-subtest instances panic on port conflict under
`main.Parallel()`). At most one top-level `TestFoo` per package may `main.Parallel()` if each starts its own `testapp`;
new app-starting functions must NOT `main.Parallel()` unless existing ones are refactored to share. Tests issuing many
synchronous requests set `cfg.RateLimit = -1`.

### Shared helpers

Keep shared test helpers in `handler_test.go`, not per-feature files, so they survive feature removal.

### Shared ECDSA test key (`tests/api/order/`)

A single `testECDSAKey` is generated in `init()`. `pubKeyPayload(t)` returns the PEM pubkey in the Monobank pubkey JSON;
`signWebhookBody(t, body)` computes the ECDSA-SHA256 signature `POST /monobank/webhook` expects. All Monobank stubs in
this package use `pubKeyPayload` for `/api/merchant/pubkey` — no test generates its own key.
