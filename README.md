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

### Product

Each product lives in its own subdirectory under `{data_dir}/products/`. The directory name is the product ID.
The product is described by a `product.yaml` file inside that directory.

Fields:

- **name** — multilingual product name (e.g. `en`, `uk`).
- **description** — multilingual long-form description; must cover the same languages as `name`.
- **specs** — optional map of specification keys, each translated into every language defined in `name`.
  Each spec entry has a `title` and `value`.
- **price** — map of country/region codes to `{currency, value}` pairs. Must contain a `default` key.
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

## API Overview

The service exposes a read-only JSON REST API validated against an OpenAPI specification.

| Method | Path                                | Description                        |
|--------|-------------------------------------|------------------------------------|
| `GET`  | `/products`                         | List all products                  |
| `GET`  | `/images/{product_id}/{file_name}`  | Download a product image by name   |

Image paths returned by `GET /products` (e.g. `/images/some-id/thumb.jpg`) map directly to the image download
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
  products/
    {product-id}/
      product.yaml
      images/
        01-preview.png
        01.png
```

Each subdirectory under `products/` defines one product. The directory name becomes the product ID. Image files
are placed in the `images/` subdirectory and referenced by filename in `product.yaml`.
