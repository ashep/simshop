# SimShop

SimShop is an HTTP API service that serves catalog data (products, pages, shop metadata) loaded from YAML files at
startup and persists customer orders to PostgreSQL. It ships a deployment image that fronts the API with Caddy and
serves a storefront theme supplied by the downstream image — no frontend is bundled (see [Frontend
theme](#frontend-theme) and [Deployment](#deployment)).

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
- `summary`, when present, must define the same set of languages as `name`.
- Every spec entry must be translated into all languages defined in `name`.
- `prices` must define at least a `default` key.
- Every attribute entry must be translated into all languages defined in `name`, and each language entry must have
  at least one value.
- All image paths (`preview` and `full`) must exist on disk relative to the product's `images/` subdirectory.
- Every `promo_codes` key must be non-empty without whitespace, unique case-insensitively, with a percent in `1..99`.

## Filesystem Layout

```
{data_dir}/
  shop.yaml                # shop metadata, incl. the ordered catalog `products` list
  products/
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

The `products` list in `shop.yaml` drives `GET /products`. Directories without a `product.yaml` are silently skipped
by the validator. Image files are placed in the `images/` subdirectory and
referenced by filename in `product.yaml`.

## Data Entities

### Shop

Shop metadata is loaded at startup from `{data_dir}/shop.yaml`. It holds a single `shop:` key containing the
allowed-countries map, multilingual maps for `name`, `title`, and `description`, an optional ordered list of
product `categories`, the ordered catalog `products` list, optional per-language `links`, an optional
`google-analytics` block, and optional storefront content (`announcement`, `menu`, `hero`, `features`, `story`,
`gallery`).

`shop.yaml` is **required** at startup. The application fails to start if it is missing or if `countries` is empty.
The `countries` map is keyed by lowercase ISO alpha-2 country code; each entry carries a per-language `name`,
per-language `currency` symbol/code, an international `phone_code` (e.g. `"+380"`), and a `flag` glyph
(typically a Unicode flag emoji such as `"🇺🇦"`). The set of keys is the authoritative allow-list from which
orders may be created — any order whose `country` field is not a key of this map is rejected with HTTP 400
`invalid country`.

The optional `categories` list is an ordered set of product categories. Each entry has a stable `id`, a per-language
title; in YAML the language keys sit flat alongside `id`, and the API nests them under a `title` object. `id` is
therefore reserved and cannot be used as a language code.
The bundled home page shows one product row per category, in list order (products in no category go to a trailing
"Other" row). A category that no product in the `products` list belongs to is dropped at startup, so the API (and the
storefront's home rows, filter tabs and footer links) never offers an empty category.

The optional `products` list names the product ids (directory names under `products/`) shown in the catalog, in
display order. A listed id with no loaded `product.yaml`, or one listed twice, is fatal at startup. Products not listed
are loaded and reachable via `GET /products/{id}/{lang}` but hidden from `GET /products`. The list is not part of the
`GET /shop` response.

The optional `links` list holds external/social links, shared by all languages; each entry has a display `title`, an
`icon` identifier, and a target `url`. The optional `google-analytics` block carries the
measurement `id` (e.g. `"G-XXXXXXXXXX"`); it is surfaced in the response under the `google_analytics` key.
The optional `cloudflare_analytics` block carries the Cloudflare Web Analytics `token`; it is surfaced under the
`cloudflare_analytics` key the same way. The theme loads the Cloudflare Web Analytics beacon with this token; the
numbers behind `GET /stats` come from that same site (see `cloudflare` in [Configuration](#configuration)).

The optional storefront content drives the home page and header of the theme; every block is optional and
the theme hides (or falls back for) whatever is missing:

- `announcement` — per-language text of the thin bar above the header.
- `announcement_page` — optional content page id (from `pages.yaml`); the whole announcement bar then links to it.
  An id with no matching page leaves the bar unlinked.
- `menu` — ordered list of content page ids (from `pages.yaml`) shown in the header navigation after the category
  links. Ids with no matching page are skipped.
- `hero` — the home-page lead block: per-language `eyebrow`, `title` and `text`, plus an optional `images` list
  shown as a slideshow under the same text (auto-advances every 6 s, also arrows, dots and swipe; pauses on hover, in
  a hidden tab and under reduced motion). Without them the frontend uses the shop `title`/`description` and the first
  product image. Each `images` entry is a bare filename or `{file, category}`; `category` (a `categories` id) is where
  a click on that slide leads (`/catalog?category=<id>`). No category, or one dropped for having no products, links to
  the whole catalog. The API always emits entries as `{file, category?}`.
- `features` — the benefits strip: a list of `{icon, title, text}` with per-language `title`/`text`. `icon` is one of
  the frontend's glyph names (`hand`, `clock`, `box`, `truck`, `mail`, `shield`, `heart`, `star`, `gift`, `leaf`);
  unknown names render without an icon.
- `story` — the "about us" band: per-language `eyebrow`, `title`, `text`, an optional `image`, and an optional `page`
  id the band links to.
- `gallery` — list of image filenames for the photo strip next to the Instagram link; hidden when empty.

Image fields (`hero.images[].file`, `story.image`, `gallery`) are **bare filenames under `{data_dir}/assets/`**, served by
`GET /assets/{name}`; they are not validated at startup.

`GET /shop` returns the shop object as JSON, including the full `countries` map, the `categories` list, the `links`
map, the `google_analytics` block, and the storefront content fields above.

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
  categories:
    - id: clocks
      en: Clocks
      uk: Годинники
    - id: organizers
      en: Organizers
      uk: Органайзери
  links:
    - title: Instagram
      icon: instagram
      url: https://www.instagram.com/example/
  google-analytics:
    id: G-XXXXXXXXXX
  announcement:
    en: Handmade · Free returns
    uk: Ручна робота · Безкоштовне повернення
  announcement_page: returns
  menu:
    - about
  hero:
    title:
      en: Objects made by hand
      uk: Речі, зроблені руками
    text:
      en: Shelves and organizers for your desk.
      uk: Полиці й органайзери для вашого столу.
    images:
      - file: hero-1.jpg
        category: organizers
      - hero-2.jpg
  features:
    - icon: clock
      title:
        en: Made in 3 days
        uk: Виготовлення за 3 дні
  story:
    title:
      en: Not mass-produced
      uk: Не серійний товар
    page: about
  gallery:
    - gallery-1.jpg
    - gallery-2.jpg
```

### Product listing

`GET /products` returns one entry per id in `shop.yaml`'s `products` list, in that order, built from each product's
`product.yaml`: `title` is the product's `name`, `description` is its short `summary` (omitted when absent), and
`categories` is its `categories` list. Without a `products` list the catalog is empty — it is not an error.

Each entry in the listing response also includes an optional `image` field — the URL path of the product's `cover`,
or else of the first still preview image (e.g. `/images/oak-shelf/thumb.png`). The field is omitted when the product
has no images. Alongside it, `images` lists up to 2 still preview paths starting with `image` (cover first, then the
gallery's stills in order, videos skipped); the storefront card crossfades to the second one on hover.

Each entry also carries a `price` (`{currency, value}`): the base price resolved for the client's country exactly
like `GET /products/{id}/{lang}` (falling back to `prices.default`). Geo detection only runs when some product defines
a country-specific price.

Example `shop.yaml` fragment:

```yaml
shop:
  products:
    - oak-shelf
    - walnut-box
```

### Product

Each product may have a subdirectory under `{data_dir}/products/` with a `product.yaml` file. If present, it is
validated at startup to enforce data integrity. Subdirectories without a `product.yaml` are silently skipped.

`GET /products/{id}/{lang}` reads this file, collapses all multilingual maps to the requested language, resolves the
price for the caller's country, and returns a JSON `ProductDetail` object. Returns 404 if the product or language is
not found.

The detail response also includes the product's `categories` from `product.yaml`; the field is omitted when empty.

Fields in `product.yaml`:

- **name** — multilingual product name (e.g. `en`, `uk`).
- **summary** — optional multilingual short blurb shown on catalog cards (the listing's `description`); must cover
  the same languages as `name` when present.
- **description** — multilingual long-form description; must cover the same languages as `name`.
- **categories** — optional list of category ids referencing the `categories` list in `shop.yaml`.
- **specs** — optional map of specification keys, each translated into every language defined in `name`.
  Each spec entry has a `title` and `value`.
- **prices** — map of country/region codes (lowercase, e.g. `ua`, `us`) to `{currency, value}` pairs. Must contain
  a `default` key. The API resolves this map to a single `{currency, value}` object per request: the country is
  detected from the `CF-IPCountry` request header (sent by Cloudflare) or via an ipinfo.io lookup on the client IP.
  If no matching country key exists the `default` entry is used.
- **attrs** — optional map of attribute keys (e.g. `display_color`). Each attribute is translated into every language
  in `name`. Each language entry has a `title` and a `values` map with at least one entry; each value has a `title`
  and an optional `prefix` (short symbol or emoji shown before the title in UIs). The API response always includes
  `prefix` alongside `title`, emitting an empty string when not set in YAML.
- **attr_prices** — optional map of add-on prices per attribute value, keyed by country (same structure as `prices`).
  Shape: `attr_key → value_key → country_key → float64`. Must contain a `default` country key for each value.
  The API resolves this to `attr_key → value_key → float64` using the same country detection logic as `prices`.
- **attr_images** — optional map of per-attribute-value image filenames. Shape: `attr_key → value_key → filename`.
  Filenames are relative to the product's `images/` subdirectory. The API response returns them as URL paths
  (e.g. `/images/{id}/red-thumb.jpg`). No country resolution is applied — each value maps to exactly one image.
- **attr_colors** — optional map of per-attribute-value swatch colors. Shape: `attr_key → value_key → "#rrggbb"`
  (validated at startup). Language-independent, returned as-is. When every value of an attribute has a color, the
  storefront shows that attribute as round color swatches instead of text buttons; a partial set falls back to text.
- **promo_codes** — optional map of promo code → integer discount percent (`1..99`), e.g. `SPRING15: 15`. Codes
  work only for this product, match case-insensitively, and stay valid until removed from the YAML (no expiry).
  Each phone number can use a code once (see *Promo codes* under `POST /orders`). Never returned by product
  endpoints; checked via `GET /products/{id}/promo-codes/{code}`.
- **cover** — optional filename of a still image under the product's `images/` subdirectory (usually one of the
  gallery previews, e.g. `03-preview.png`). It becomes the listing `image` — the picture on product cards and the
  fallback for the home hero — instead of the first gallery image. For link previews (OG tags) the
  matching gallery entry's `full` file is used when the cover is a gallery preview. Validated at startup: the file must
  exist and must not be a video. The product page gallery order is unaffected.
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
          prefix: "🌿"
          title: Natural
        dark:
          title: Dark
    uk:
      title: Покриття
      values:
        natural:
          prefix: "🌿"
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

attr_colors:
  finish:
    natural: "#d8b98c"
    dark: "#5a3b24"

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
| `GET`    | `/shop`                            | Get shop metadata (name, title, description, categories, links, analytics) |
| `GET`    | `/products`                        | List all products (id, categories, title, description, image) |
| `GET`    | `/products/{id}/{lang}`            | Get full product detail in the requested language     |
| `GET`    | `/products/{id}/promo-codes/{code}` | Check a product promo code (percent off, or 404)     |
| `GET`    | `/images/{product_id}/{file_name}` | Download a product image by filename                  |
| `GET`    | `/assets/{path...}`                | Download a static asset (favicon, logo, etc.) by path |
| `GET`    | `/pages`                           | List all pages (id, title)                            |
| `GET`    | `/pages/{id}/{lang}`               | Get page content (Markdown) in the requested language |
| `GET`    | `/nova-poshta/cities?q=<query>`    | Search Nova Poshta cities by name                     |
| `GET`    | `/nova-poshta/branches?city_ref=<ref>&q=<query>` | Search Nova Poshta branches in a city    |
| `GET`    | `/nova-poshta/streets?city_ref=<ref>&q=<query>`  | Search Nova Poshta streets in a city     |
| `POST`   | `/orders`                          | Submit a customer order (persisted to PostgreSQL)     |
| `GET`    | `/orders`                          | List persisted orders (requires API key)              |
| `GET`    | `/orders/{id}`                     | Get an order's status (public)                        |
| `PATCH`  | `/orders/{id}/status`              | Operator-driven status transition (requires API key)  |
| `GET`    | `/stats?days=7\|14\|30`            | Daily visits and most viewed products (requires API key and `cloudflare` config) |
| `POST`   | `/monobank/webhook`                | Receive Monobank invoice-status callbacks (ECDSA auth)|

`GET /stats` returns `{days: [{date, visits}], top_products: [{id, title, views}]}` for the last 7 (default), 14 or 30
UTC days including today, oldest first and zero-filled; up to 10 products, most viewed first, titled in English,
else the alphabetically first available language (empty for ids no longer in the catalog). *Visits* count only
pageloads arriving from another site; *views* count every load of a product page, so internal navigation raises
views but not visits. Data comes from Cloudflare Web Analytics and is cached for 10 minutes. The route exists only when
`server.api_key` and all three `cloudflare.*` settings are set; otherwise it responds 404. A Cloudflare failure
yields 502.

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
Optional: `middle_name`, `attributes`, `notes` (stored in the `customer_note` column), `promo_code`.

**Promo codes.** When `promo_code` is non-empty it must be one of the product's `promo_codes` (case-insensitive),
otherwise the request is rejected with HTTP 400 `invalid promo code` before anything is written — an order is never
silently placed at full price. The percent is taken off the whole subtotal (base price plus add-ons) and the discounted
total is rounded to whole currency units, half up (no kopecks): `total = round(subtotal × (100 − percent) / 100)`,
`discount = subtotal − total` (e.g. 2499 at 15% → 2124, discount 375). The Monobank invoice is for that total. The
storefront previews the discount via `GET /products/{id}/promo-codes/{code}`, which returns `{"code": "SPRING15",
"discount_percent": 15}` (canonical spelling) or 404 `promo code not found` for an unknown product or code. That
endpoint is per-IP rate-limited like `POST /orders` (separate limiter, same `rate_limit`).

A phone number can use a given code only once: if any non-`cancelled` order already carries the same code
(case-insensitive) and the same phone digits (formatting ignored), the request is rejected with HTTP 400 `promo code
already used` and nothing is written. Orders of any status count, including unpaid `new`/`awaiting_payment` ones; a
failed or expired payment moves the order to `cancelled`, which frees the code for a retry. The check runs inside the
order-insert transaction under an advisory lock, so two concurrent requests can't both use the code. The storefront
shows this error next to the promo field.

The applied code, percent and
discount show up in `GET /orders`, the Telegram new-order message, customer emails and `simshop order get`.

The supplied `country` must be a key of the `shop.countries` map defined in `shop.yaml`; otherwise the request
is rejected with HTTP 400 `invalid country`. The server resolves the price from `product.yaml` using the
request-supplied `country` against `prices.<country>` (falling back to `prices.default` when there is no matching
key) plus per-attribute add-on prices resolved the same way, converts each amount to integer minor units (cents),
renders the selected attributes into title pairs in the customer's language, and then executes the two-phase flow
described above. The `country` column receives the request-supplied value verbatim; geo detection is no longer
consulted at order time. Returns 201 on success, 400 for invalid input (including a missing or disallowed `country`),
404 for an unknown product, and 502 if the DB write, the Monobank call, or the second DB transaction fails.

#### Schema

`orders` columns: `id` (uuid v7, default), `product_id`, `status` (enum, default `new`), `email`, `price` (int, minor
units, charged total = base + sum of attr add-ons − discount), `promo_code`, `discount_percent`, `discount` (all
nullable — set together when a promo code was applied; `discount` in minor units), `currency`, `lang` (checkout language
code captured from the request, drives downstream localized notifications), `first_name`, `middle_name` (nullable),
`last_name`, `country`, `city`, `phone`, `address`, `admin_note` (nullable), `customer_note` (nullable),
`tracking_number` (nullable; populated by the operator on the `shipped` transition, write-once), `created_at`,
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
`server.api_key` from `config.yaml`. A missing or malformed header returns HTTP 401 with
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

**Filtering:** the optional `?status=<csv>` query parameter narrows the result set to orders whose current status
matches one of the listed `order_status` values (comma-separated, e.g. `?status=paid,shipped`). Omitting the
parameter returns every order (filter disabled). A literal `?status=` (empty value) is rejected with HTTP 400 by the
OpenAPI validator before the handler runs; unknown status values are likewise rejected with 400.

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

#### Operator status update

`PATCH /orders/{id}/status` drives an order through the operator-owned fulfillment lifecycle. Authenticated by the same
`Authorization: Bearer <key>` header as `GET /orders` (`server.api_key`); a missing/invalid key returns HTTP 401.

**Request body (JSON):**

```json
{
  "status": "shipped",
  "note": "Sent via Nova Poshta",
  "tracking_number": "20451234567890"
}
```

- `status` — required; the target `order_status`. Must be one of the operator-allowed transitions below.
- `note` — optional; an operator-supplied free-text note appended to `order_history.note` and forwarded to the
  customer email and Telegram notification. Max 500 characters.
- `tracking_number` — **required iff** `status == "shipped"`, **forbidden** for any other target. Max 64 characters.
  Persisted on `orders.tracking_number` write-once: once set, subsequent transitions cannot overwrite or clear it.

**Allowed transitions:**

| From               | Allowed targets                                              |
|--------------------|--------------------------------------------------------------|
| `paid`             | `processing`, `refund_requested`, `refunded`                 |
| `processing`       | `shipped`, `refund_requested`, `refunded`                    |
| `shipped`          | `delivered`, `refund_requested`                              |
| `delivered`        | `refund_requested`                                           |
| `refund_requested` | `processing`, `shipped`, `delivered`, `returned`, `refunded` |
| `returned`         | `refunded`                                                   |

In addition to the matrix above, an operator may set `status: "cancelled"` from any state except `cancelled` and
`refunded` — this override exists so an order can be aborted anywhere in the lifecycle (including the webhook-owned
pre-paid cluster). `refunded` is fully terminal: the money is already back with the customer, so no operator
transition, not even `cancelled`, may follow it.

Any transition outside this matrix returns HTTP 409 with `{"error": "transition not allowed"}`. Submitting the order's
current status is treated as a no-op idempotent success (HTTP 200, no extra `order_history` row, no notification).

**Responses:**

- `200 {"status": "<new>"}` — the transition was applied (or was a no-op idempotent repeat).
- `400` — malformed body, missing/unknown `status`, missing `tracking_number` on `shipped`, `tracking_number` on a
  non-`shipped` target, or any field exceeding its length cap.
- `401` — missing/invalid bearer token.
- `404` — order id not found.
- `409` — current status disallows the requested target.
- `500` — DB error.

The `tracking_number` is **not** exposed by the public `GET /orders/{id}` endpoint (which returns only `status`); it
surfaces through the customer email (`shipped` template) and the Telegram operator feed.

#### Monobank webhook

`POST /monobank/webhook` receives Monobank invoice-status deliveries. Authenticated via the `X-Sign` ECDSA signature
header verified against the merchant public key fetched from `/api/merchant/pubkey` at startup (refetched once on a
verification failure to recover from key rotation). The route does not pass through CORS, OpenAPI validation, or the
rate limiter.

Response codes: 200 for processed/idempotent/informational/unknown reference (Monobank stops retrying); 401 for an
invalid signature; 400 for malformed JSON; 500 for transient DB errors so Monobank retries. The handler never writes
a JSON error body — Monobank does not consume them.

## Configuration

Config is loaded from `config.yaml`:

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
- `cloudflare.api_token`, `cloudflare.account_id`, `cloudflare.site_tag` — Cloudflare API token with Account Analytics
  read access, the account tag, and the Web Analytics site tag. Optional, but all three must be set together (a partial
  set is a startup error); setting them enables `GET /stats`. `cloudflare.service_url` overrides the API base URL
  (tests only). Env form: `SIMSHOP_CLOUDFLARE_APITOKEN`, `SIMSHOP_CLOUDFLARE_ACCOUNTID`, `SIMSHOP_CLOUDFLARE_SITETAG`.
  Operator setup: enable Web Analytics for the domain in Cloudflare; **turn off its automatic beacon injection** (the
  theme injects the beacon itself, so auto-injection double-counts); put the beacon token in `shop.yaml`
  `cloudflare_analytics.token`; create an API token with Account Analytics: Read for `api_token`. Whether the beacon
  token equals `site_tag` is unverified — if stats come back empty, check both values in the Cloudflare dashboard.
- `telegram.token` — Telegram bot token obtained from `@BotFather`. Optional; see below.
- `telegram.chat_id` — Telegram chat or channel id (numeric like `-1001234567890`, or `@handle`). Optional; see below.
- `resend.api_key` — Resend API key. Optional; empty disables customer email notifications.
- `mail.from` — sender address, e.g. `"Shop <orders@shop.example>"`. Required when `resend.api_key` is set.
- `mail.order_url` — customer-facing URL with `{id}` placeholder. Required when `resend.api_key` is set.

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

### Customer email notifications (Resend)

Optional. When configured, the service sends a transactional email to the customer's address whenever their order
transitions to `paid`, `processing`, `shipped`, `delivered`, `refund_requested`, or `refunded`. Useful for confirming
payment, signalling that the order is being prepared, tracking shipping, and acknowledging refund flows without
standing up a separate notification system.

```yaml
resend:
  api_key: "<resend api key>"
mail:
  from: "Shop Orders <orders@shop.example>"
  order_url: "https://shop.example/order?id={id}"
```

- `resend.api_key` — Resend API key (`re_...`). Empty disables the feature entirely.
- `mail.from` — sender address as it appears in the customer's inbox. Required when `resend.api_key` is set.
- `mail.order_url` — customer-facing URL pattern with a literal `{id}` placeholder, substituted with the order id at
  send time. Required when `resend.api_key` is set.

**Templates.** Email content lives at `{data_dir}/emails/{status}/{lang}.md`. Each file is a Markdown body preceded
by a YAML frontmatter block declaring the subject:

````markdown
---
subject: Order {{ .OrderShortID }} has been paid
---

Hi {{ .CustomerName }},

Thank you for your order **{{ .ProductTitle }}** ({{ .Total }}).

Track it here: <{{ .OrderURL }}>
````

Available template variables: `.OrderID` (full UUID), `.OrderShortID` (first 13 chars — UUIDv7 timestamp prefix),
`.CustomerName`, `.ProductTitle` (rendered title in the customer's language), `.Attrs` (slice of `{Name, Value}`),
`.Total` (formatted, e.g. `"49.99 USD"`), `.StatusNote` (operator note, e.g. tracking number for `shipped`),
`.ShopName` (in customer's language), `.OrderURL` (resolved from `mail.order_url`).

**Languages.** The customer's checkout language is persisted on the order and used to look up
`emails/{status}/{lang}.md`. If a language file is missing, the notifier falls back to `emails/{status}/en.md`.

**Required at startup.** When `resend.api_key` is set, `emails/{status}/en.md` must exist for every notify-on
status (`paid`, `processing`, `shipped`, `delivered`, `refund_requested`, `refunded`). The service refuses to start
otherwise.

**Failure handling.** Best-effort, like the Telegram notifier: events queue in a 256-event in-memory buffer drained
by a single background worker. Transient failures (5xx, 429) retry up to three times with 1s/2s backoff (or honor
`Retry-After` clamped to `[1s, 30s]`). Permanent errors (4xx other than 429) and buffer-full conditions log and
drop the event. In-flight events are discarded after a 5s graceful drain at process exit.

## CLI

`simshop` is a command-line client for operators, talking to a running backend over its HTTP API. It is a separate
binary from the server.

**Install:**

```bash
go install github.com/ashep/simshop/cmd/simshop@latest
```

**Configuration.** The CLI reads `~/.simshop.yaml` (or `~/.simshop.yml`), a mapping of shop name to connection
settings:

```yaml
my-shop-1:
  url: https://api.shop1.example
  api_key: foobar
  default: true
my-shop-2:
  url: https://api.shop2.example
  api_key: barbaz
```

- `url` — base URL of that shop's backend. **Required.**
- `api_key` — operator bearer token (matches the backend's `server.api_key`). **Required.**
- `default` — optional. Marks the shop used when `--shop` is omitted. At most one shop may set it. When no shop sets
  `default`, the **first shop in file order** is the default.

Override the config location with `--config <path>`.

**Commands:**

```
simshop [--shop NAME] [--config PATH]          [tui]
simshop [--shop NAME] [--config PATH] [--json] order list   [--status a,b|all]
                                                 order get    <id>
                                                 order set-status <status> <id> [<id>...] [--tracking N] [--note TEXT]
                                                 shops
```

- `tui` (or bare `simshop`) — full-screen interactive order browser in the spirit of k9s. Shows the same active
  orders as `order list` (newest first, color-coded by status, auto-refreshed every 30 s) with per-status counters.
  Keys: `↑↓`/`jk` move, `enter` opens the order card (customer, delivery, items, notes, invoices, history; `J`/`K`
  step to the next/previous order), `/` searches by name, phone, city, id, tracking or promo code, `a` toggles
  active/all, `s` changes the status — only transitions the backend allows are offered; `shipped` asks for the
  tracking number, every change asks for an optional note — `y` copies the order id, `r` refreshes, `q` quits.
  `tab` switches to the visit dashboard (needs the Cloudflare setup behind `GET /stats`): a bar chart of daily
  visits with total and daily average, and a table of the most viewed products. On it `p` cycles the period
  7 → 14 → 30 days, `r` refreshes, `tab` returns to the orders, `q` quits.
- `order list` — list orders, newest first. By default only active orders are shown, hiding the closed statuses
  `delivered`, `cancelled`, and `refunded`; `returned` is always listed since the refund is still pending.
  `--status paid,shipped` filters to the given statuses; `--status all` shows every order including closed ones.
- `order get <id>` — show one order's full record (attributes, history, invoices).
- `order set-status <status> <id> [<id>...]` — change the status of one or more orders. The status comes first,
  followed by any number of order ids, e.g. `order set-status cancelled 019e9de8-c3c0 019e9dd7-0740`. `status` must be
  one of `processing`, `shipped`, `delivered`, `cancelled`, `refund_requested`, `returned`, `refunded`. `--tracking`
  is required by the backend when setting `shipped`; `--note` records an optional note in the order history; both
  apply to every id. Each id is handled independently — a failure on one is reported and the rest still run, and the
  command exits non-zero if any failed.

`order get` and `order set-status` accept either a full order UUID or a **short id** — any unique prefix of it, such
as the abbreviated id shown by `order list`. An ambiguous prefix (matching more than one order) is rejected with the
list of candidates.
- `shops` — list the shops configured in `~/.simshop.yaml`, marking the default. API keys are never printed.

**Global flags:**

- `--shop NAME` — select a shop from the config (default: the configured default shop).
- `--config PATH` — config file path (default: `~/.simshop.yaml`).
- `--json` — emit raw JSON instead of a table, for scripting and piping.

## Frontend theme

The storefront is **not part of this repository**. The image serves static files from `/frontend`, which a downstream
image fills in (`FROM ashep/simshop` + `COPY <theme> /frontend`); the reference theme is
a separate theme repository. Without a theme the image serves only the API.

Contract a theme must follow:

- **Required pages.** `index.html` (fallback for every unmatched path — home, catalog, content pages, client-side
  routing), `product.html` (served at `/product/<id>` and `/<lang>/product/<id>` to humans; bots get the backend's
  `GET /product/{id}` / `GET /product/{lang}/{id}` preview), and `order-status.html` (served at `/order/status` and
  `/<lang>/order/status`, the target of `SIMSHOP_MONOBANK_REDIRECTURL` and `SIMSHOP_MAIL_ORDERURL`).
- **URL scheme.** Pages live at path-based, language-prefixed URLs: `/<lang>/`, `/<lang>/catalog/<category>`,
  `/<lang>/product/<id>`, `/<lang>/order/status` (`<lang>` is two lowercase letters; the unprefixed forms also work).
  Legacy query-string URLs (`/product?id=&lang=`, `/product.html?id=`, `/catalog?category=`, `/?lang=`) are redirected
  with a permanent 301 to the path form. Because the same page is served at varying depths, the theme's HTML must
  reference assets with **absolute paths** (`/main.js`, not `main.js`).
- **Analytics.** Loading the Cloudflare Web Analytics beacon (using `cloudflare_analytics.token` from `GET /shop`) is
  the theme's job; the backend only reads the collected data for `GET /stats`.
- **API base.** The API is reachable at `<origin>/api` (prefix stripped before the Go server); brand files at
  `<origin>/api/assets/<file>`.
- **Caching.** Images and video (`jpg jpeg png gif webp svg ico mp4`) get `max-age=86400`; `.js`/`.css` `no-cache`;
  `.html` `no-store`. Everything else is served as-is.

Routing is owned by `Caddyfile`; a theme cannot add routes.

For local development `task dev` serves the `frontend` symlink at the repo root (gitignored), e.g.
`ln -s ../shop-content/frontend frontend`.

## Deployment

The published artifact is a single Docker image (`Dockerfile`) built on the Caddy base image:

- The compiled Go server binary (`app.out`) is copied to `/app/app`; no frontend is copied — Caddy's web root is
  `/frontend`, filled by the downstream image (see [Frontend theme](#frontend-theme)).
- `entrypoint.sh` starts both the Go server (listening on `:9000`) and Caddy, exiting when either child exits.
- `Caddyfile` routes requests: `/api*` is reverse-proxied to the Go server with the `/api` prefix stripped; legacy
  query-string URLs are 301-redirected to path URLs; `/product/<id>` and `/<lang>/product/<id>` are served by the
  backend's social-crawler preview (`GET /product/{id}`, `GET /product/{lang}/{id}`, `ServeProductPreview`) for bots
  and rewritten to `product.html` for humans (distinguished by a User-Agent regex); `/order/status` (optionally
  language-prefixed) is rewritten to `order-status.html`;
  images, JS, CSS, and HTML are served from `/frontend` with per-type cache headers; every other path falls back to
  `index.html` for client-side routing.
