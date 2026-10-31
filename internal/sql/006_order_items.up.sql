-- Cart orders: an order holds one or more items, each a product bought qty
-- times. price is the unit price before discount (base + attr add-ons);
-- discount is the promo discount for the whole line. order_attrs now belong to
-- an item. Existing single-product orders become one item with qty 1.
CREATE TABLE IF NOT EXISTS order_items
(
    id         uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    order_id   uuid REFERENCES orders (id) NOT NULL,
    position   INT                         NOT NULL,
    product_id TEXT                        NOT NULL,
    qty        INT                         NOT NULL CHECK ( qty > 0 ),
    price      INT                         NOT NULL CHECK ( price >= 0 ),
    discount   INT                         NOT NULL DEFAULT 0 CHECK ( discount >= 0 ),

    UNIQUE (order_id, position)
);

INSERT INTO order_items (order_id, position, product_id, qty, price, discount)
SELECT id, 0, product_id, 1, price + COALESCE(discount, 0), COALESCE(discount, 0)
FROM orders;

ALTER TABLE order_attrs
    ADD COLUMN IF NOT EXISTS item_id uuid REFERENCES order_items (id);

UPDATE order_attrs a
SET item_id = i.id
FROM order_items i
WHERE i.order_id = a.order_id;

ALTER TABLE order_attrs
    ALTER COLUMN item_id SET NOT NULL;

ALTER TABLE orders
    DROP COLUMN IF EXISTS product_id;

CREATE INDEX IF NOT EXISTS order_attrs_order_id_idx ON order_attrs (order_id);
