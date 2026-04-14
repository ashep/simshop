# SimShop

SimShop is a read-only HTTP API service that serves catalog data (products) loaded from YAML files at startup.

## Purpose

SimShop is designed as a lightweight catalog backend where:

- Products are defined in YAML files on disk and served via a JSON REST API.

## Key Concepts

### YAML-based catalog

All catalog data is loaded from a configurable `data_dir` at startup. No database is required. The catalog is
read-only at runtime — to update content, update the YAML files and restart the service.

### Validation at startup

Every product YAML is validated when the service starts. Invalid products are fatal — the service refuses to start
if any product file fails validation. Rules:

- `name` and `description` are required and must define the same set of languages.
- Every spec entry must be translated into all languages defined in `name`.
- `price` must define at least a `default` key.
- Every attribute entry must be translated into all languages defined in `name`, and each language entry must have
  at least one value.
- All image paths (`preview` and `full`) must exist on disk relative to the product's `images/` subdirectory.

## Data Entities

### Product listing (`products.yaml`)

The product listing served by `GET /products` comes from `{data_dir}/products/products.yaml`. This is a flat YAML
file containing lightweight product entries: an `id`, a multilingual `title`, and a multilingual `description`. A
missing `products.yaml` results in an empty list — it is not an error.

Each entry in the listing response also includes an optional `image` field — the URL path of the first preview image
from the product's `product.yaml` (e.g. `/images/cronus/thumb.png`). The field is omitted when the product has no
images or when the product subdirectory has no `product.yaml`.

Example `products.yaml`:

```yaml
products:
  - id: cronus
    title:
      en: Cronus
      uk: Cronus
    description:
      en: A wooden desktop clock.
      uk: Настільний годинник у деревʼяному корпусі.
```

### Product detail content

Per-language long-form content lives in `{data_dir}/products/{id}/{lang}.md`. `GET /products/{id}/{lang}` returns
the raw Markdown content as `text/plain`. Returns 404 if the product ID or language file does not exist.

### Product (full definition, validated at startup)

Each product may also have a subdirectory under `{data_dir}/products/` with a `product.yaml` file. If present,
it is validated at startup to enforce data integrity. Subdirectories without a `product.yaml` are silently skipped
and treated as content-only directories (housing `{lang}.md` files).

Fields in `product.yaml`:

- **name** — multilingual product name (e.g. `en`, `uk`).
- **description** — multilingual long-form description; must cover the same languages as `name`.
- **specs** — optional map of specification keys, each translated into every language defined in `name`.
  Each spec entry has a `title` and `value`.
- **price** — map of country/region codes (lowercase, e.g. `ua`, `us`) to `{currency, value}` pairs. Must contain
  a `default` key. The API resolves this map to a single `{currency, value}` object per request: the country is
  detected from the `CF-IPCountry` request header (sent by Cloudflare) or via an ipinfo.io lookup on the client IP.
  If no matching country key exists the `default` entry is used.
- **attrs** — optional map of attribute keys (e.g. `display_color`). Each attribute is translated into every
  language in `name`. Each language entry has a `title` and a `values` map with at least one entry;
  each value has a `title` and an `add_price` surcharge.
- **images** — optional list of `{preview, full}` filename pairs. Filenames are relative to the product's
  `images/` subdirectory. The API response returns these as URL paths (e.g. `/images/{id}/thumb.jpg`) that
  can be appended to the server's base URL to download the file.

Example `product.yaml`:

```yaml
name:
  en: Cronus
  uk: Cronus

description:
  en: A wooden desktop clock.
  uk: Настільний годинник у деревʼяному корпусі.

specs:
  weight:
    en:
      title: Weight
      value: 420 g
    uk:
      title: Вага
      value: 420 г

price:
  default:
    currency: EUR
    value: 100
  ua:
    currency: UAH
    value: 99

attrs:
  display_color:
    en:
      title: Display color
      values:
        red:
          title: Red
          add_price: 0
    uk:
      title: Колір дисплея
      values:
        red:
          title: Червоний
          add_price: 0

images:
  - preview: 01-preview.png
    full: 01.png
```

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

| Method | Path                                | Description                             |
|--------|-------------------------------------|-----------------------------------------|
| `GET`  | `/shop`                             | Get shop metadata (name, title, desc)   |
| `GET`  | `/products`                         | List all products (id, title, desc)     |
| `GET`  | `/products/{id}/{lang}`             | Get product content in a language       |
| `GET`  | `/images/{product_id}/{file_name}`  | Download a product image by name        |
| `GET`  | `/pages`                            | List all page IDs                       |
| `GET`  | `/pages/{id}/{lang}`                | Get page content in a language          |

Image paths associated with a product (e.g. `/images/some-id/thumb.jpg`) map directly to the image download
endpoint — prepend the server's base URL to get a complete download URL.

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
data_dir: "./data"
```

- `data_dir` — root directory containing product subdirectories (default: `./data`).

## Filesystem Layout

```
{data_dir}/
  shop.yaml                # shop metadata (name, title, description)
  products/
    products.yaml          # flat product listing (id, title, description)
    {product-id}/
      product.yaml         # optional: full product definition (validated at startup)
      en.md                # optional: per-language content for GET /products/{id}/{lang}
      uk.md
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
`product.yaml` are silently skipped by the validator and serve only as content directories for `{lang}.md` files.
Image files are placed in the `images/` subdirectory and referenced by filename in `product.yaml`.
