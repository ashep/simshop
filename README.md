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

The service exposes a read-only JSON REST API validated against an OpenAPI specification.

| Method | Path                               | Description                                           |
|--------|------------------------------------|-------------------------------------------------------|
| `GET`  | `/shop`                            | Get shop metadata (name, title, description)          |
| `GET`  | `/products`                        | List all products (id, title, description, image)     |
| `GET`  | `/products/{id}/{lang}`            | Get full product detail in the requested language     |
| `GET`  | `/images/{product_id}/{file_name}` | Download a product image by filename                  |
| `GET`  | `/pages`                           | List all pages (id, title)                            |
| `GET`  | `/pages/{id}/{lang}`               | Get page content (Markdown) in the requested language |

Image paths returned in product responses (e.g. `/images/oak-shelf/thumb.jpg`) map directly to the image download
endpoint — prepend the server's base URL to get a complete download URL.

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
  cors_origins:
    - "https://example.com"
data_dir: "./data"
```

- `debug` — enable verbose logging (default: `false`).
- `server.addr` — address and port to listen on (default: `":9000"`).
- `server.cors_origins` — list of allowed CORS origins. Use `["*"]` to allow all origins.
- `data_dir` — root directory containing catalog files (default: `"./data"`).
