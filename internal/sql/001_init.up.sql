CREATE TYPE order_status AS ENUM (
    'new',
    'paid',
    'payment_failed',
    'processing',
    'cancelled',
    'shipped',
    'delivered',
    'refund_requested',
    'returned', -- physical return received but not yet refunded
    'refunded'
    );

CREATE TABLE IF NOT EXISTS orders
(
    id            uuid PRIMARY KEY                     DEFAULT uuidv7(),
    product_id    TEXT                        NOT NULL,
    status        order_status                NOT NULL DEFAULT 'new',
    email         TEXT                        NOT NULL,
    price         INT                         NOT NULL CHECK ( price >= 0 ),
    currency      TEXT                        NOT NULL,
    first_name    TEXT                        NOT NULL,
    middle_name   TEXT,
    last_name     TEXT                        NOT NULL,
    country       TEXT                        NOT NULL,
    city          TEXT                        NOT NULL,
    phone         TEXT                        NOT NULL,
    address       TEXT                        NOT NULL,
    admin_note    TEXT,
    customer_note TEXT,
    created_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS order_attrs
(
    order_id   uuid REFERENCES orders (id) NOT NULL,
    attr_name  TEXT                        NOT NULL,
    attr_value TEXT                        NOT NULL,
    attr_price INT                         NOT NULL
);

CREATE TABLE IF NOT EXISTS order_history
(
    id         uuid PRIMARY KEY                     DEFAULT uuidv7(),
    order_id   uuid REFERENCES orders (id) NOT NULL,
    status     order_status                NOT NULL,
    note       TEXT,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS order_history_order_id_idx ON order_history (order_id);
