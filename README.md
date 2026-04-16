# SimShop

SimShop is a read-only HTTP API service that serves catalog data (products, pages, shop metadata) loaded from YAML
files at startup.

## Purpose

SimShop is designed as a lightweight catalog backend where:

- Products, pages, and shop metadata are defined in YAML files on disk and served via a JSON REST API.
- No database is required — content is read from the filesystem at startup.
- To update content, update the YAML files and restart the service.

## Key Concepts

### YAML-based catalog

All catalog data is loaded from a configurable `data_dir` at startup. The catalog is read-only at runtime.

### Validation at startup

Every product YAML is validated when the service starts. Invalid products are fatal — the service refuses to start
if any product file fails validation. Rules:

- `name` and `description` are required and must define the same set of languages.
- Every spec entry must be translated into all languages defined in `name`.
- `price` must define at least a `default` key.
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

Shop metadata is loaded at startup from `{data_dir}/shop.yaml`. It holds a single `shop:` key containing
multilingual maps for `name`, `title`, and `description`.

`GET /shop` returns the shop object as JSON. A missing `shop.yaml` returns an empty JSON object `{}`.

Example `shop.yaml`:

```yaml
shop:
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
- **price** — map of country/region codes (lowercase, e.g. `ua`, `us`) to `{currency, value}` pairs. Must contain
  a `default` key. The API resolves this map to a single `{currency, value}` object per request: the country is
  detected from the `CF-IPCountry` request header (sent by Cloudflare) or via an ipinfo.io lookup on the client IP.
  If no matching country key exists the `default` entry is used.
- **attrs** — optional map of attribute keys (e.g. `display_color`). Each attribute is translated into every language
  in `name`. Each language entry has a `title` and a `values` map with at least one entry; each value has a `title`.
- **attr_prices** — optional map of add-on prices per attribute value, keyed by country (same structure as `price`).
  Shape: `attr_key → value_key → country_key → float64`. Must contain a `default` country key for each value.
  The API resolves this to `attr_key → value_key → float64` using the same country detection logic as `price`.
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

price:
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
| `POST`   | `/orders`                          | Submit a customer order (appended to Google Sheets)   |

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

`POST /orders` accepts a single-product order and appends it as a row to a Google Sheet via a service account.
No data is stored on the service side — Google Sheets is the sole persistence layer.

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
  "city": "Київ",
  "address": "Відділення №5, вул. Хрещатик, 1",
  "notes": "Зателефонуйте за годину"
}
```

Required fields: `product_id`, `lang`, `first_name`, `last_name`, `phone`, `email`, `city`, `address`. Optional:
`middle_name`, `attributes`, `notes`.

The server resolves the product name and price from `product.yaml` (including geo-based pricing and per-attribute
add-on prices), formats the attributes string, and stamps the server-side datetime. Returns 201 on success, 400
for invalid input, 404 for an unknown product, and 502 if the Google Sheets API call fails.

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
  cors_origins:
    - "https://example.com"
data_dir: "./data"
nova_poshta:
  api_key: "<your-api-key>"
```

- `debug` — enable verbose logging (default: `false`).
- `server.addr` — address and port to listen on (default: `":9000"`).
- `server.cors_origins` — list of allowed CORS origins. Use `["*"]` to allow all origins.
- `data_dir` — root directory containing catalog files (default: `"./data"`).
- `nova_poshta.api_key` — Nova Poshta API key. Required for the `/nova-poshta/*` endpoints to work.
- `nova_poshta.service_url` — override the Nova Poshta API base URL (default: `https://api.novaposhta.ua/v2.0/json/`).
  Leave unset in production; used in tests.
- `google_sheets.credentials_json` — full service account JSON key (inline). The sheet must be shared with the
  service account's email. Required for `POST /orders` to work; if empty, orders return 502.
- `google_sheets.spreadsheet_id` — Google Sheets spreadsheet ID (from the sheet URL).
- `google_sheets.sheet_name` — name of the target sheet/tab (defaults to `Sheet1` if unset).
- `google_sheets.service_url` — override the Sheets API base URL. Leave unset in production; used in tests.

### Setting up Google Sheets credentials

1. Open [Google Cloud Console](https://console.cloud.google.com/) and select or create a project.
2. Navigate to **APIs & Services → Library** and enable the **Google Sheets API**.
3. Navigate to **APIs & Services → Credentials**, click **Create credentials → Service account**, fill in a name,
   and click **Done**.
4. Click the newly created service account, open the **Keys** tab, click **Add key → Create new key**, choose
   **JSON**, and download the file.
5. Open the downloaded JSON file, copy its entire contents, and paste them as the value of
   `google_sheets.credentials_json` in `config.yml`. Because the JSON contains newlines, use a YAML block scalar:

   ```yaml
   google_sheets:
     credentials_json: |
       {
         "type": "service_account",
         "project_id": "...",
         ...
       }
     spreadsheet_id: "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgVE2upms"
     sheet_name: "Orders"
   ```

6. In Google Sheets, open the target spreadsheet, click **Share**, and share it with the service account's email
   address (shown in the JSON as `client_email`) with **Editor** access.
