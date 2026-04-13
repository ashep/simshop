# SimShop

SimShop is an HTTP API service for managing a multi-tenant e-commerce platform. It provides the building blocks for
running one or more online shops, each with their own products, multilingual content, and country-specific pricing.

## Purpose

SimShop is designed as a backend for marketplace or shop-hosting scenarios where:

- Multiple independent shops coexist on the same platform.
- Each shop has an owner responsible for managing it.
- Products are listed under a shop and described in multiple languages.
- Prices vary by country/region.
- Catalog attributes are defined as reusable properties that can be attached to products.

## Key Concepts

### Multilingual content

All user-facing text — shop names and descriptions, product titles and descriptions, property titles — is stored per
language. At minimum an English (`EN`) entry is required. Additional languages (e.g. Ukrainian `uk`) can be supplied for
any entity.

### Country-based pricing

Product prices are defined per country. A `DEFAULT` country price serves as the fallback, with country-specific
overrides (e.g. `UA`) layered on top. Pricing is managed separately from product creation.

### Authentication and authorization

Access to write operations is controlled by API keys passed in the `X-API-Key` request header. Each user holds a set of
permission scopes. The `admin` scope grants platform-wide privileges, such as creating shops and properties. Read
endpoints are generally public, but authenticated admin callers may receive additional fields in the response (e.g.
owner ID, timestamps).

## Data Entities

### User

A platform account identified by a UUID. Each user has an API key used for authentication and a set of permission
scopes (e.g. `admin`). External identity providers can be linked to a user through the external user record (`ext_id`,
`ext_login`).

### Shop

A shop is the top-level container for products. It is identified by a short human-readable string ID (3–16 characters).
Each shop has:

- **Names** — a map of language code → name string (English required).
- **Descriptions** — an optional map of language code → description string.
- **Owner** — a reference to the user who owns the shop.
- **Max products** — an integer cap on the number of active (non-deleted) products the shop may hold (default: 50).
  Attempting to create a product when the cap is reached returns `409 Conflict`.
- **Timestamps** — creation and last-update times.

### Product

A product belongs to a shop. Each product has:

- **Data** — per-language title and description.
- **Prices** — per-country integer prices (smallest currency unit). Managed separately from product creation.

### Property

A property is a reusable catalog attribute (e.g. "Color", "Size") defined at the platform level and shared across shops.
Each property has:

- **Titles** — a map of language code → human-readable title.

Properties can be associated with products to capture variant-specific values and optional price adjustments.

### File

A binary object (image, document) uploaded by an authenticated user and stored in the database. Each file has:

- **MIME type** — detected from the first 512 bytes of the upload (not from the filename or header).
- **Size** — byte count of the file content.
- **Data** — the raw bytes stored as `BYTEA`.
- **Owner** — the user who uploaded the file.
- **Path** — URL-relative path to the materialized file on disk (`{server.public_dir}/files/{id}/{name}`), returned in
  the upload response and in file-listing responses.

After the database row is committed, the file is materialized to `{server.public_dir}/files/{id}/{name}` on the local
filesystem. If materialization fails after commit, the row persists in the DB and `GetForProduct` will re-materialize
from the stored `data` column on the next read.

File uploads are subject to per-user quota enforcement. Admins bypass the quota but are still subject to the size
limit. Allowed MIME types are configured at startup; unsupported types are rejected with `400 Bad Request`.

Files can be attached to products via `PUT /products/{id}/files`. The operation fully replaces the current set of
attachments. Non-admin callers (shop owners) may only attach files whose `owner_id` matches the product's shop owner.
Admins may attach any file to any product.

### Supporting entities

| Entity   | Purpose                                                     |
|----------|-------------------------------------------------------------|
| Language | Supported locale codes (e.g. `EN`, `UK`).                   |
| Currency | Currency definitions (e.g. `EUR`, `USD`, `UAH`).            |
| Country  | Country codes mapped to their currency (e.g. `UA` → `UAH`). |

## API Overview

The service exposes a JSON REST API validated against an OpenAPI specification.

| Method  | Path               | Description                                          | Auth required |
|---------|--------------------|------------------------------------------------------|---------------|
| `GET`   | `/shops`           | List all shops                                       | Admin         |
| `POST`  | `/shops`           | Create a shop                                        | Admin         |
| `GET`   | `/shops/{id}`      | Get a shop (public; extra fields for admins)         | No            |
| `PUT`   | `/shops/{id}`      | Fully replace a shop's titles (and descriptions)     | Owner/Admin   |
| `POST`  | `/products`               | Create a product in a shop                                    | Admin       |
| `GET`   | `/products/{id}`          | Get a product (public; extra fields for admin/owner)          | No          |
| `PATCH` | `/products/{id}`          | Fully replace a product's content (EN title required)         | Owner/Admin |
| `PUT`   | `/products/{id}/prices`   | Fully replace all prices for a product                        | Owner/Admin |
| `GET`   | `/products/{id}/prices`   | Get the resolved price for a country (`?country=XX`)          | No          |
| `GET`   | `/products/{id}/files`    | List files attached to a product (public; timestamps for admin/owner) | No |
| `PUT`   | `/products/{id}/files`    | Fully replace all file attachments for a product              | Owner/Admin |
| `GET`   | `/shops/{id}/products`    | List all products in a shop (public; extra fields for admin/owner) | No     |
| `POST`  | `/properties`      | Create a property                                    | Admin         |
| `GET`   | `/properties`      | List all properties                                  | No            |
| `PUT` | `/properties/{id}` | Update a property's titles                           | Admin         |
| `POST`  | `/files`           | Upload a file (`multipart/form-data`, fields `file` + `name`); returns 7 fields including `path` | Yes |
