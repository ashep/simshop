-- Supports the "one promo code per phone number" check in orderdb.Writer.Write,
-- which matches codes case-insensitively and phones by digits only.
CREATE INDEX IF NOT EXISTS orders_promo_phone_idx
    ON orders (lower(promo_code), regexp_replace(phone, '\D', '', 'g'))
    WHERE promo_code IS NOT NULL;
