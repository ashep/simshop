-- Buyer matching data for the Meta Conversions API Purchase event: Pixel cookies
-- sent by the storefront, plus the IP and User-Agent seen on POST /orders.
-- NULL when unknown.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS meta_fbp   TEXT,
    ADD COLUMN IF NOT EXISTS meta_fbc   TEXT,
    ADD COLUMN IF NOT EXISTS client_ip  TEXT,
    ADD COLUMN IF NOT EXISTS user_agent TEXT;
