CREATE TABLE IF NOT EXISTS languages
(
    id    TEXT PRIMARY KEY CHECK ( length(id) >= 2 ),
    title TEXT NOT NULL CHECK ( length(title) > 0 )
);

INSERT INTO languages
VALUES ('AR', 'العربية'),
       ('BN', 'বাংলা'),
       ('CS', 'Čeština'),
       ('DA', 'Dansk'),
       ('DE', 'Deutsch'),
       ('EL', 'Ελληνικά'),
       ('EN', 'English'),
       ('ES', 'Español'),
       ('FA', 'فارسی'),
       ('FI', 'Suomi'),
       ('FR', 'Français'),
       ('HE', 'עברית'),
       ('HI', 'हिन्दी'),
       ('HR', 'Hrvatski'),
       ('HU', 'Magyar'),
       ('ID', 'Bahasa Indonesia'),
       ('IT', 'Italiano'),
       ('JA', '日本語'),
       ('KO', '한국어'),
       ('MS', 'Bahasa Melayu'),
       ('NL', 'Nederlands'),
       ('NO', 'Norsk'),
       ('PL', 'Polski'),
       ('PT', 'Português'),
       ('RO', 'Română'),
       ('RU', 'Русский'),
       ('SK', 'Slovenčina'),
       ('SV', 'Svenska'),
       ('TH', 'ภาษาไทย'),
       ('TR', 'Türkçe'),
       ('UK', 'Українська'),
       ('VI', 'Tiếng Việt'),
       ('ZH', '中文');

CREATE TABLE IF NOT EXISTS currencies
(
    id     TEXT NOT NULL PRIMARY KEY CHECK ( length(id) = 3 ),
    symbol TEXT NOT NULL CHECK ( length(symbol) > 0 )
);

INSERT INTO currencies
VALUES ('AED', 'د.إ'),
       ('ARS', '$'),
       ('AUD', '$'),
       ('BDT', '৳'),
       ('BRL', 'R$'),
       ('CAD', '$'),
       ('CHF', 'Fr'),
       ('CNY', '¥'),
       ('CZK', 'Kč'),
       ('DKK', 'kr'),
       ('EGP', '£'),
       ('EUR', '€'),
       ('GBP', '£'),
       ('HKD', '$'),
       ('HUF', 'Ft'),
       ('IDR', 'Rp'),
       ('ILS', '₪'),
       ('INR', '₹'),
       ('JPY', '¥'),
       ('KRW', '₩'),
       ('MXN', '$'),
       ('MYR', 'RM'),
       ('NGN', '₦'),
       ('NOK', 'kr'),
       ('NZD', '$'),
       ('PHP', '₱'),
       ('PKR', '₨'),
       ('PLN', 'zł'),
       ('RON', 'lei'),
       ('RUB', '₽'),
       ('SAR', '﷼'),
       ('SEK', 'kr'),
       ('SGD', '$'),
       ('THB', '฿'),
       ('TRY', '₺'),
       ('TWD', '$'),
       ('UAH', '₴'),
       ('USD', '$'),
       ('VND', '₫'),
       ('ZAR', 'R');

CREATE TABLE IF NOT EXISTS countries
(
    id          TEXT NOT NULL PRIMARY KEY CHECK ( length(id) >= 2 ),
    currency_id TEXT NOT NULL REFERENCES currencies (id)
);

INSERT INTO countries
VALUES ('DEFAULT', 'EUR'),
       ('AE', 'AED'),
       ('AR', 'ARS'),
       ('AT', 'EUR'),
       ('AU', 'AUD'),
       ('BD', 'BDT'),
       ('BE', 'EUR'),
       ('BR', 'BRL'),
       ('CA', 'CAD'),
       ('CH', 'CHF'),
       ('CN', 'CNY'),
       ('CZ', 'CZK'),
       ('DE', 'EUR'),
       ('DK', 'DKK'),
       ('EG', 'EGP'),
       ('ES', 'EUR'),
       ('FI', 'EUR'),
       ('FR', 'EUR'),
       ('GB', 'GBP'),
       ('GR', 'EUR'),
       ('HK', 'HKD'),
       ('HU', 'HUF'),
       ('ID', 'IDR'),
       ('IE', 'EUR'),
       ('IL', 'ILS'),
       ('IN', 'INR'),
       ('IT', 'EUR'),
       ('JP', 'JPY'),
       ('KR', 'KRW'),
       ('MX', 'MXN'),
       ('MY', 'MYR'),
       ('NG', 'NGN'),
       ('NL', 'EUR'),
       ('NO', 'NOK'),
       ('NZ', 'NZD'),
       ('PH', 'PHP'),
       ('PK', 'PKR'),
       ('PL', 'PLN'),
       ('PT', 'EUR'),
       ('RO', 'RON'),
       ('RU', 'RUB'),
       ('SA', 'SAR'),
       ('SE', 'SEK'),
       ('SG', 'SGD'),
       ('SK', 'EUR'),
       ('TH', 'THB'),
       ('TR', 'TRY'),
       ('TW', 'TWD'),
       ('UA', 'UAH'),
       ('US', 'USD'),
       ('VN', 'VND'),
       ('ZA', 'ZAR');

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
    id           TEXT                        NOT NULL CHECK ( length(id) >= 3 ) PRIMARY KEY,
    owner_id     uuid                        NOT NULL REFERENCES users (id),
    max_products INT                         NOT NULL DEFAULT 50,
    created_at   TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at   TIMESTAMP WITHOUT TIME ZONE
);

CREATE TABLE IF NOT EXISTS shop_data
(
    shop_id     TEXT NOT NULL REFERENCES shops (id),
    lang_id     TEXT NOT NULL REFERENCES languages (id),
    title       TEXT NOT NULL CHECK ( length(title) >= 3 ),
    description TEXT,
    PRIMARY KEY (shop_id, lang_id)
);

CREATE TABLE IF NOT EXISTS properties
(
    id uuid NOT NULL PRIMARY KEY DEFAULT uuidv7()
);

CREATE TABLE IF NOT EXISTS property_titles
(
    property_id uuid NOT NULL REFERENCES properties (id),
    lang_id     TEXT NOT NULL REFERENCES languages (id),
    title       TEXT NOT NULL CHECK ( length(title) > 0 ),
    PRIMARY KEY (lang_id, title)
);


CREATE TABLE IF NOT EXISTS files
(
    id         uuid                        NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    owner_id   uuid                        NOT NULL REFERENCES users (id),
    name       TEXT                        NOT NULL,
    mime_type  TEXT                        NOT NULL,
    size_bytes INT                         NOT NULL,
    data       BYTEA                       NOT NULL,
    metadata   jsonb,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL             DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITHOUT TIME ZONE
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
    value      INT  NOT NULL,
    PRIMARY KEY (product_id, country_id)
);

CREATE TABLE IF NOT EXISTS product_data
(
    product_id  uuid NOT NULL REFERENCES products (id),
    lang_id     TEXT NOT NULL REFERENCES languages (id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (product_id, lang_id)
);

CREATE TABLE IF NOT EXISTS property_values
(
    product_id  uuid NOT NULL REFERENCES products (id),
    property_id uuid NOT NULL REFERENCES properties (id),
    ordering    INT  NOT NULL DEFAULT 0,
    value       TEXT NOT NULL,
    PRIMARY KEY (product_id, property_id)
);

CREATE TABLE IF NOT EXISTS property_prices
(
    product_id  uuid NOT NULL REFERENCES products (id),
    property_id uuid NOT NULL REFERENCES properties (id),
    country_id  TEXT NOT NULL REFERENCES countries (id),
    price       INT  NOT NULL,
    PRIMARY KEY (product_id, property_id, country_id)
);

CREATE TABLE IF NOT EXISTS product_files
(
    product_id uuid NOT NULL REFERENCES products (id),
    file_id    uuid NOT NULL REFERENCES files (id),
    PRIMARY KEY (product_id, file_id)
);
