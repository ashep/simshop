-- Lossy: keeps only the first item of each order.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS product_id TEXT;

UPDATE orders o
SET product_id = i.product_id
FROM order_items i
WHERE i.order_id = o.id
  AND i.position = 0;

ALTER TABLE orders
    ALTER COLUMN product_id SET NOT NULL;

DELETE FROM order_attrs a
USING order_items i
WHERE i.id = a.item_id
  AND i.position <> 0;

DROP INDEX IF EXISTS order_attrs_order_id_idx;

ALTER TABLE order_attrs
    DROP COLUMN IF EXISTS item_id;

DROP TABLE IF EXISTS order_items;
