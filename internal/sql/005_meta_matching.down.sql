ALTER TABLE orders
    DROP COLUMN IF EXISTS meta_fbp,
    DROP COLUMN IF EXISTS meta_fbc,
    DROP COLUMN IF EXISTS client_ip,
    DROP COLUMN IF EXISTS user_agent;
