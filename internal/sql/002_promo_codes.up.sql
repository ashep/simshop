-- Promo code applied to an order: all three NULL when no code was used, all set
-- when one was. orders.price stays the charged (discounted) total.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS promo_code       TEXT,
    ADD COLUMN IF NOT EXISTS discount_percent INT CHECK ( discount_percent BETWEEN 1 AND 99 ),
    ADD COLUMN IF NOT EXISTS discount         INT CHECK ( discount >= 0 );
