CREATE TYPE order_status AS ENUM (
    'new',
    'awaiting_payment',
    'payment_processing',
    'payment_hold',
    'paid',
    'processing',
    'cancelled',
    'shipped',
    'delivered',
    'refund_requested',
    'returned', -- physical return received but not yet refunded
    'refunded'
    );

CREATE TYPE invoice_status AS ENUM (
    'processing',
    'hold',
    'paid',
    'failed',
    'reversed'
    );

CREATE TABLE IF NOT EXISTS orders
(
    id            uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    product_id    TEXT                        NOT NULL,
    status        order_status                NOT NULL             DEFAULT 'new',
    email         TEXT                        NOT NULL,
    price         INT                         NOT NULL CHECK ( price >= 0 ),
    currency      TEXT                        NOT NULL,
    lang          TEXT                        NOT NULL,
    first_name    TEXT                        NOT NULL,
    middle_name   TEXT,
    last_name     TEXT                        NOT NULL,
    country       TEXT                        NOT NULL,
    city          TEXT                        NOT NULL,
    phone         TEXT                        NOT NULL,
    address       TEXT                        NOT NULL,
    admin_note    TEXT,
    customer_note TEXT,
    created_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP
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
    id         uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    order_id   uuid REFERENCES orders (id) NOT NULL,
    status     order_status                NOT NULL,
    note       TEXT,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS order_history_order_id_idx ON order_history (order_id);

CREATE TABLE IF NOT EXISTS order_invoices
(
    id         TEXT                        NOT NULL,
    order_id   uuid REFERENCES orders (id) NOT NULL,
    provider   TEXT                        NOT NULL,
    page_url   TEXT                        NOT NULL,
    amount     INT                         NOT NULL CHECK (amount >= 0),
    currency   TEXT                        NOT NULL,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (id, provider)
);

CREATE INDEX IF NOT EXISTS order_invoices_order_id_idx ON order_invoices (order_id);

CREATE TABLE IF NOT EXISTS invoice_history
(
    id         uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    order_id   uuid                        NOT NULL REFERENCES orders (id),
    invoice_id TEXT                        NOT NULL,
    provider   TEXT                        NOT NULL,
    status     invoice_status              NOT NULL,
    note       TEXT,
    payload    JSONB                       NOT NULL,
    event_at   TIMESTAMP WITHOUT TIME ZONE NOT NULL, -- modifiedDate from webhook
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (invoice_id, provider, status, event_at)
);

CREATE INDEX IF NOT EXISTS invoice_history_order_idx
    ON invoice_history (order_id, event_at DESC);