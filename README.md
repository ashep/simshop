# SimShop

SimShop is an HTTP API service that serves catalog data (products, pages, shop metadata) loaded from YAML files at
startup and persists customer orders to PostgreSQL.

## Purpose

SimShop is designed as a lightweight catalog backend where:

- Products, pages, and shop metadata are defined in YAML files on disk and served via a JSON REST API.
- The catalog is read from the filesystem at startup; no database is involved in serving catalog reads.
- Customer orders submitted through `POST /orders` are persisted to a PostgreSQL database.
- To update catalog content, update the YAML files and restart the service.

## Key Concepts

### YAML-based catalog

All catalog data is loaded from a configurable `data_dir` at startup. The catalog is read-only at runtime.

### Validation at startup

Every product YAML is validated when the service starts. Invalid products are fatal — the service refuses to start
if any product file fails validation. Rules:

- `name` and `description` are required and must define the same set of languages.
- Every spec entry must be translated into all languages defined in `name`.
- `prices` must define at least a `default` key.
- Every attribute entry must be translated into all languages defined in `name`, and each language entry must have
  at least one value.
- All image paths (`preview` and `full`) must exist on disk relative to the product's `images/` subdirectory.

## Filesystem Layout

```
{data_dir}/
  shop.yaml                # shop metadata (name, title, description)
  products/
    products.yaml          # flat product listing (id, title, description)
    {product-id}/
      product.yaml         # optional: full product definition (validated at startup)
      images/
        01-preview.png
        01.png
  pages/
    pages.yaml             # page metadata listing
    {page-id}/
      en.md
      uk.md
```

`products/products.yaml` drives `GET /products`. Each product subdirectory is optional; directories without a
`product.yaml` are silently skipped by the validator. Image files are placed in the `images/` subdirectory and
referenced by filename in `product.yaml`.

## Data Entities

### Shop

Shop metadata is loaded at startup from `{data_dir}/shop.yaml`. It holds a single `shop:` key containing the
allowed-countries map and multilingual maps for `name`, `title`, and `description`.

`shop.yaml` is **required** at startup. The application fails to start if it is missing or if `countries` is empty.
The `countries` map is keyed by lowercase ISO alpha-2 country code; each entry carries a per-language `name`,
per-language `currency` symbol/code, an international `phone_code` (e.g. `"+380"`), and a `flag` glyph
(typically a Unicode flag emoji such as `"🇺🇦"`). The set of keys is the authoritative allow-list from which
orders may be created — any order whose `country` field is not a key of this map is rejected with HTTP 400
`invalid country`.

`GET /shop` returns the shop object as JSON, including the full `countries` map.

Example `shop.yaml`:

```yaml
shop:
  countries:
    ua:
      name:
        en: Ukraine
        uk: Україна
      currency:
        en: UAH
        uk: грн
      phone_code: "+380"
      flag: "🇺🇦"
    us:
      name:
        en: United States
        uk: США
      currency:
        en: USD
        uk: дол.
      phone_code: "+1"
      flag: "🇺🇸"
  name:
    en: My Shop
    uk: Мій магазин
  title:
    en: Handcrafted goods
    uk: Товари ручної роботи
  description:
    en: Designed and made by hand.
    uk: Спроєктовано та виготовлено вручну.
```

### Product listing

The product listing served by `GET /products` comes from `{data_dir}/products/products.yaml`. This is a flat YAML
file containing lightweight product entries: an `id`, a multilingual `title`, and a multilingual `description`. A
missing `products.yaml` results in an empty list — it is not an error.

Each entry in the listing response also includes an optional `image` field — the URL path of the first preview image
from the product's `product.yaml` (e.g. `/images/oak-shelf/thumb.png`). The field is omitted when the product has no
images or when the product subdirectory has no `product.yaml`.

Example `products.yaml`:

```yaml
products:
  - id: oak-shelf
    title:
      en: Oak Shelf
      uk: Дубова полиця
    description:
      en: A handcrafted solid oak wall shelf.
      uk: Настінна полиця з масиву дуба, виготовлена вручну.
```

### Product

Each product may have a subdirectory under `{data_dir}/products/` with a `product.yaml` file. If present, it is
validated at startup to enforce data integrity. Subdirectories without a `product.yaml` are silently skipped.

`GET /products/{id}/{lang}` reads this file, collapses all multilingual maps to the requested language, resolves the
price for the caller's country, and returns a JSON `ProductDetail` object. Returns 404 if the product or language is
not found.

Fields in `product.yaml`:

- **name** — multilingual product name (e.g. `en`, `uk`).
- **description** — multilingual long-form description; must cover the same languages as `name`.
- **specs** — optional map of specification keys, each translated into every language defined in `name`.
  Each spec entry has a `title` and `value`.
- **prices** — map of country/region codes (lowercase, e.g. `ua`, `us`) to `{currency, value}` pairs. Must contain
  a `default` key. The API resolves this map to a single `{currency, value}` object per request: the country is
  detected from the `CF-IPCountry` request header (sent by Cloudflare) or via an ipinfo.io lookup on the client IP.
  If no matching country key exists the `default` entry is used.
- **attrs** — optional map of attribute keys (e.g. `display_color`). Each attribute is translated into every language
  in `name`. Each language entry has a `title` and a `values` map with at least one entry; each value has a `title`.
- **attr_prices** — optional map of add-on prices per attribute value, keyed by country (same structure as `prices`).
  Shape: `attr_key → value_key → country_key → float64`. Must contain a `default` country key for each value.
  The API resolves this to `attr_key → value_key → float64` using the same country detection logic as `prices`.
- **attr_images** — optional map of per-attribute-value image filenames. Shape: `attr_key → value_key → filename`.
  Filenames are relative to the product's `images/` subdirectory. The API response returns them as URL paths
  (e.g. `/images/{id}/red-thumb.jpg`). No country resolution is applied — each value maps to exactly one image.
- **images** — optional list of `{preview, full}` filename pairs. Filenames are relative to the product's `images/`
  subdirectory. The API response returns these as URL paths (e.g. `/images/{id}/thumb.jpg`) that can be appended to
  the server's base URL to download the file.

Example `product.yaml`:

```yaml
name:
  en: Oak Shelf
  uk: Дубова полиця

description:
  en: A handcrafted solid oak wall shelf.
  uk: Настінна полиця з масиву дуба, виготовлена вручну.

specs:
  weight:
    en:
      title: Weight
      value: 1.2 kg
    uk:
      title: Вага
      value: 1.2 кг

prices:
  default:
    currency: EUR
    value: 80
  ua:
    currency: UAH
    value: 3200

attrs:
  finish:
    en:
      title: Finish
      values:
        natural:
          title: Natural
        dark:
          title: Dark
    uk:
      title: Покриття
      values:
        natural:
          title: Натуральне
        dark:
          title: Темне

attr_prices:
  finish:
    natural:
      default: 0
    dark:
      default: 10
      ua: 400

attr_images:
  finish:
    natural: finish-natural.jpg
    dark: finish-dark.jpg

images:
  - preview: 01-preview.png
    full: 01.png
```

### Page

Page metadata is defined in `{data_dir}/pages/pages.yaml` and loaded at startup. Each entry has an `id`
and a multilingual `title` map.

`GET /pages` returns a JSON array of page objects: `[{"id": "...", "title": {"en": "...", "uk": "..."}}]`.
A missing `pages.yaml` returns an empty array.

Per-language content lives in `{data_dir}/pages/{id}/{lang}.md`. `GET /pages/{id}/{lang}` returns the raw
Markdown content as `text/plain`. Returns 404 if the page ID or language file does not exist.

Example `pages.yaml`:

```yaml
pages:
  - id: about
    title:
      en: About
      uk: Про нас
```

## API Overview

The service exposes a JSON REST API validated against an OpenAPI specification.

| Method   | Path                               | Description                                           |
|----------|------------------------------------|-------------------------------------------------------|
| `GET`    | `/shop`                            | Get shop metadata (name, title, description)          |
| `GET`    | `/products`                        | List all products (id, title, description, image)     |
| `GET`    | `/products/{id}/{lang}`            | Get full product detail in the requested language     |
| `GET`    | `/images/{product_id}/{file_name}` | Download a product image by filename                  |
| `GET`    | `/pages`                           | List all pages (id, title)                            |
| `GET`    | `/pages/{id}/{lang}`               | Get page content (Markdown) in the requested language |
| `GET`    | `/nova-poshta/cities?q=<query>`    | Search Nova Poshta cities by name                     |
| `GET`    | `/nova-poshta/branches?city_ref=<ref>&q=<query>` | Search Nova Poshta branches in a city    |
| `GET`    | `/nova-poshta/streets?city_ref=<ref>&q=<query>`  | Search Nova Poshta streets in a city     |
| `POST`   | `/orders`                          | Submit a customer order (persisted to PostgreSQL)     |
| `GET`    | `/orders`                          | List persisted orders (requires API key)              |
| `GET`    | `/orders/{id}`                     | Get an order's status (public)                        |
| `POST`   | `/monobank/webhook`                | Receive Monobank invoice-status callbacks (ECDSA auth)|

Image paths returned in product responses (e.g. `/images/oak-shelf/thumb.jpg`) map directly to the image download
endpoint — prepend the server's base URL to get a complete download URL.

### Nova Poshta autocomplete

Three endpoints proxy search queries to the Nova Poshta JSON API v2 to support address-autocomplete widgets.

`GET /nova-poshta/cities?q=<query>` — searches settlements by name. Returns a JSON array of
`[{"ref": "<uuid>", "name": "<string>"}]` objects. The `q` parameter is required; omitting it returns 400.

`GET /nova-poshta/branches?city_ref=<ref>&q=<query>` — searches warehouses (branches) within a city identified
by `city_ref`. Returns a JSON array of `[{"ref": "<uuid>", "name": "<string>"}]` objects. Both `city_ref` and
`q` are required; omitting either returns 400.

`GET /nova-poshta/streets?city_ref=<ref>&q=<query>` — searches streets within a city identified by `city_ref`
(the settlement ref returned by the cities endpoint). Returns a JSON array of
`[{"ref": "<uuid>", "name": "<string>"}]` objects. Both `city_ref` and `q` are required; omitting either returns
400. Intended for courier (door-to-door) delivery address input.

All three endpoints return 502 if the Nova Poshta API call fails.

### Orders

`POST /orders` accepts a single-product order and persists it to PostgreSQL via a two-phase flow:

1. **Transaction 1** — inserts the order header (`orders`), one row per selected attribute (`order_attrs`), and one
   initial `order_history` row with `status='new'`, all inside a single database transaction. On commit the order
   exists in `status='new'`.
2. **Monobank invoice** — the handler calls the Monobank acquiring API to create a hosted-payment invoice. If this call
   fails, the order stays in `status='new'` and the client receives HTTP 502; the operator can re-issue an invoice
   manually.
3. **Transaction 2** — if the invoice is created successfully, a second transaction transitions the order to
   `status='awaiting_payment'` by appending a second `order_history` row. The response returns HTTP 201 with the
   Monobank `pageUrl` as `{"payment_url": "<url>"}`.

After a successful request there are always **two** `order_history` rows: one for `new` (written in tx1) and one for
`awaiting_payment` (written in tx2). HTTP 502 is returned if the DB write, the Monobank call, or the second DB
transaction fails.

Once Monobank delivers a webhook (`POST /monobank/webhook`, authenticated via ECDSA `X-Sign`), the event is appended
to the `invoice_history` table — keyed by `(invoice_id, provider, status, event_at)` so duplicate deliveries are a
silent no-op — and the order's payment status is **re-derived from the latest event in the timeline** (ordered by
the provider's own `modifiedDate`, not by our receive time). The order moves along the payment lifecycle
`awaiting_payment → payment_processing → payment_hold → paid`, with `failure`/`expired` mapping to `cancelled` and
`reversed` mapping to `refunded`. Because state is derived from the latest event by `modifiedDate`, **out-of-order
webhook delivery is tolerated**: a late-arriving event with an older timestamp inserts behind the latest and does not
move the order. **Retry-after-failure is also supported**: a customer who hits "Retry" on a cancelled order — Monobank
reuses the same `invoiceId` — drives the order out of `cancelled` back into `payment_processing` and onward, because
`cancelled` is treated as re-enterable by the lifecycle rule.

Unknown references and informational `created` events return HTTP 200 without any DB write. Transient DB errors return
500 so Monobank retries. Once an operator moves an order into a fulfillment state (`processing`, `shipped`, etc.),
later invoice events become informational and do not roll the order backwards — except for `reversed → refunded`,
which always wins because the customer's money was returned.

**Request body (JSON):**

```json
{
  "product_id": "oak-shelf",
  "lang": "uk",
  "attributes": {"finish": "dark"},
  "first_name": "Іван",
  "middle_name": "Іванович",
  "last_name": "Іваненко",
  "phone": "+380501234567",
  "email": "ivan@example.com",
  "country": "ua",
  "city": "Київ",
  "address": "Відділення №5, вул. Хрещатик, 1",
  "notes": "Зателефонуйте за годину"
}
```

Required fields: `product_id`, `lang`, `first_name`, `last_name`, `phone`, `email`, `country`, `city`, `address`.
Optional: `middle_name`, `attributes`, `notes` (stored in the `customer_note` column).

The supplied `country` must be a key of the `shop.countries` map defined in `shop.yaml`; otherwise the request
is rejected with HTTP 400 `invalid country`. The server resolves the price from `product.yaml` using the
request-supplied `country` against `prices.<country>` (falling back to `prices.default` when there is no matching
key) plus per-attribute add-on prices resolved the same way, converts each amount to integer minor units (cents),
renders the selected attributes into title pairs in the customer's language, and then executes the two-phase flow
described above. The `country` column receives the request-supplied value verbatim; geo detection is no longer
consulted at order time. Returns 201 on success, 400 for invalid input (including a missing or disallowed `country`),
404 for an unknown product, and 502 if the DB write, the Monobank call, or the second DB transaction fails.

#### Schema

`orders` columns: `id` (uuid v7, default), `product_id`, `status` (enum, default `new`), `email`, `price` (int,
minor units, total = base + sum of attr add-ons), `currency`, `first_name`, `middle_name` (nullable), `last_name`,
`country`, `city`, `phone`, `address`, `admin_note` (nullable), `customer_note` (nullable), `created_at`,
`updated_at`.

`order_attrs` columns: `order_id` (FK to `orders.id`), `attr_name`, `attr_value`, `attr_price` (int, minor units;
the per-attribute add-on amount). One row per selected attribute on the order.

`order_history` columns: `id` (uuid v7, default), `order_id` (FK to `orders.id`), `status` (`order_status`,
NOT NULL), `note` (nullable), `created_at`. A successfully created order has exactly two initial rows: one with
`status = 'new'` written in the first transaction (alongside `orders` and `order_attrs`), and one with
`status = 'awaiting_payment'` written in the second transaction after the Monobank invoice is confirmed. Further rows
are appended whenever a webhook event drives the order to a new payment state. `order_history` is the order-state
transition log; the verbatim webhook body lives on `invoice_history.payload` instead, where it serves as the audit
trail for "what the provider told us."

`invoice_history` columns: `id` (uuid v7, default), `order_id` (FK to `orders.id`), `invoice_id` (text — the
provider's own invoice id), `provider` (text, e.g. `monobank`), `status` (`invoice_status`, NOT NULL), `note`
(nullable; pre-rendered human summary like `monobank: success, finalAmount=3200`), `payload` (jsonb, NOT NULL;
verbatim provider webhook body), `event_at` (timestamp; populated from the provider's `modifiedDate`, not our
receive time), `created_at`. A `UNIQUE (invoice_id, provider, status, event_at)` constraint dedupes idempotent
webhook replays. The "current" invoice status for an order is the row with the largest `event_at` (with `created_at`
as a tiebreaker for the pathological case of two distinct rows sharing a `modifiedDate`).

`invoice_status` enum values: `processing`, `hold`, `paid`, `failed`, `reversed`. Monobank's webhook statuses map onto
these via `monobankStatusToInvoiceStatus` (`success → paid`, `failure | expired → failed`, `created → no row`).

`order_status` enum values: `new`, `awaiting_payment`, `payment_processing`, `payment_hold`, `paid`, `processing`,
`cancelled`, `shipped`, `delivered`, `refund_requested`, `returned`, `refunded`. The two `payment_*` values are
written when the latest invoice event is `processing` / `hold`; `paid` is the terminal success state, `cancelled`
covers `failed`, and `refunded` is set on `reversed`.

Migrations are embedded under `internal/sql/` and applied automatically at startup.

#### Listing orders

`GET /orders` returns every persisted order, newest first, with the order header, all selected attributes, and the
full status history inlined into each record. It is intended for operator/admin tooling and is protected by a bearer
token.

**Auth:** the request must carry an `Authorization: Bearer <key>` header whose value matches the configured
`server.api_key` from `config.yml`. A missing or malformed header returns HTTP 401 with
`{"error": "missing or invalid authorization header"}`; a well-formed header carrying the wrong key returns HTTP 401
with `{"error": "invalid api key"}`.

**Conditional registration:** the `GET /orders` route is registered only when `server.api_key` is non-empty in
config. When the key is empty (or absent) the route is not registered at all. Because `POST /orders` is registered
unconditionally on the same path, requests to `GET /orders` then receive HTTP 405 Method Not Allowed (the path
exists, just not for `GET`) rather than 404.

**Response:** a JSON array of order records. Each record carries the `orders` table fields (`id`, `status`,
`product_id`, `email`, `price` in minor units, `currency`, `first_name`, `middle_name` (optional), `last_name`,
`country`, `city`, `phone`, `address`, `admin_note` (optional), `customer_note` (optional), `created_at`,
`updated_at`) plus two inlined arrays: `attrs` — the rendered title pairs from `order_attrs` (one entry per selected
attribute, each `{name, value, price}`) — and `history` — the timeline from `order_history` (one entry per status
change, each `{id, status, note (optional), created_at}`). Optional text fields are omitted from JSON when NULL.
The verbatim provider payloads are not surfaced through this endpoint; they live on `invoice_history` for forensic
inspection via the database.

#### Order status

`GET /orders/{id}` returns `{"status": "<order_status>"}` for a single order identified by its UUIDv7. The endpoint is
public — no API key, no rate limit. The `id` value is the same UUID the storefront receives in the Monobank
`redirectUrl` query string after a customer completes (or cancels) payment, so the post-payment confirmation page can
poll the order's current state.

The path parameter is validated by the OpenAPI request middleware as `format: uuid`; a malformed value returns HTTP
400 with the standard validator body before the handler runs. A valid UUID with no matching row returns HTTP 404 with
`{"error": "order not found"}`. The `status` value is the raw `order_status` enum (`new`, `awaiting_payment`,
`payment_processing`, `payment_hold`, `paid`, `cancelled`, etc.) — clients are responsible for mapping it to a
user-facing message in their own language.

#### Monobank webhook

`POST /monobank/webhook` receives Monobank invoice-status deliveries. Authenticated via the `X-Sign` ECDSA signature
header verified against the merchant public key fetched from `/api/merchant/pubkey` at startup (refetched once on a
verification failure to recover from key rotation). The route does not pass through CORS, OpenAPI validation, or the
rate limiter.

Response codes: 200 for processed/idempotent/informational/unknown reference (Monobank stops retrying); 401 for an
invalid signature; 400 for malformed JSON; 500 for transient DB errors so Monobank retries. The handler never writes
a JSON error body — Monobank does not consume them.

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
  api_key: "<operator-api-key>"
  public_url: "https://shop.example"
  cors_origins:
    - "https://example.com"
data_dir: "./data"
database:
  dsn: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
nova_poshta:
  api_key: "<your-api-key>"
monobank:
  api_key: "<your monobank acquiring X-Token>"
  service_url: ""                                # empty → use https://api.monobank.ua/
  redirect_url: "https://shop.example/order/thanks"
rate_limit: 1
```

- `debug` — enable verbose logging (default: `false`).
- `server.addr` — address and port to listen on (default: `":9000"`).
- `server.api_key` — bearer token required to call `GET /orders`. Leave empty to disable the listing endpoint
  entirely; in that case `GET /orders` is not registered and responds with HTTP 405 Method Not Allowed (because
  `POST /orders` is registered on the same path).
- `server.cors_origins` — list of allowed CORS origins. Use `["*"]` to allow all origins.
- `server.public_url` — public HTTPS base URL of this service (e.g. `https://shop.example`). **Required** — empty
  value causes a startup error. Used to derive (a) the Monobank webhook URL, posted as `webHookUrl` on every
  `CreateInvoice` so each invoice records where to deliver status updates, fixed at
  `<public_url>/monobank/webhook`; and (b) the Monobank basket-item `icon`, pointing at
  `<public_url>/images/<product_id>/<preview>` when the product has at least one image with a non-empty `preview`,
  which the bank app renders next to the line on the payment screen. A trailing slash on `public_url` is trimmed.
- `data_dir` — root directory containing catalog files (default: `"./data"`).
- `database.dsn` — PostgreSQL DSN for the orders database. Required: the service runs the embedded migrations
  against this DSN at startup and refuses to start if the connection fails. PostgreSQL 18 or newer is required
  (the order id default uses the built-in `uuidv7()` function).
- `nova_poshta.api_key` — Nova Poshta API key. Required for the `/nova-poshta/*` endpoints to work.
- `nova_poshta.service_url` — override the Nova Poshta API base URL (default: `https://api.novaposhta.ua/v2.0/json/`).
  Leave unset in production; used in tests.
- `monobank.api_key` — Monobank acquiring X-Token. **Required** — the application refuses to start if empty.
- `monobank.redirect_url` — URL the Monobank payment page redirects to after the customer completes (or cancels)
  payment. **Required** — the application refuses to start if empty.
- `monobank.service_url` — override the Monobank API base URL (default: `https://api.monobank.ua/`). Leave unset in
  production; used in tests.
- `rate_limit` — requests per minute allowed for `POST /orders` per client IP (positive integer). `0` or a negative
  value disables rate limiting entirely.
- `telegram.token` — Telegram bot token obtained from `@BotFather`. Optional; see below.
- `telegram.chat_id` — Telegram chat or channel id (numeric like `-1001234567890`, or `@handle`). Optional; see below.

### Telegram notifications

Optional. When configured, every change to an order's `order_history` is mirrored as a MarkdownV2 message to a Telegram
chat. Useful for shop owners who want a real-time feed of order activity without standing up a separate dashboard.

```yaml
telegram:
  token: "<bot token from @BotFather>"
  chat_id: "<numeric channel id like -1001234567890, or @channel handle>"
```

Both keys must be set together; setting one without the other is a fatal startup error. With both empty the feature is
silently disabled (this is the default; existing deployments are unaffected by the upgrade).

**Message format.** New orders get the full detail; every other status change gets a slim notification. The order id is
rendered as inline code so it's tap-to-copy in the Telegram client.

A new order:

```
*New order* `0193c5fa-7b3a-7000-8000-0123456789ab`

*Product:* pro-plan-annual
*Display color:* Red
*Total:* 49.00 USD
*Customer:* Jane Doe
*Phone:* +1234567890
*Email:* jane@example.com
*Delivery:* UA, Kyiv, Some Street 5
*Customer note:* Please ship after Friday
```

Any subsequent status change (e.g. `awaiting_payment`, `paid`, `failed`):

```
Order `0193c5fa-7b3a-7000-8000-0123456789ab` — *paid*

monobank: success, finalAmount=4900
```

The trailing line ("status note") is the rendered note from the underlying invoice event and is omitted when empty.

**Failure handling.** The notifier is best-effort: a Telegram outage cannot break order placement or webhook processing.
Events are queued in an in-memory bounded buffer (256 entries) drained by a single background worker that retries
transient failures up to three times with 1s/2s backoff between attempts (or honors `retry_after` on 429). Permanent
errors (4xx other than 429) and buffer-full conditions log and drop the event. Events still in the buffer at process
exit are discarded after a 5s graceful drain.

To get a chat id for a private channel: add the bot to the channel as an administrator, then post once and inspect the
update via `getUpdates`.
