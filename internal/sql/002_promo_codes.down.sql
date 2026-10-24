ALTER TABLE orders
    DROP COLUMN IF EXISTS discount,
    DROP COLUMN IF EXISTS discount_percent,
    DROP COLUMN IF EXISTS promo_code;
