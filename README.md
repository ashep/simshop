# SimShop

SimShop is a read-only HTTP API service that serves catalog data (products, properties, and product files) loaded from
YAML files at startup.

## Purpose

SimShop is designed as a lightweight catalog backend where:

- Products are defined in YAML files on disk and served via a JSON REST API.
- Prices are resolved per country with a `DEFAULT` fallback.
- Catalog attributes are defined as reusable properties.
- File metadata (MIME type, size, path) is served alongside products.

## Key Concepts

### YAML-based catalog

All catalog data is loaded from a configurable `data_dir` at startup. No database is required. The catalog is
read-only at runtime — to update content, update the YAML files and restart the service.

### Country-based pricing

Product prices are defined per country code directly in the YAML file. A `DEFAULT` key serves as the fallback when no
country-specific price exists. `GET /products/{id}/prices?country=XX` resolves the price for the requested country.
The response always echoes back the requested country code in `country_id`, even when the resolved price comes from
the `DEFAULT` fallback.

## Data Entities

### Product

Each product has:

- **ID** — a UUID assigned at load time (derived from the YAML filename or explicit field).
- **Data** — per-language title and description (e.g. `EN`, `UK`).
- **Prices** — per-country integer prices (smallest currency unit), with a `DEFAULT` fallback key.
- **Files** — list of file names attached to the product (resolved to `FileInfo` from the file catalog).

### Property

A property is a reusable catalog attribute (e.g. "Color", "Size"). Each property has:

- **ID** — a UUID.
- **Titles** — a map of language code → human-readable title.

### File

A file record describes a binary asset associated with a product:

- **Name** — filename.
- **MIME type** — content type.
- **Size** — byte count.
- **Path** — URL-relative path to the file on disk.

## API Overview

The service exposes a read-only JSON REST API validated against an OpenAPI specification.

| Method | Path                      | Description                                          | Auth required |
|--------|---------------------------|------------------------------------------------------|---------------|
| `GET`  | `/products`               | List all products                                    | No            |
| `GET`  | `/products/{id}`          | Get a single product by ID                           | No            |
| `GET`  | `/products/{id}/prices`   | Get the resolved price for a country (`?country=XX`) | No            |
| `GET`  | `/products/{id}/files`    | List files attached to a product                     | No            |
| `GET`  | `/properties`             | List all properties                                  | No            |

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
  public_dir: "./public"
data_dir: "./data"
```

- `server.public_dir` — directory where static files are served from (default: `./public`).
- `data_dir` — root directory containing YAML catalog files (default: `./data`).

## Filesystem Layout

```
{data_dir}/
  properties.yaml
  products/
    {product-id}.yaml

{public_dir}/
  {product-id}/
    image.jpg
    manual.pdf
```

The `properties.yaml` file defines reusable catalog attributes. Each `{product-id}.yaml` file in `products/` defines
one product including its multilingual data, prices, and the list of file names attached to it. Binary assets (images,
PDFs, etc.) are placed under `{public_dir}/{product-id}/` and are served as static files. A binary asset referenced
in a product YAML but absent on disk is silently skipped at load time.
