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
- Schema changes go in a **new numbered migration** `internal/sql/NNN_<name>.{up,down}.sql` (picked up by `go:embed`
  + `dbmigrator`); never edit an already-applied file — existing DBs (local dev, prod) would never receive it. Use
  `IF NOT EXISTS`/`IF EXISTS` so a DB patched by hand still migrates cleanly.
- `data/` is a **symlink** to `../shop-content/data` (separate git repo). **Never commit data from this repo** —
  "commit everything" here excludes it. Commit it only when asked, in its own repo:
  `git -C "$(dirname "$(readlink data)")" add ...`. `git add data/...` here fails ("beyond a symbolic link").
- `vendor/` is gitignored (regen via `go mod vendor`); only `go.mod`/`go.sum` are staged. `go mod tidy` strips modules
  no real import reaches — to land a dep before its consumer, `go get` it (records `// indirect`) and **skip `go mod
  tidy`** until the importer lands. `go mod vendor` populates `vendor/` only once a `.go` file imports the module.

## Project overview

`simshop` is a Go HTTP API (`github.com/ashep/simshop`). Stack: filesystem catalog (products from YAML at startup, no
product DB), PostgreSQL for orders, zerolog, `go-app` framework (`github.com/ashep/go-app`), OpenAPI spec in `api/` for
request/response validation. Config from `config.yaml` + env; data from `data_dir` (default `./data`). Use `api/` as
context for new features.

Dirs: `internal/` (app, cli, handler, geo, loader, monobank, novaposhta, openapi, order, orderdb, page, product, resend,
shop, sql, telegram), `api/`, `tests/` (build tag `functest`),
`vendor/`. Server entrypoint is repo-root `main.go`; the CLI is a separate binary (`cmd/simshop/main.go`).

### CLI (`cmd/simshop` + `internal/cli`)

Operator CLI, a **separate binary** from the server. All logic in `internal/cli` (cobra). Pure HTTP client over the
API — unit + `httptest` tests only, no functest.

`~/.simshop.yaml`: top-level mapping shop-name → `{url, api_key, default?}`, parsed via `yaml.Node` to preserve
insertion order. Shop precedence: `--shop` flag → `default: true` → first in file order.

Commands: `order list [--status csv]`, `order get <id>`, `order set-status <status> <id> [<id>...]`, `shops`.

- `order list` (and the TUI's ACTIVE scope) with no `--status` defaults to **active** statuses (full enum minus
  `delivered, cancelled, refunded`). `returned` is deliberately active — it still awaits a refund. `--status all`
  clears the filter.
- `order get` has **no single-record endpoint** — fetches GET /orders and filters client-side.
- `order set-status`: status first, then ≥1 ids, applied independently — a per-id failure is reported and the rest
  still run; exits non-zero with `"N of M orders failed"`. Client validates status against an allow-list for a fast
  error, but the **server is authority** on legal transitions (409) and tracking-iff-`shipped` (400).
- **Short ids** (unique UUID prefix) accepted by `get`/`set-status` alongside full UUIDs (`resolve.go`
  `matchOrder`: exact-then-unique-prefix, case-insensitive; no match → "not found", >1 → "ambiguous").
- `shops` masks the api key as `<hidden>`.
- Bare `simshop` (or `simshop tui`) opens the Bubble Tea TUI (`tui.go`/`tui_view.go`). Status changes go only
  through the `s` picker (no per-status shortcuts, by request), which calls `order.ShouldApplyOperatorTransition`
  directly (that package is stdlib-only) — don't copy the transition map into `cli`. Tests drive the model by feeding `Update` and running returned cmds; never run `Init()`'s batch — it
  contains the 30s refresh tick and blocks.
  The dashboard screen (`tui_dashboard.go`, `tab` toggles orders⇄dashboard) is a second screen in the same model; it
  reloads `GET /stats` on every `tab` into it (`openDashboard` → `loadStats`) and drops a `statsMsg` whose period no
  longer matches (stale reply after `p`).

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
  (created in `internal/sql/001_init.up.sql`) is the runtime authority — change it (via a new migration) together
  with the OpenAPI schema.

### HTTP middleware

Chain (inner→outer): `content-type → openapi validation → handler`.

- **CORS**: unconditionally sets `Access-Control-Allow-Origin: *`. **Explicit `OPTIONS` routes must be registered
  alongside each `GET` in `app.go`** — stdlib mux has no implicit OPTIONS.
- **Rate limit**: per-IP sliding window `60s/rpm`. `cfg.RateLimit`: positive → RPM; **`0`/negative → disabled**
  (functests use `-1`). Applied only to `POST /orders` and `GET /products/{id}/promo-codes/{code}`, each with **its own
  limiter instance** (the promo check is the sole brute-force guard on codes and must not eat the order budget).
- **API key**: `Authorization: Bearer <token>`, constant-time compare. Empty `apiKey` is a programmer error — only
  register protected routes when `cfg.Server.APIKey != ""`; don't push the empty check into the middleware.

### Money

Integer minor units (cents) across the domain boundary, never float. **Round once per amount; don't multiply
totals** (bias compounds).

### Product availability

`Product.Available *bool` in `product.yaml`; absent → `nil` → treated as `true` everywhere. `CreateOrder` checks
it before any DB write — absent or `true` → proceed; explicit `false` → 400 `"product not available"`.
`ServeProductContent` and `joinProductImages` (loader) convert nil/true → `bool true` on `ProductDetail.Available`
and `Item.Available` respectively; both fields are serialized to JSON and are `required` in the OpenAPI schemas.

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
`attr_images` is **two-way** in `main.js`: a variant click jumps the carousel (`_goTo`), and landing on a mapped slide
selects that value (`carousel._onSlide`, set by `renderVariants`; it never calls `_goTo`, so no loop).
The fullscreen lightbox pages through the same list and drives the page carousel via `_goTo` on every step, so
the URL `?img=` and the selected variant stay in sync after it closes.
Listing `Item.Images` (≤ `product.ItemMaxImages` = 2 stills, `Images[0]` == `Image`) feeds the card hover swap: pure
CSS, `.card-image-alt` fades in over the opaque first image (fading both lets the background flash through), only
under `@media (hover: hover)` so a tap doesn't stick. A position-following scrub was tried and rejected — don't redo it.

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

**One promo code per phone**: `Writer.Write` takes `pg_advisory_xact_lock` on `(lower(code), phone digits)`, then
rejects with `order.ErrPromoCodeUsed` (→ 400) if a **non-`cancelled`** order matches. `new` must count — it's the
state of a concurrent order between tx1 and tx2, so excluding it reopens the race. Functests creating 2+ orders on
the shared Monobank stub rely on it returning a distinct `invoiceId` per call (derived from `reference`).

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
startup**. `{data_dir}/shop.yaml` is **mandatory**: missing → fatal, and `shop.countries` must be non-empty.

There is **no `products.yaml`**: the catalog listing is `shop.products` (ordered ids) and each `Item` is built from
that product's `product.yaml` (`Title`←`name`, `Description`←`summary`, `categories`), so `loadShop` must run before
`buildProductItems`. Unknown/duplicate listed id → fatal; unlisted products load but aren't listed. `Shop.Products`
is `json:"-"` (loader input, not part of `GET /shop`). `dropEmptyCategories` then removes `shop.categories` with no
*listed* product — filtering is server-side so all three frontend category UIs (home rows, catalog tabs, footer) agree;
shop tests declaring categories must list a product in each.

`validate` (fatal at startup) checks: name/description non-empty and same language set; every spec/attr entry covers
exactly `name`'s languages (attrs need ≥1 value each); `prices` contains `"default"`; every image
`preview`/`full`/`attr_images`/`cover` filename exists under `{productDir}/images/` (cover must not be a video); every
`attr_colors` value is `#rrggbb`. `Product.Cover` stays a **bare filename** after loading (unlike `Images`) —
`joinProductImages` and the OG preview build `/images/{id}/...` themselves.
`attr_colors` is language-independent (like `attr_images`), not a field on the per-language `AttrValue` — that would
force duplicating the hex in every language block.

`Product.PromoCodes` is `json:"-"` and must **never** be serialized — any product response including it leaks every
code. The only exposure is the single-code check `GET /products/{id}/promo-codes/{code}` (functest
`ProductEndpointsDoNotLeakCodes` guards this). Matching is case-insensitive, hence the loader's case-insensitive
duplicate check.

## Routes

Full request/response shapes live in `api/root.yaml` — this section covers cross-file gotchas only.

- **`GET /products/{id}/{lang}`**: price-field naming gotcha: YAML `prices:` / Go `Product.Prices`, but the resolved
  single-country price serialises as `"price"`.
- **`GET /product/{id}` and `GET /product/{lang}/{id}`** (singular, crawler preview; id/lang from path values): registered
  **without** `openapiMw`/`corsMw`, **not** in `api/root.yaml`. **Never returns non-200 to a crawler** — invalid/unknown
  ID falls back to shop-level OG tags. The lang-first form `/{lang}/product/{id}` must not be registered: it overlaps
  `/products/{id}/{lang}`/`/pages/{id}/{lang}` and the stdlib mux panics at startup.
- **`GET /stats?days=7|14|30`**: registered (with `OPTIONS`) only when `Server.APIKey` **and** the `cloudflare`
  block are set; otherwise it is unregistered (404 = "not configured", the TUI maps it to `ErrStatsNotConfigured`).
  Daily chart is **visits** (`sum.visits`; per Cloudflare's definition only pageloads arriving from another site, not
  verified live), top products are **views**
  (`count` per `requestPath`, language variants summed per id) — internal navigation is a view but not a visit, so
  don't compare the two. Report is cached 10 min per `days` in `internal/stats`; any Cloudflare failure →
  `ErrUpstream` → generic 502 (detail logged). The Web Analytics beacon itself is the theme's job
  (`cloudflare_analytics.token` in `shop.yaml`).
  Both queries were checked against the live schema (2026-10-25, Free plan). Live `settings` for
  `rumPageloadEventsAdaptiveGroups`: `maxDuration` 93 days, `notOlderThan` 184 days, `maxPageSize` 10000. A 30-day
  query fits in one request, so there is **no range splitting**; add it only if `days` ever exceeds 93. The API token
  only needs Account Analytics: Read. It can't list RUM sites, so `site_tag` (the id in the dashboard's Web Analytics
  "Manage site" URL) is confirmed only once beacon data arrives.
- **`POST /orders`**: two-phase commit — tx1 `Submit` (`status='new'`, DB is truth before Monobank knows) →
  Monobank `CreateInvoice` (failure leaves order at `new`, 502, reconciled out of band) → tx2 `AttachInvoice`
  (`status='awaiting_payment'`). `country` is **request-supplied, not geo-detected** (drives price + stored
  country); geo detection is for `ServeProductContent` only. **Promo code**: an invalid non-empty `promo_code` → 400
  (never a silent full-price order); `product.Discount` works off the full subtotal incl. add-ons and
  rounds the **discounted total** to whole units (no kopecks; integer math), and the storefront (`promoTotalCents`)
  repeats the same formula — change both together. `orders.price` stays the charged
  total; `promo_code`/`discount_percent`/`discount` are all-NULL or all-set (`orderdb.scanPromo` keys off
  `promo_code`). `order_invoices` has **no unique constraint on
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

**No frontend lives here.** The image's Caddy serves a theme from `/frontend` that downstream images `COPY` in
(reference theme + its UI rules: `../shop-content/frontend`, its `CLAUDE.md`). Theme contract (README "Frontend
theme"): `index.html`, `product.html`, `order-status.html`; API at `<origin>/api`. Changing routes in `Caddyfile`
changes that contract — update the README and the theme together. Locally `frontend` is a gitignored symlink.

Storefront content in `shop.yaml` (`announcement`, `announcement_page`, `menu`, `hero`, `features`, `story`, `gallery`)
is optional and passed through as-is; image fields are bare `assets/` filenames, not validated. `hero.images` is a
slideshow (rotated by the theme); there is no single `hero.image` field. Each slide is a `HeroImage` (custom
`UnmarshalYAML`: bare filename or `{file, category}`; JSON always `{file, category?}`); a click on a slide's
`img[data-href]` opens its category's catalog, unknown/dropped ids fall back to `/catalog`. `Category` has a custom
`UnmarshalYAML`: YAML keeps language keys flat (`{id, en, uk}`), so `id` is reserved — a new non-language key must be
added to `categoryReservedKeys` or it becomes a bogus language. Social `links` is a flat list shared by all languages
(not a per-language map) and renders **only in the footer**, as text; `links[].icon` is used just to find the Instagram
link for the home gallery. `GET /products` resolves `price` per request and **skips geo detection unless some product
has a non-`default` price** (avoids an ipinfo.io call per listing).

Local run: `task dev` (backend + Caddy on :8104 serving the `frontend` symlink, generated `tmp/Caddyfile.dev`).
`dev:api` deliberately builds **without the `netgo` tag**: on macOS the pure-Go resolver ignores system DNS and the
startup Monobank pubkey fetch times out, while `go:run` (with `netgo`) fails. `dev:api` runs under **`air`**
(`.air.toml`): ldflags read `$APP_NAME`/`$APP_VERSION` from the task's `env` (air's `build.cmd` runs via `sh -c`);
`.sql`/`.yaml` are watched because they're `go:embed`-ed. Air never descends into the `data` symlink (`follow_symlink`
only affects `exclude_unchanged` checksums), so the task passes `-build.include_dir "$(realpath data)"` — air ≥1.67
watches out-of-root include dirs as extra roots.
Env vars (`SIMSHOP_*`/`APP_*`) are loaded **before** `config.yaml`, so they can't override it locally; point a run at
another DB with an overlay file via `APP_CONFIG_PATH` (loaded after `config.yaml`). Checking the theme in the desktop
browser pane requires the pane to be **visible**: init waits two `requestAnimationFrame`s, which never fire while hidden.

`Caddyfile`: `/api*` reverse-proxied to `:9000` with `/api` stripped; `/product/{id}` and `/{lang}/product/{id}` route
to backend `ServeProductPreview` for bots (User-Agent regex; the lang form is rewritten to `/product/{lang}/{id}`, since
the backend can't register lang-first) vs `product.html` for humans; SPA fallback to `index.html`. Storefront URLs are
path-based and language-prefixed (`/<lang>/`, `/<lang>/catalog/<category>`, `/<lang>/product/<id>`,
`/<lang>/order/status`); legacy query-string URLs (`/product?id=&lang=`, `/catalog?category=`, `?lang=`) 301 to them,
with id/lang/category regex-validated in the matcher so a crafted value can't make an open redirect. Path URLs mean
**theme HTML must use absolute asset paths** (`/main.js`), or `/uk/product/x` resolves them under `/uk/product/`.
`entrypoint.sh` runs both the Go server and Caddy, exiting when either child exits.

## Configuration

Keys in `internal/app/config.go`:

- **`Monobank.APIKey`**, **`Monobank.RedirectURL`**, **`Server.PublicURL`** — required; empty → startup error.
- **`Server.PublicURL`** derives the webhook URL (`<PublicURL>/monobank/webhook`) and basket icon URL — **no
  separate `monobank.webhook_url` config.**
- **`Telegram.Token`** + **`Telegram.ChatID`** — both optional; exactly one set → startup error.
- **`Cloudflare.APIToken`** + **`AccountID`** + **`SiteTag`** — all-or-none (partial → startup error); all set
  enables `GET /stats` (needs `Server.APIKey` too). `ServiceURL` is the test override.
- **`Server.APIKey`** — optional; empty disables `GET /orders` via conditional registration.
- **`RateLimit`** — positive → RPM/IP; `0`/negative → disabled.
- **`Resend.APIKey`** — optional; empty disables the email notifier. **`Mail.From`**, **`Mail.OrderURL`** (literal
  `{id}` pattern) required when it's set.
- `Monobank.ServiceURL`/`NovaPoshta.ServiceURL`/`Telegram.ServiceURL`/`Resend.ServiceURL` — optional, empty → real
  upstream; tests inject `httptest.Server.URL` via `testapp.New` opts.

Local dev PG on `:5432` is **shared with other projects** — the gitignored `config.yaml` DSN must use the dedicated
`simshop` database, never `postgres` (another project's migrator owns its `schema_migrations` → "no migration found
for version N").

`uuidv7()` is built-in PostgreSQL 18 (no extension); test container pins `postgres:18-alpine`.

## Tests

### Running

```
task go:test:unit -- [FLAGS]
task go:test:func -- [FLAGS]
```

In worktrees without `.ci/`, run directly: `go test -run TestFoo -v ./internal/...` or
`go test -tags=functest ./tests/...`. Dirty PG → `task go:test:func:clean`.

Bumping the `go` directive in `go.mod` also requires bumping the `tests` image in `docker-compose.tests.yaml` (CI runs
functests in it with `GOTOOLCHAIN=local`, so an older image fails), and reinstalling local `govulncheck`/
`golangci-lint` with the new toolchain (they refuse to load a module newer than the Go they were built with).

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
