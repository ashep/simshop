ALTER TABLE orders
    DROP COLUMN IF EXISTS ga_client_id,
    DROP COLUMN IF EXISTS ga_session_id;
