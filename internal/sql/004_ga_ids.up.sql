-- Google Analytics ids of the buyer's browser at checkout, sent by the storefront.
-- The server-side purchase event carries them so GA attributes the sale to the
-- visitor's traffic source. NULL when the storefront sent none.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS ga_client_id  TEXT,
    ADD COLUMN IF NOT EXISTS ga_session_id TEXT;
