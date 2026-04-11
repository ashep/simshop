CREATE TABLE IF NOT EXISTS languages
(
    id    TEXT PRIMARY KEY CHECK ( length(id) >= 2 ),
    title TEXT NOT NULL CHECK ( length(title) > 0 )
);

INSERT INTO languages
VALUES ('EN', 'English'),
       ('UK', 'Українська');

CREATE TABLE IF NOT EXISTS currencies
(
    id     TEXT NOT NULL PRIMARY KEY CHECK ( length(id) = 3 ),
    symbol TEXT NOT NULL CHECK ( length(symbol) > 0 )
);

INSERT INTO currencies
VALUES ('EUR', '€'),
       ('USD', '$'),
       ('UAH', '₴');

CREATE TABLE IF NOT EXISTS countries
(
    id          TEXT NOT NULL PRIMARY KEY CHECK ( length(id) >= 2 ),
    currency_id TEXT NOT NULL REFERENCES currencies (id)
);

INSERT INTO countries
VALUES ('DEFAULT', 'EUR'),
       ('UA', 'UAH');

CREATE TABLE IF NOT EXISTS users
(
    id      uuid NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    api_key TEXT NOT NULL CHECK ( length(api_key) >= 64 )
);

CREATE TABLE IF NOT EXISTS user_permissions
(
    user_id uuid NOT NULL REFERENCES users (id),
    scope   TEXT NOT NULL,
    PRIMARY KEY (user_id, scope)
);

WITH inserted_user AS (
    INSERT INTO users VALUES (uuidv7(), md5(random()::text) || md5(random()::text)) RETURNING id)
INSERT
INTO user_permissions
SELECT id, 'admin'
FROM inserted_user;


CREATE TABLE IF NOT EXISTS ext_users
(
    user_id    uuid                        NOT NULL REFERENCES users (id),
    ext_id     TEXt                        NOT NULL CHECK ( length(ext_id) > 0 ),
    ext_login  TEXT                        NOT NULL CHECK ( length(ext_login) > 3 ),
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITHOUT TIME ZONE,

    PRIMARY KEY (user_id, ext_id)
);

CREATE TABLE IF NOT EXISTS shops
(
    id         TEXT                        NOT NULL CHECK ( length(id) >= 3 ) PRIMARY KEY,
    owner_id   uuid                        NOT NULL REFERENCES users (id),
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITHOUT TIME ZONE
);

CREATE TABLE IF NOT EXISTS shop_names
(
    shop_id TEXT NOT NULL REFERENCES shops (id),
    lang_id TEXT NOT NULL REFERENCES languages (id),
    name    TEXT NOT NULL CHECK ( length(name) >= 3 ),
    PRIMARY KEY (shop_id, lang_id)
);

CREATE TABLE IF NOT EXISTS products
(
    id         uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    shop_id    TEXT REFERENCES shops (id),
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITHOUT TIME ZONE
);

CREATE TABLE IF NOT EXISTS product_prices
(
    product_id uuid NOT NULL REFERENCES products (id),
    country_id TEXT NOT NULL REFERENCES countries (id),
    value INT NOT NULL,
    PRIMARY KEY (product_id, country_id)
);

CREATE TABLE IF NOT EXISTS product_content
(
    product_id  uuid NOT NULL REFERENCES products (id),
    lang_id     TEXT NOT NULL REFERENCES languages (id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (product_id, lang_id)
);

CREATE TABLE IF NOT EXISTS properties
(
    id uuid NOT NULL PRIMARY KEY DEFAULT uuidv7()
);

CREATE TABLE IF NOT EXISTS property_names
(
    property_id uuid NOT NULL REFERENCES properties (id),
    lang_id     TEXT NOT NULL REFERENCES languages (id),
    name        TEXT NOT NULL CHECK ( length(name) > 0 ),
    PRIMARY KEY (property_id, lang_id)
);


CREATE TABLE IF NOT EXISTS product_property_values
(
    product_id  uuid NOT NULL REFERENCES products (id),
    property_id uuid NOT NULL REFERENCES properties (id),
    value       TEXT NOT NULL,
    price_add   INT,
    PRIMARY KEY (product_id, property_id)
);

