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
- `data/` is a **symlink** to `../shop-content/data` (separate git repo). Commit files under it there:
  `git -C "$(dirname "$(readlink data)")" add ...`. `git add data/...` here fails ("beyond a symbolic link").
- `vendor/` is gitignored (regen via `go mod vendor`); only `go.mod`/`go.sum` are staged. `go mod tidy` strips modules
  no real import reaches — to land a dep before its consumer, `go get` it (records `// indirect`) and **skip `go mod
  tidy`** until the importer lands. `go mod vendor` populates `vendor/` only once a `.go` file imports the module.

## Project overview

`simshop` is a Go HTTP API (`github.com/ashep/simshop`). Stack: filesystem catalog (products from YAML at startup, no
product DB), PostgreSQL for orders, zerolog, `go-app` framework (`github.com/ashep/go-app`), OpenAPI spec in `api/` for
request/response validation. Config from `config.yml` + env; data from `data_dir` (default `./data`). Use `api/` as
context for new features.

Dirs: `internal/` (app, cli, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product, resend,
shop, sql, telegram), `api/`, `frontend/` (static storefront, served by Caddy), `tests/` (build tag `functest`),
`vendor/`. Server entrypoint is repo-root `main.go`; the CLI is a separate binary (`cmd/simshop/main.go`).

### CLI (`cmd/simshop` + `internal/cli`)

Operator CLI, a **separate binary** from the server. All logic in `internal/cli` (cobra). Pure HTTP client over the
API — unit + `httptest` tests only, no functest.

`~/.simshop.yaml`: top-level mapping shop-name → `{url, api_key, default?}`, parsed via `yaml.Node` to preserve
insertion order. Shop precedence: `--shop` flag → `default: true` → first in file order.

Commands: `order list [--status csv]`, `order get <id>`, `order set-status <status> <id> [<id>...]`, `shops`.

- `order list` with no `--status` defaults to **active** statuses (full enum minus terminal set `delivered,
  cancelled, returned, refunded`). `--status all` clears the filter.
- `order get` has **no single-record endpoint** — fetches GET /orders and filters client-side.
- `order set-status`: status first, then ≥1 ids, applied independently — a per-id failure is reported and the rest
  still run; exits non-zero with `"N of M orders failed"`. Client validates status against an allow-list for a fast
  error, but the **server is authority** on legal transitions (409) and tracking-iff-`shipped` (400).
- **Short ids** (unique UUID prefix) accepted by `get`/`set-status` alongside full UUIDs (`resolve.go`
  `matchOrder`: exact-then-unique-prefix, case-insensitive; no match → "not found", >1 → "ambiguous").
- `shops` masks the api key as `<hidden>`.

## Conventions

### Errors and status codes

Handlers map domain errors via `h.writeError(w, err)`. Five typed errors in `internal/handler/handler.go`:
`BadRequestError` (400), `NotFoundError` (404), `BadGatewayError` (502, upstream), `UnauthorizedError` (401),
`ConflictError` (409, illegal transition). Each has a `Reason string` — always populate; written as
`{"error": "<reason>"}`. Domain errors live in service packages; handler maps via `errors.Is`.

### OpenAPI

Validator (`kin-openapi`) runs in 3.0 compatibility mode:

- No 3.1 array-type (`type: ["string","null"]`); use `type: string` + `nullable: true`.
- UUID path params: `type: string` + `format: uuid`, never `type: uuid` (panics at startup).
- CSV query arrays (`?status=a,b`): `in: query` + `style: form` + `explode: false` + `schema.type: array` with
  item-level `enum`. Validator decodes CSV and rejects unknown values with 400 — don't redeclare the enum in Go.
- `?status=` decodes to `[""]` → fails enum → 400. "No filter" tests must omit the param; `allowEmptyValue: true`
  does NOT rescue this.
- Full `order_status` lifecycle lives in the `OrderStatus` component schema; `$ref` it from any endpoint
  surfacing/accepting a full-enum status. Operator-only subsets keep inline enums. The Postgres `order_status` enum
  in `internal/sql/001_init.up.sql` is the runtime authority — edit it together with the OpenAPI schema.

### HTTP middleware

Chain (inner→outer): `content-type → openapi validation → handler`.

- **CORS**: unconditionally sets `Access-Control-Allow-Origin: *`. **Explicit `OPTIONS` routes must be registered
  alongside each `GET` in `app.go`** — stdlib mux has no implicit OPTIONS.
- **Rate limit**: per-IP sliding window `60s/rpm`. `cfg.RateLimit`: positive → RPM; **`0`/negative → disabled**
  (functests use `-1`).
- **API key**: `Authorization: Bearer <token>`, constant-time compare. Empty `apiKey` is a programmer error — only
  register protected routes when `cfg.Server.APIKey != ""`; don't push the empty check into the middleware.

### Money

Integer minor units (cents) across the domain boundary, never float. **Round once per amount; don't multiply
totals** (bias compounds).

### Country codes

Lowercase ISO 3166-1 alpha-2. Two distinct sets: **allow-list** `shop.Countries` (authoritative for who can order,
validated by `CreateOrder` before reading product YAML) vs **price keys** `prices.<country>` per `product.yaml`
(independent of the allow-list; `"default"` is required and is a price fallback, not an allowed country). A country
can be allowed without a per-product price (falls back to `prices.default`).

### Image path transformation

YAML stores bare filenames; clients receive `/images/{id}/<filename>`. Rewrite in **two** places: (1) loader
`loadProduct`, after `validate` (which needs bare filenames for `os.Stat`); (2) `ServeProductContent`, which reads
`product.yaml` directly. Same for `attr_images`. Any new handler reading `product.yaml` directly must apply this.

### Gallery media: images and video share one list

`Product.Images` (`[]ImageItem`) is a **single ordered media list** of stills and MP4 clips — no separate `videos`
field. Discriminator `ImageItem.Type` (empty/`"image"` vs `"video"`) must stay on both the Go struct and the
`ImageItem` schema. Unified (not split) because `attr_images`/carousel are index-based into it.

### Customer language on orders

`orders.lang` carries the checkout language from `req.Lang` on `POST /orders`; the email notifier resolves
`emails/{status}/{lang}.md` from it. Tests seeding `orders` directly must set a non-empty `lang` (column NOT NULL).

## Packages

### internal/handler

`internal/app` imports `internal/handler`, so **`handler` must never import `app`** (same for `geo`, `monobank`,
`novaposhta`, `order`): define a local interface in `handler` and wire the concrete type in `app`.

`orderService` interface: `Submit`, `AttachInvoice`, `List`, `GetStatus`, `UpdateStatus`, `RecordInvoiceEvent`. Do
**not** add delegating `order.Service` methods for every new `Reader` method — only proxy what `orderService` needs;
other consumers (workers) take `order.Reader` directly.

`order.Service`'s `Notify` fires **only after a successful commit**. `RecordInvoiceEvent` dispatches only on a
non-empty `newStatus` (forward transition).

### internal/geo

`Detector.Detect(r)` precedence: (1) header `CF-IPCountry`, validated as exactly two ASCII letters, else falls
through (anti-spoof); (2) client IP → in-memory cache (TTL 1h) → `ipinfo.io` on miss. `clientIP` uses
`X-Forwarded-For` only when `CF-IPCountry` is absent (XFF is spoofable without Cloudflare). Cache writes on success
only.

### internal/novaposhta

**Branches/streets use `SettlementRef`, not `CityRef`** (old `getCities` ref → "City not found"). `success=false`
is an error regardless of HTTP status.

### internal/monobank

**Error policy: generic responses, detailed logs.** All failures (timeout, non-2xx, parse, app error, unsupported
currency, tx2) → 502 `{"error":"bad gateway"}` to the client; full detail (`order_id`, `monobank_status`,
`monobank_err_code`, ...) logged server-side via `errors.As` on `*APIError`.

`Verifier` verifies `X-Sign` (ECDSA-P256/SHA-256). **On failure the pubkey is refetched once and rechecked**
(recovers from key rotation); `ErrInvalidSignature` only on persistent mismatch. Concurrent refetches deduped via
`atomic.Bool`.

### internal/telegram, internal/resend

Both implement `order.Notifier`, non-blocking `Notify` (full buffer/closed channel → drop + Warn), single
background retry worker (≤3 attempts; 4xx≠429 permanent; 429 clamped `[1s,30s]`; else fixed backoff schedule).

Resend's status filter (only `paid`, `processing`, `shipped`, `delivered`, `refund_requested`, `refunded` dispatch)
is duplicated in `internal/app/email_validate.go`'s `notifyStatuses` for the startup `en.md` check — **edit both
together**.

`resend.TemplateStore`: Markdown → HTML via `goldmark` **after** `text/template` substitution (so customer names
with markdown chars aren't mangled). Lang fallback `(status, lang)` → `(status, "en")`.

### internal/order

`MultiNotifier` fans out over `[]Notifier`; `app.Run` wraps enabled notifiers in it only with 2+ (one → pass child
directly, zero → nil). A panic in any child is recovered silently.

`UpdateStatusByOperator` returns `(applied bool, err error)`: `applied=false`+nil means the order was already at the
target under `SELECT FOR UPDATE` (concurrent same-target write) — caller must NOT notify. `NewService` takes
`ow OperatorWriter` as the **5th positional arg** (before notifier); pass `nil` only in tests not exercising
UpdateStatus.

### internal/orderdb

`Writer.Write` (order header + order_attrs + initial `order_history`) is **transactional**.

`Attr.Name`/`Attr.Value` store **rendered titles** in the customer's language, not raw IDs (orders stay
self-documenting and immutable to YAML renames) — `handler.CreateOrder` resolves titles first.

`Writer.UpdateStatusByOperator` writes `tracking_number` write-once via a `CASE WHEN` SQL guard (defense-in-depth;
handler already enforces "tracking required iff `shipped`").

**`RecordInvoiceEvent(ctx, evt)`** is the single entry point for provider invoice events, one tx: `SELECT ... FOR
UPDATE` on `orders` (serializes webhooks) → idempotent insert into `invoice_history` (`ON CONFLICT (invoice_id,
provider, status, event_at) DO NOTHING`) → re-derive current status from the timeline (out-of-order tolerant) → map
via `order.InvoiceStatusToOrderStatus` and apply iff `order.ShouldApplyInvoiceTransition`. Payloads live only on
`invoice_history.payload`, not `order_history`.

**No unit tests in `internal/orderdb`** — `pgx.Tx` is impractical to stub; `Writer`/`Reader` are exercised by
`tests/api/order/` against real PG. Don't reintroduce a stub-based unit test without a real `pgx.Tx`.

### internal/loader

Missing `data_dir`/`products/` → empty catalog (not an error). Malformed YAML or validation failure → **fatal at
startup**. `{data_dir}/shop.yaml` is **mandatory**: missing → fatal, and `shop.countries` must be non-empty. `products/
products.yaml` missing → empty list, not an error.

`validate` (fatal at startup) checks: name/description non-empty and same language set; every spec/attr entry covers
exactly `name`'s languages (attrs need ≥1 value each); `prices` contains `"default"`; every image
`preview`/`full`/`attr_images` filename exists under `{productDir}/images/`.

## Routes

Full request/response shapes live in `api/root.yaml` — this section covers cross-file gotchas only.

- **`GET /products/{id}/{lang}`**: categories live only in the listing (`product.yaml` has none) —
  `ServeProductContent` cross-references them by matching `id` against `h.prod.List()`; a product missing from the
  listing gets no categories. Price-field naming gotcha: YAML `prices:` / Go `Product.Prices`, but the resolved
  single-country price serialises as `"price"`.
- **`GET /product`** (singular, crawler preview): registered **without** `openapiMw`/`corsMw`, **not** in
  `api/root.yaml`. **Never returns non-200 to a crawler** — invalid/unknown ID falls back to shop-level OG tags.
- **`POST /orders`**: two-phase commit — tx1 `Submit` (`status='new'`, DB is truth before Monobank knows) →
  Monobank `CreateInvoice` (failure leaves order at `new`, 502, reconciled out of band) → tx2 `AttachInvoice`
  (`status='awaiting_payment'`). `country` is **request-supplied, not geo-detected** (drives price + stored
  country); geo detection is for `ServeProductContent` only. `order_invoices` has **no unique constraint on
  `order_id`** (re-issued invoices); PK is `(id, provider)` where `id` is the provider's invoice id — a mock
  serving multiple orders must return distinct ids per call.
- **`GET /orders`**: registered only when `cfg.Server.APIKey != ""`; `POST /orders` is registered unconditionally
  on the same path, so an unregistered `GET` → 405 (conditional registration in `app.go` is the single source of
  truth).
- **`POST /monobank/webhook`**: authed by `X-Sign` only (no API key/CORS/OpenAPI/rate-limit). Response is status
  code only, empty body. By retryability: 200 processed/idempotent/unknown-reference; 401 bad signature; 400
  malformed; **500 for transient DB/verifier-transport errors so Monobank retries** (the only handler using 500 as
  a retry signal). `event_at` = Monobank's `modifiedDate`, not `CURRENT_TIMESTAMP`.
  Lifecycle (`order.ShouldApplyInvoiceTransition`): invoice events freely drive the pre-paid cluster {`new`,
  `awaiting_payment`, `payment_processing`, `payment_hold`, `cancelled`}; `paid` is stable against payment_*/
  `cancelled` (only `refunded` advances it); operator-owned fulfillment states ignore invoice events except
  `refunded`, which always wins.
- **`GET /assets/{path...}`**: containment guard is `filepath.Join(dataDir, "assets", path)` + `HasPrefix` check —
  `filepath.Base` is unusable here since paths are multi-segment.

## Frontend & deployment

`frontend/www/` is a static storefront — vanilla HTML/CSS/JS, **no framework, no build step**, served by **Caddy,
not the Go app**. `js/i18n.js`: **every key must have both `en` and `uk`** (`main.js` `t()` falls back to the key
otherwise). API base is `<origin>/api`, hardcoded in `api.js`.

`Caddyfile`: `/api*` reverse-proxied to `:9000` with `/api` stripped; `/product` routes to backend
`ServeProductPreview` for bots (User-Agent regex) vs `product.html` for humans; SPA fallback to `index.html`.
`entrypoint.sh` runs both the Go server and Caddy, exiting when either child exits.

## Configuration

Keys in `internal/app/config.go`:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, **`Server.PublicURL`** — required; empty → startup error.
- **`Server.PublicURL`** derives the webhook URL (`<PublicURL>/monobank/webhook`) and basket icon URL — **no
  separate `monobank.webhook_url` config.**
- **`Telegram.Token`** + **`Telegram.ChatID`** — both optional; exactly one set → startup error.
- **`Server.APIKey`** — optional; empty disables `GET /orders` via conditional registration.
- **`RateLimit`** — positive → RPM/IP; `0`/negative → disabled.
- **`Resend.APIKey`** — optional; empty disables the email notifier. **`Mail.From`**, **`Mail.OrderURL`** (literal
  `{id}` pattern) required when it's set.
- `Monobank.ServiceURL`/`NovaPoshta.ServiceURL`/`Telegram.ServiceURL`/`Resend.ServiceURL` — optional, empty → real
  upstream; tests inject `httptest.Server.URL` via `testapp.New` opts.

`uuidv7()` is built-in PostgreSQL 18 (no extension); test container pins `postgres:18-alpine`.

## Tests

### Running

```
task go:test:unit -- [FLAGS]
task go:test:func -- [FLAGS]
```

In worktrees without `.ci/`, run directly: `go test -run TestFoo -v ./internal/...` or
`go test -tags=functest ./tests/...`. Dirty PG → `task go:test:func:clean`.

### Unit tests

- Before implementing, invoke `superpowers:test-driven-development`.
- Before claiming done, invoke `superpowers:verification-before-completion` and run **both** `task go:test:unit --
  ./...` and `task go:test:func -- -v` — both must pass. Then `task go:golangci-lint`. No summary before tests pass.
- Group related tests: `TestFoo(main *testing.T)` + `main.Run("Case", ...)`. Never separate `TestFoo_Case` functions.
- New file in a package with `_test.go` companions → write its unit test in the same task.
- Handler responses with `id` `format: uuid`: mock returns must be valid UUIDs, else the validator → 500 masks your
  assertion.

### Functional tests

PG persists across runs — truncate explicitly (see `tests/api/order/post_test.go` `truncateOrders`). After a
destructive test, restore schema in `t.Cleanup` (use `RENAME`, not `DROP`). **`t.Cleanup` runs after `defer`** — a
test that `defer pool.Close()` then registers a cleanup using the pool silently fails; close the pool inside the
cleanup instead.

`testapp.New(t, dataDir, opts...)` **does not start the app** — call `a.Start()` before requests. All subtests in a
`TestFoo` share one `testapp` (separate instances panic on port conflict under `main.Parallel()`); subtests needing
a separate empty catalog start their own `testapp` (random port). Tests issuing many synchronous requests set
`cfg.RateLimit = -1`.

### Shared helpers

Keep shared test helpers in `handler_test.go`, not per-feature files, so they survive feature removal.

### Shared ECDSA test key (`tests/api/order/`)

A single `testECDSAKey` is generated in `init()`. `pubKeyPayload(t)`/`signWebhookBody(t, body)` are used by all
Monobank stubs in this package — no test generates its own key.
