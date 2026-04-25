CREATE TABLE IF NOT EXISTS order_invoices
(
    id         uuid PRIMARY KEY                     DEFAULT uuidv7(),
    order_id   uuid REFERENCES orders (id) NOT NULL,
    provider   TEXT                        NOT NULL,
    invoice_id TEXT                        NOT NULL,
    page_url   TEXT                        NOT NULL,
    amount     INT                         NOT NULL CHECK (amount >= 0),
    currency   TEXT                        NOT NULL,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS order_invoices_order_id_idx ON order_invoices (order_id);
