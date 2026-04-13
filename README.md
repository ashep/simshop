# SimShop

SimShop is a read-only HTTP API service that serves catalog data (products) loaded from YAML files at startup.

## Purpose

SimShop is designed as a lightweight catalog backend where:

- Products are defined in YAML files on disk and served via a JSON REST API.

## Key Concepts

### YAML-based catalog

All catalog data is loaded from a configurable `data_dir` at startup. No database is required. The catalog is
read-only at runtime — to update content, update the YAML files and restart the service.

## Data Entities

### Product

Each product has:

- **ID** — a UUID assigned at load time (derived from the YAML filename).
- **Data** — per-language title and description (e.g. `EN`, `UK`).

## API Overview

The service exposes a read-only JSON REST API validated against an OpenAPI specification.

| Method | Path        | Description       | Auth required |
|--------|-------------|-------------------|---------------|
| `GET`  | `/products` | List all products | No            |

## Configuration

Config is loaded from `config.yml`:

```yaml
debug: false
server:
  addr: ":9000"
data_dir: "./data"
```

- `data_dir` — root directory containing YAML catalog files (default: `./data`).

## Filesystem Layout

```
{data_dir}/
  products/
    {product-id}.yaml
```

Each `{product-id}.yaml` file in `products/` defines one product including its multilingual data.
