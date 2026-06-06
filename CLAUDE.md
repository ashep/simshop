# Project Rules

## Meta

After any task, if a non-obvious decision/pattern/constraint is likely to recur, record it here under the right
section. Don't record what's derivable from reading the code. **Mandatory** — update this file as the final step of
every task before the summary.

After changing a feature, update `README.md` if business logic worth describing changed (entities, concepts,
endpoints, behavior). Wrap `README.md` at 120 chars.

In commit messages NEVER mention yourself. Commit messages must look like a regular person worked on them.

When adding a field to an existing struct/method, read the file first and preserve all existing fields/logic
(including user edits outside the plan). Never `replace_all` a new field into test bodies without auditing every
match — a "missing X" test must not gain X.

`Write` requires the target be `Read` first in this session; `Read` each target before a multi-file write pass.

`data/` is a **symlink** to `../websites/craft.d5y.xyz/data` (separate git repo). Commit files under `data/` there:
`git -C /Users/ashep/src/my/websites/craft.d5y.xyz add ...`. `git add data/...` here fails ("beyond a symbolic link").

`vendor/` is gitignored (regenerated via `go mod vendor`); only `go.mod`/`go.sum` are staged. `go mod tidy` strips
modules no real import reaches — to land a dep before its consumer, `go get` it (records `// indirect`) and **skip
`go mod tidy`** until the importer lands. `go mod vendor` only populates `vendor/` once a `.go` file imports the module.

## Project overview

`simshop` is a Go HTTP API (`github.com/ashep/simshop`). Stack: filesystem catalog (products from YAML at startup, no
product DB), PostgreSQL for orders, zerolog, `go-app` framework (`github.com/ashep/go-app`), OpenAPI spec in `api/`
for request/response validation.

Dirs: `internal/` (app, cli, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product,
shop, sql), `api/`, `tests/` (build tag `functest`), `vendor/`. Config from `config.yml` + env; data from `data_dir`
(default `./data`). Use `api/` as context for new features.

### CLI (`cmd/simshop` + `internal/cli`)

Operator CLI, a **separate binary** from the server. Entrypoint is `cmd/simshop/main.go` (thin: builds
`cli.NewRootCmd()` and `Execute()`s); the server stays at the repo-root `main.go`. `go install
github.com/ashep/simshop/cmd/simshop@latest` yields the `simshop` binary. All logic is in `internal/cli` (cobra-based:
`spf13/cobra` is a direct dep). The CLI is a pure HTTP client over the existing API — covered by unit tests +
`httptest` only, no functest harness.

`~/.simshop.yaml` is a top-level mapping of shop-name → `{url, api_key, default?}` (both `url` and `api_key`
required). File order matters: the first shop is the implicit default when no entry has `default: true`. Parsed via
`yaml.Node` (not a plain `map`) to preserve insertion order. `Config.Select("")` returns the default;
`Config.Select(name)` returns the named shop or an error listing all configured names. `LoadConfig(path)` reads from
disk; `DefaultConfigPath()` prefers `.yaml` over `.yml`. Shop precedence: `--shop` flag → `default: true` → first.

Commands: `order list [--status csv]` (GET /orders), `order get <id>`, `order set-status <id> <status>` (PATCH
/orders/{id}/status), `shops`. `order list` with no `--status` defaults to the **active** statuses
(`resolveStatusFilter`/`activeStatuses` in `order.go`) — the full enum minus the terminal set `delivered, cancelled,
returned, refunded` — sent as the server CSV filter; `--status all` (anywhere in the CSV) clears the filter to show
everything; any other value list passes through verbatim. `order get` has **no single-record endpoint** — it fetches
GET /orders and filters by id client-side. `set-status` validates `status` against the operator allow-list client-side for a fast error, but the
**server stays authority** on legal transitions (409) and the tracking-iff-`shipped` rule (400).

`order get` and `order set-status` accept a **short id** (any unique prefix of the UUID, e.g. the `id[:13]` shown by
`order list`) alongside a full UUID. Resolution is in `resolve.go`: `matchOrder` does exact-then-unique-prefix
matching (case-insensitive) over the order list — no match → "not found", >1 → "ambiguous" listing candidates.
`get` matches against the list it already fetches; `set-status` calls `Client.ResolveOrderID` first, which
short-circuits a full UUID (`isFullUUID`, canonical 8-4-4-4-12) **without** a list fetch and otherwise resolves the
prefix to the full id before the server PATCH (server's `format: uuid` would reject a short id). `shops` masks the
api key as `<hidden>`, never printing it. Output: aligned `text/tabwriter` table by default; `--json` (persistent
flag) emits raw JSON. Renderers use `_, _ = fmt.Fprintf(...)` blank-assignment to satisfy errcheck (as `main.go`
does). Money renders via `formatPrice` (integer minor units → `"19.99 USD"`, negative-safe).

## Conventions

### Errors and status codes

Handlers map domain errors via `h.writeError(w, err)`. Five typed errors in `internal/handler/handler.go`:
`BadRequestError` (400), `NotFoundError` (404), `BadGatewayError` (502, upstream), `UnauthorizedError` (401),
`ConflictError` (409, illegal transition). Each has a `Reason string` — always populate it; written client-visibly as
`{"error": "<reason>"}`. Domain errors live in service packages; handler maps via `errors.Is`. Use the most
semantically appropriate status (404 for missing, not 400).

### OpenAPI

Validator (`kin-openapi`) runs in 3.0 compatibility mode:

- No 3.1 array-type syntax (`type: ["string","null"]`); use `type: string` + `nullable: true`.
- UUID path params: `type: string` + `format: uuid`, never `type: uuid` (panics at startup).
- CSV query arrays (`?status=a,b`): `in: query` + `style: form` + `explode: false` + `schema.type: array` with
  item-level `enum`. Validator decodes CSV and rejects unknown values with 400 before the handler — don't redeclare
  the enum in Go.
- `?status=` decodes to `[""]` and fails the enum check → 400. "No filter" tests must omit the param entirely;
  `allowEmptyValue: true` does NOT rescue this (kin-openapi only checks it when decoded value is nil).
- Full `order_status` lifecycle lives in the `OrderStatus` component schema; `$ref` it from any endpoint surfacing/
  accepting a full-enum status. Operator-only subsets (e.g. `OrderStatusUpdateRequest.status`) keep inline enums
  (they exclude `new`, `awaiting_payment`, etc.). The Postgres `order_status` enum in
  `internal/sql/001_init.up.sql` is the runtime authority — edit it together with the OpenAPI schema.

### HTTP middleware

Go 1.22+ stdlib pattern syntax (`"GET /products"`). Chain (inner→outer): `content-type → openapi validation → handler`.

- **CORS** (`handler.CORSMiddleware`): unconditionally sets `Access-Control-Allow-Origin: *`; OPTIONS preflight → 204.
  **Explicit `OPTIONS` routes must be registered alongside each `GET` in `app.go`** — stdlib mux doesn't handle
  OPTIONS implicitly.
- **Rate limit** (`handler.RateLimitMiddleware(rpm)`): per-IP sliding window `60s/rpm`, 429 + `Retry-After`. IP from
  first `X-Forwarded-For` (Cloudflare) → `RemoteAddr`. `cfg.RateLimit`: positive → RPM; **`0`/negative → disabled**
  (functests use `-1`).
- **API key** (`handler.APIKeyMiddleware`): `Authorization: Bearer <token>`, `crypto/subtle.ConstantTimeCompare`. Two
  401 reasons (`"missing or invalid authorization header"`, `"invalid api key"`) via local `writeUnauthorized`, not
  `writeError`. Empty `apiKey` is a programmer error — only register protected routes when `cfg.Server.APIKey != ""`;
  don't push the empty check into the middleware.

### Money

Prices crossing the domain boundary are integer minor units (cents), never float. `int(math.Round(value * 100))` for
base price and each add-on. **Round once per amount; don't multiply totals** (bias compounds).

### Country codes

Lowercase ISO 3166-1 alpha-2. Two distinct sets:

- **Allow-list** — `shop.Countries` (from `shop.yaml`). Authoritative for who can order; validated by `CreateOrder`
  before reading product YAML.
- **Price keys** — `prices.<country>` per `product.yaml`. Independent of allow-list. The `"default"` key is required
  and is **not** an allowed country — only a price fallback.

A country can be allowed without a per-product price (falls back to `prices.default`). To test the fallback, pass an
allowed country with no matching `prices` entry.

### Image path transformation

YAML stores bare filenames; clients receive `/images/{id}/<filename>`. Rewrite in **two** places:

1. **Loader** (`loadProduct`): after `validate` (which needs bare filenames for `os.Stat`).
2. **`ServeProductContent`**: reads `product.yaml` directly, so applies the transform inline before `ProductDetail`.

Same for `attr_images`. Any new handler reading `product.yaml` directly must apply this.

### Gallery media: images and video share one list

`Product.Images` (`[]ImageItem`) is a **single ordered media list** of stills and MP4 clips — no separate `videos`
field. Discriminator `ImageItem.Type`: empty/`"image"` = still, `"video"` = MP4 (`.mp4` `Preview`/`Full`). Frontend
keys off this, so it **must** stay on the Go struct (`internal/product/model.go`) and the `ImageItem` schema
(`api/root.yaml`). Unified (not split) because `attr_images`/carousel are index-based into it.

Only `joinProductImages` (`internal/loader`) discriminates: it skips `Type == "video"` when picking the `/products`
list thumbnail (`Item.Image`) since an `.mp4` can't render in `<img>`. Path transform, `validate`, and `ServeImage`
(MIME-agnostic `http.ServeFile`) treat `.mp4` like any file. The generator (`craft.d5y.xyz/prepare-images.sh`) orders
photos before videos.

### Customer language on orders

`orders.lang` carries the checkout language from `req.Lang` on `POST /orders`; the email notifier resolves
`emails/{status}/{lang}.md` from it. Tests seeding `orders` directly must set a non-empty `lang` (column is NOT NULL).

## Packages

### internal/handler

`internal/app` imports `internal/handler`, so **`handler` must never import `app`**. Pass config scalars at
construction. Same for other internal packages (`geo`, `monobank`, `novaposhta`, `order`): define a local interface in
`handler` (`geoDetector`, `monobankClient`, `monobankVerifier`, `novaPoshtaClient`, `orderService`) and wire the
concrete type in `app`.

`NewHandler` trims `publicURL`'s trailing slash once and stores `webhookURL = publicURL + "/monobank/webhook"` — the
webhook URL is **always derived**, never from config.

`orderService`: `Submit`, `AttachInvoice`, `List`, `GetStatus`, `UpdateStatus`, `RecordInvoiceEvent`.

Do **not** add delegating `order.Service` methods for every new `Reader` method — only proxy what the `orderService`
interface needs. Other consumers (background workers) take `order.Reader` directly; a `Service` proxy is dead code.

`order.Service` holds an optional `Notifier` (5th arg to `NewService`; `nil` disables). `Notify` fires **only after a
successful commit**, guarded by `if s.n != nil`. `RecordInvoiceEvent` dispatches only on a non-empty `newStatus`
(forward transition). `app.go` wires `*telegram.Notifier` only when both `Telegram.Token` and `Telegram.ChatID` set.

### internal/geo

`Detector.Detect(r)` resolves a country in two steps:

1. **Header**: `CF-IPCountry`, validated as exactly two ASCII letters (`isAlpha2`); else falls through (anti-spoof).
2. **IP lookup**: client IP → in-memory cache (TTL 1h) → `https://ipinfo.io/{ip}/country` on miss. Lowercased,
   trimmed, body capped 16 B via `io.LimitReader`.

Cache writes only on success; errors return `""` and aren't cached. `clientIP` uses `X-Forwarded-For` only when
`CF-IPCountry` is absent (XFF is spoofable without Cloudflare). `NewDetector()` is production; tests construct
`*Detector` directly to inject `httpClient`/`serviceURL`. `errcheck` needs `defer func() { _ = resp.Body.Close() }()`.

### internal/novaposhta

`Client` wraps NP JSON API v2 (`POST https://api.novaposhta.ua/v2.0/json/`): `SearchCities`
(`Address.searchSettlements`), `SearchBranches` (`Address.getWarehouses`), `SearchStreets`
(`Address.searchSettlementStreets`). **Branches/streets use `SettlementRef`, not `CityRef`** (old `getCities` ref →
"City not found"). `NewClient(apiKey, serviceURL)` is production (empty → default); tests inject directly. Body capped
1 MB. `success=false` is an error regardless of HTTP status.

### internal/monobank

`Client.CreateInvoice` → `/api/merchant/invoice/create`. `NewClient(apiKey, serviceURL)` production (empty → default);
tests inject. Body capped 1 MB. `MapCurrency(code)` maps alpha-3 (case-insensitive) → ISO-4217 numeric; unknown →
`*APIError{ErrCode: "unsupported_currency"}` so the handler treats it like any Monobank failure.

**Error policy: generic responses, detailed logs.** All failures (timeout, non-2xx, parse, app error, unsupported
currency, tx2) → HTTP 502 `{"error":"bad gateway"}`. Handler logs full detail via `errors.As` on `*APIError` with
structured fields (`order_id`, `monobank_status`, `monobank_err_code`, `monobank_err_text`, `monobank_body`, plus
`invoice_id`/`page_url`). Both HTTP non-2xx and `errCode`/`errText` body fields produce `*APIError`.

`ParseWebhook(body)` decodes invoice-status webhooks; validates `invoiceId`/`status`/`reference` non-empty.
`WebhookPayload.RawBody` is a **defensive copy** (safe as JSONB). Domain type has no JSON tags (only the wire struct).
Empty dates → zero `time.Time`; parsed with `time.RFC3339`.

`Verifier` verifies `X-Sign` (ECDSA-P256/SHA-256, base64 ASN.1). Pubkey from `/api/merchant/pubkey` at startup, cached
under `sync.RWMutex`. **On failure the key is refetched once and rechecked** (recovers from rotation).
`ErrInvalidSignature` on persistent mismatch (→ 401). Transport/parse errors during refetch returned as-is.
Concurrent refetches deduped via `atomic.Bool` (`refetching`); others spin on 10 ms ticks then re-verify.
`NewVerifier(apiKey, serviceURL)` production; tests construct directly.

### internal/telegram

`Client.SendMessage` → `/bot<token>/sendMessage`. `NewClient(token, serviceURL)` production (empty →
`https://api.telegram.org`); tests inject. Body capped 1 MB. `*APIError` on non-2xx; `RetryAfter` non-zero only on 429
with `parameters.retry_after`. Transport errors wrapped plain (distinguish via `errors.As`).

`Notifier` implements `order.Notifier`. `NewNotifier(client, chatID, reader, log)` returns stopped; call `Start()`
before first `Notify`, `Stop()` on shutdown. `Notify` is non-blocking: full 256-slot buffer or closed channel → drop
+ Warn. `Stop()` closes channel, waits ≤5s to drain. `messageSender` local interface (`fakeSender` in tests);
`sleepFn` injected. Retry: ≤3 attempts; 4xx (≠429) permanent; 429 → `RetryAfter` clamped `[1s,30s]`; else
`backoffSchedule[attempt-1]` (1s,2s,4s). `formatMessage` uses em-dash (—), uppercases currency/country, omits empty
optional lines.

### internal/resend

`Client.SendEmail` POSTs `{serviceURL}/emails`. `NewClient(apiKey, serviceURL)` production (empty →
`https://api.resend.com`); tests inject. Body capped 1 MB. `*APIError` on non-2xx; `RetryAfter` non-zero only on 429
with `Retry-After` header. Transport errors wrapped plain.

`Notifier` implements `order.Notifier`. Status filter atop `handle()`: only `paid`, `shipped`, `delivered`,
`refund_requested`, `refunded` dispatch; anything else silently dropped before any DB read or template lookup.
`NewNotifier(client, from, orderURL, reader, products, shop, templates, log)` returns stopped; `Start()`/`Stop()` as
above. Non-blocking drop on full buffer (256)/closed channel. Retry mirrors Telegram (≤3; 4xx≠429 permanent; 429 →
clamped `[1s,30s]`; else `backoffSchedule[attempt-1]` 1s,2s).

`TemplateStore` (via `LoadTemplates(dir)`) holds templates per `(status, lang)`. `Render(status, lang, data)` →
`(subject, html, text, error)`. Markdown → HTML via `goldmark` **after** `text/template` substitution (so customer
names with markdown chars aren't mangled). Lang fallback: `(status, lang)` → `(status, "en")`; missing both is an
error. Plain-text body is the post-template Markdown (good enough for Resend's tracker).

Templates at `{data_dir}/emails/{status}/{lang}.md` with YAML frontmatter; `subject` required non-empty.
`internal/loader` parses the dir (stays policy-free); `app.validateEmailTemplates` requires `en.md` for every
notify-on status when `Resend.APIKey != ""`, aggregating missing paths into one error.

`shop.Service.Name(lang)` returns the shop name in `lang` with alphabetical fallback; the notifier relies on this
instead of its own en-fallback.

### internal/order

`MultiNotifier` (`notifier.go`) fans out over `[]Notifier`. `app.Run` wraps enabled notifiers in it only with 2+;
with one passes the child directly, with zero passes nil. A panic in any child is recovered silently (children log
internally).

`ErrTransitionNotAllowed` from `Service.UpdateStatus`/`OperatorWriter.UpdateStatusByOperator` when target isn't a
legal next step → handler 409.

`OperatorWriter.UpdateStatusByOperator` returns `(applied bool, err error)`. `applied=false`+nil = order already at
target under SELECT FOR UPDATE (concurrent same-target write) — caller must NOT notify. `NewService` takes
`ow OperatorWriter` as the 5th positional arg (before notifier); pass `nil` only in tests not exercising UpdateStatus.

### internal/orderdb

`Writer`/`Reader` own a `*pgxpool.Pool` (concrete). Pool created in `app.Run` after `dbmigrator.RunPostgres` applies
`internal/sql/` migrations.

`Writer.Write` (= `Submit` via `order.Service`) is **transactional**: order header + order_attrs + initial
`order_history (status='new')` in one tx (deferred `Rollback` after `Commit` is a no-op). Empty `MiddleName`/
`CustomerNote` → SQL NULL via `nullIfEmpty`.

`Attr.Name`/`Attr.Value` store **rendered titles** in the customer's language, not raw IDs (orders are
self-documenting, immutable to YAML renames) — `handler.CreateOrder` resolves `attrLang.Title`/`attrVal.Title` first.

`order.Order` (write side, INSERT fields only) and `order.Record` (read side, adds DB-generated columns + inline
`Attrs`/`History`/`Invoices`) are intentionally separate. Nullable text → `*string` (`json:",omitempty"`); read-side
scans use `pgtype.Text`.

`Writer.UpdateStatusByOperator` writes `tracking_number` write-once via `CASE WHEN $3 = '' THEN tracking_number ELSE
$3 END`. Handler enforces "tracking required iff `shipped`"; the SQL guard is defense-in-depth.

#### `RecordInvoiceEvent` and the invoice timeline

`RecordInvoiceEvent(ctx, evt)` — single entry point for provider invoice events, one tx:

1. `SELECT … FOR UPDATE` on `orders` (serializes concurrent webhooks). Missing → `order.ErrNotFound`.
2. `INSERT invoice_history … ON CONFLICT (invoice_id, provider, status, event_at) DO NOTHING` — idempotent on the
   provider dedupe key.
3. `SELECT status, note FROM invoice_history WHERE order_id=… ORDER BY event_at DESC, created_at DESC LIMIT 1` —
   re-derives current invoice status from the timeline (out-of-order tolerance).
4. Map via `order.InvoiceStatusToOrderStatus`; apply iff `order.ShouldApplyInvoiceTransition(current, candidate)`:
   `UPDATE orders.status` + `INSERT order_history` with the latest event's note.

`evt.Payload` must be non-empty (errors before opening the tx). `order_history.payload` was removed — payloads live
only on `invoice_history.payload` (audit trail). `invoice_status` enum: `processing, hold, paid, failed, reversed`;
Monobank maps via `monobankStatusToInvoiceStatus` (`success → paid`, `failure|expired → failed`, `created → no row`).

#### No unit tests in `internal/orderdb`

`pgx.Tx` is impractical to stub and no mock is vendored. `Writer`/`Reader` are exercised by `tests/api/order/`
against real PG. Don't reintroduce a stub-based unit test without a real `pgx.Tx`.

### internal/loader

Missing `data_dir`/`products/` → empty catalog (not an error). Malformed YAML or validation failure → **fatal at
startup** (validation runs in `loadProduct` after parsing). Subdirs without `product.yaml` are silently skipped.

`{data_dir}/shop.yaml` is **mandatory**: missing → fatal, and `shop.countries` must be non-empty. Tests must lay down
a valid `shop.yaml` with ≥1 country.

`{data_dir}/products/products.yaml` is a flat list of `product.Item` (id, title, description) → `catalog.ProductItems`
(missing → empty). Separate from per-dir `loadProducts`; both coexist in `Catalog`. `joinProductImages` sets each
`Item.Image` to the matching `Product`'s first preview URL (rewritten) or leaves nil.

`validate` (fatal at startup): (1) `name` non-empty; (2) `description` non-empty; (3) `name`/`description` language
sets identical; (4) every spec entry covers exactly `name`'s languages; (5) `prices` contains `"default"`; (6) every
attr entry covers exactly `name`'s languages with ≥1 value each; (7) every image `preview`/`full` and `attr_images`
filename exists under `{productDir}/images/`.

`loadProducts` is wired into no handler — it exists solely to enforce YAML integrity at startup.

## Routes

### `GET /products`, `GET /products/{id}/{lang}`

`ListProducts` returns `products/products.yaml` as `[{id, title, description, image?}]`; `image` = first preview URL
of the matching `product.yaml`, omitted when nil; missing file → `[]`. `product.NewService` normalises nil → `[]*Item{}`
(OpenAPI validator needs it).

`ServeProductContent` reads `{data_dir}/products/{id}/product.yaml` directly → lang-filtered `ProductDetail`. `id`/
`lang` validated by `if v != filepath.Base(v) || v == "" || v == "."`. Missing dir/file → 404; missing lang key → 404.
Price: `h.geo.Detect(r)` → country → `p.Prices[country]` with `p.Prices["default"]` fallback. `ProductDetail.Prices`
is a single `PriceItem`, not a map.

**Price field naming gotcha:** YAML key `prices:`, Go field `Product.Prices`, but the resolved single-country price
serialises as `"price"` (`ProductDetail.Prices` has `json:"price"`). Intentional mismatch.

### `GET /pages`, `GET /pages/{id}/{lang}`

`ListPages` → `pages/pages.yaml` as `[{id, title}]` (missing → `[]`). `ServePage` reads
`{data_dir}/pages/{id}/{lang}.md` as `text/plain; charset=utf-8`; same path-reject pattern. **Skips OpenAPI response
middleware** (plain text) but declared in spec for docs. Missing → 404.

### `GET /shop`

Returns `shop.yaml` (`*shop.Shop`) as JSON.

### `GET /product` (singular — social crawler preview)

`ServeProductPreview` server-renders HTML for crawlers. Registered in `app.go` **without** `openapiMw` and `corsMw`;
**not** in `api/root.yaml`. Reads `product.yaml` directly. Lang priority: `?lang=` → alphabetically-first key in
`shop.Name` → first key in product `Name`. Markdown stripped from description for `<meta>`/`og:description` (~200
runes). First non-video image → `og:image`. Renders OG + Twitter (`summary_large_image`), `<link rel="canonical">`,
and a `<meta http-equiv="refresh">` (bounces humans to the SPA) via `html/template`. **Never returns non-200 to a
crawler** — invalid/unknown ID falls back to shop-level tags.

`shopOrigin()` derives the absolute shop origin for `og:url`/`canonical` from `h.redirectURL` (scheme+host only),
falling back to `publicURL` minus any `/api` suffix. `og:image` uses `publicURL` directly. Two-source derivation
keeps `og:url` correct in both same-origin (API under `/api`) and split-host deployments.

### `POST /orders`, `GET /orders`

`CreateOrder` validates, resolves product/price, then a **two-phase committed flow**:

1. **tx1 (`Submit`)** — `INSERT orders (status='new')` + `order_attrs` + `order_history (new)`. DB is truth before
   Monobank knows.
2. **Monobank** — `CreateInvoice`. On failure order stays `new`; caller gets 502. Orphan state reconciled out of band.
3. **tx2 (`AttachInvoice`)** — `INSERT order_invoices` + `UPDATE orders SET status='awaiting_payment'` + `INSERT
   order_history (awaiting_payment)`, one tx.

Required: `product_id, lang, first_name, last_name, phone, email, country, city, address`. Returns
`201 {"payment_url": ...}`; 400 invalid; 404 unknown product/path traversal; 502 tx1/Monobank/tx2 failure (always
`{"error":"bad gateway"}`).

`country` is **request-supplied, not geo-detected** — drives price resolution and `orders.country`, so the stored
country matches the quoted tier. Geo (`h.geo`) is for `ServeProductContent` only. Validated against `shop.Countries`
**before reading product YAML**; disallowed → 400 `{"error": "invalid country"}`. Test the `default` fallback with an
allowed country lacking a `prices.<country>` entry.

Total = base (`p.Prices[country]`/default) + per-attr add-ons, each `int(math.Round(value*100))`.

`merchantPaymInfo.basketOrder` is **mandatory** (`INVALID_MERCHANT_PAYM_INFO` otherwise). One line item: `name =
"<title>"` or `"<title> (<Attr1>: <Val1>, …)"` (titles in `req.Lang`, attrs in persisted `order.Attrs` order). Fields:
`qty=1`, `sum=totalCents`, `code=req.ProductID`, `tax=h.taxIDs` (from `Monobank.TaxIDs`). Sum of item `sum` must equal
invoice `amount`. Optional `icon` = `<public_url>/images/<product_id>/<images[0].preview>` when present (else omitted).
Other fields: `reference=orderID`, `destination="<shop name in req.Lang>, order <orderID first 13 chars>"`,
`redirectUrl=Monobank.RedirectURL + "?order_id=<orderID>"`. `order.id` from PG `DEFAULT uuidv7()` via `RETURNING id`.

`order_invoices` has **no unique constraint on `order_id`** (re-issued invoices). PK is `(id, provider)` where
`id TEXT` is the **provider's** invoice id (no separate `invoice_id` column). A provider mock must return distinct ids
per call when seeding multiple orders, else the 2nd `AttachInvoice` violates the PK → 502 (see counter-based mock in
`tests/api/order/get_test.go`).

`GET /orders` returns all orders (attrs/history/invoices, newest first). **Registered only when
`cfg.Server.APIKey != ""`** (handler doesn't check the key). Since `POST /orders` is registered unconditionally on the
same path, an unregistered `GET` → 405. Conditional registration in `app.go` is the single source of truth.

### `GET /orders/{id}`

`GetOrderStatus` → `{"status": "<order_status>"}` by UUIDv7. Path param validated by OpenAPI (`format: uuid`) →
malformed 400. Valid UUID not in DB → 404 `{"error": "order not found"}`. Public (no API key).

### `POST /monobank/webhook`

`MonobankWebhook` processes invoice-status webhooks. Authed by `X-Sign`; no API key/CORS/OpenAPI/rate-limit (Monobank
is sole caller). Reads ≤1 MB, verifies via `h.mbVerifier`, parses with `ParseWebhook`, maps `Status` →
`invoice_status` via `monobankStatusToInvoiceStatus` (`created` → 200, no DB write). Builds `order.InvoiceEvent{…,
Payload: payload.RawBody, EventAt: payload.ModifiedDate}` and calls `RecordInvoiceEvent`.

**Response: status code only, empty body** via `w.WriteHeader` (never `writeError`). Codes by retryability: 200 for
processed/idempotent/informational/unknown-reference; 401 bad signature; 400 malformed; **500 for transient DB/
verifier-transport errors so Monobank retries**. The only handler that returns 500 as a retry signal.

`event_at` = Monobank's `modifiedDate` (not `CURRENT_TIMESTAMP`) → out-of-order webhooks resolve via the provider's
authoritative timestamp (e.g. `processing@t1 → failure@t2 → processing@t3 → success@t4` → `paid`).

Lifecycle: `order.ShouldApplyInvoiceTransition(current, candidate)` (`internal/order/lifecycle.go`). Invoice events
freely drive the pre-paid cluster {`new`, `awaiting_payment`, `payment_processing`, `payment_hold`, `cancelled`};
`cancelled` re-enterable; `paid` stable against payment_*/`cancelled` (only `refunded` advances it); fulfillment
states (`processing`, `shipped`, `delivered`, `refund_requested`, `returned`) are operator-owned and ignore invoice
events except `refunded`, which always wins.

### `GET /nova-poshta/cities`, `/branches`, `/streets`

`cities?q=` → `searchSettlements`; `branches?city_ref=&q=` → `getWarehouses`; `streets?city_ref=&q=` →
`searchSettlementStreets`. `q` required everywhere; `city_ref` required for branches/streets — missing → 400. NP
failure → 502.

### `GET /images/{product_id}/{file_name}`

`ServeImage` serves `{data_dir}/products/{product_id}/images/{file_name}` via `http.ServeFile`; `filepath.Base` on
both values (traversal guard). Functest: only the image file is needed to serve it; `product.yaml` is needed only for
the product to appear in `catalog.Products`.

## Configuration

Keys in `internal/app/config.go`:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, **`Server.PublicURL`** — required; empty → startup error.
- **`Server.PublicURL`** — public https base URL of this service. Derives the webhook URL (`<PublicURL>/monobank/
  webhook`, sent as `webHookUrl`) and basket icon (`<PublicURL>/images/<product_id>/<preview>`). Trailing slash
  trimmed in `NewHandler`. **No separate `monobank.webhook_url` config.**
- **`Monobank.ServiceURL`**, **`NovaPoshta.ServiceURL`**, **`Telegram.ServiceURL`**, **`Resend.ServiceURL`** —
  optional; empty → real upstream. Tests inject `httptest.Server.URL` via `testapp.New` opts.
- **`Telegram.Token`** + **`Telegram.ChatID`** — both optional; both empty → disabled; exactly one → startup error
  (`"telegram: token and chat_id must be set together"`); both → enabled. `defer tn.Stop()` registered **after**
  `defer db.Close()` so LIFO drains pending events while the pool is open.
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
(use `RENAME`, not `DROP`; see `TestCreateOrder_DBFailure`).

**`t.Cleanup` runs after `defer`.** If a test `defer pool.Close()` then registers a cleanup using the pool, the
cleanup silently fails. Close the pool inside the cleanup, or let the cleanup own teardown.

Fixtures — loader unit tests (`internal/loader/`): `makeProductDir(t, dataDir, id, yaml, extraFiles)`,
`makeProductsFile(t, dataDir, content)`. API functional tests (`tests/api/product/`): `makeDataDir(t, productsYAML,
productYAMLs)`; `testapp.New(t, dataDir, opts...)` (mutators override fields). **`testapp.New` does not start the app**
— call `a.Start()` before requests. Subtests needing a separate empty catalog start their own `testapp` (random port).

Orchestration: all subtests in a `TestFoo` share one `testapp` (per-subtest instances panic on port conflict under
`main.Parallel()`). At most one top-level `TestFoo` per package may `main.Parallel()` if each starts its own
`testapp`; new app-starting functions must NOT `main.Parallel()` unless existing ones are refactored to share. Tests
issuing many synchronous requests set `cfg.RateLimit = -1`.

### Shared helpers

Keep shared test helpers in `handler_test.go`, not per-feature files, so they survive feature removal.

### Shared ECDSA test key (`tests/api/order/`)

A single `testECDSAKey` is generated in `init()`. `pubKeyPayload(t)` returns the PEM pubkey in the Monobank pubkey
JSON; `signWebhookBody(t, body)` computes the ECDSA-SHA256 signature `POST /monobank/webhook` expects. All Monobank
stubs in this package use `pubKeyPayload` for `/api/merchant/pubkey` — no test generates its own key.
