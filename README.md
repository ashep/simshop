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

`POST /orders` accepts a single-product order and inserts it as a row into the `orders` table in PostgreSQL. The
order id, status (`new`), and timestamps are populated by database defaults.

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
renders the selected attributes into title pairs in the customer's language, and inserts the order header, one row
per selected attribute, and one initial `order_history` row inside a single transaction. The `country` column
receives the request-supplied value verbatim; geo detection is no longer consulted at order time. Returns 201 on
success, 400 for invalid input (including a missing or disallowed `country`), 404 for an unknown product, and 502
if the database write fails.

#### Schema

`orders` columns: `id` (uuid v7, default), `product_id`, `status` (enum, default `new`), `email`, `price` (int,
minor units, total = base + sum of attr add-ons), `currency`, `first_name`, `middle_name` (nullable), `last_name`,
`country`, `city`, `phone`, `address`, `admin_note` (nullable), `customer_note` (nullable), `created_at`,
`updated_at`.

`order_attrs` columns: `order_id` (FK to `orders.id`), `attr_name`, `attr_value`, `attr_price` (int, minor units;
the per-attribute add-on amount). One row per selected attribute on the order.

`order_history` columns: `id` (uuid v7, default), `order_id` (FK to `orders.id`), `status` (`order_status`,
NOT NULL), `note` (nullable), `created_at`. Each newly created order writes one initial row here with
`status = 'new'` and `note` NULL inside the same transaction as the `orders` and `order_attrs` inserts, so an
order always has at least one history entry. Future status changes will append further rows carrying the new
status.

`order_status` enum values: `new`, `paid`, `payment_failed`, `processing`, `cancelled`, `shipped`, `delivered`,
`refund_requested`, `returned`, `refunded`.

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

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
  api_key: "<operator-api-key>"
  cors_origins:
    - "https://example.com"
data_dir: "./data"
database:
  dsn: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
nova_poshta:
  api_key: "<your-api-key>"
rate_limit: 1
```

- `debug` — enable verbose logging (default: `false`).
- `server.addr` — address and port to listen on (default: `":9000"`).
- `server.api_key` — bearer token required to call `GET /orders`. Leave empty to disable the listing endpoint
  entirely; in that case `GET /orders` is not registered and responds with HTTP 405 Method Not Allowed (because
  `POST /orders` is registered on the same path).
- `server.cors_origins` — list of allowed CORS origins. Use `["*"]` to allow all origins.
- `data_dir` — root directory containing catalog files (default: `"./data"`).
- `database.dsn` — PostgreSQL DSN for the orders database. Required: the service runs the embedded migrations
  against this DSN at startup and refuses to start if the connection fails. PostgreSQL 18 or newer is required
  (the order id default uses the built-in `uuidv7()` function).
- `nova_poshta.api_key` — Nova Poshta API key. Required for the `/nova-poshta/*` endpoints to work.
- `nova_poshta.service_url` — override the Nova Poshta API base URL (default: `https://api.novaposhta.ua/v2.0/json/`).
  Leave unset in production; used in tests.
- `rate_limit` — requests per minute allowed for `POST /orders` per client IP. `0` defaults to 1 RPM; a negative
  value disables rate limiting.
