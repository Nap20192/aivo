-- Menu context schema. Postgres dialect.
-- Every tenant-scoped table carries restaurant_id for isolation; queries
-- MUST filter on it, never rely on FK presence alone.

CREATE TABLE restaurants (
    id         uuid PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tables (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    label         text NOT NULL,
    token         text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- A token is only meaningful within its owning restaurant; this also
-- backs TableByToken(restaurant_id, token) lookups.
CREATE UNIQUE INDEX tables_restaurant_id_token_idx ON tables (restaurant_id, token);

CREATE TABLE categories (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    name          text NOT NULL,
    position      integer NOT NULL DEFAULT 0
);

CREATE INDEX categories_restaurant_id_idx ON categories (restaurant_id);

CREATE TABLE menu_items (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    category_id   uuid NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    price_cents   integer NOT NULL,
    image_url     text NOT NULL DEFAULT '',
    available     boolean NOT NULL DEFAULT true
);

CREATE INDEX menu_items_restaurant_id_idx ON menu_items (restaurant_id);
CREATE INDEX menu_items_category_id_idx ON menu_items (category_id);

-- One row per (menu_item, allergen). allergen is one of the 14 EU
-- allergen category codes from domain.Allergen; enforced in application
-- code, not a DB enum, to keep adding a code a one-line const change.
CREATE TABLE menu_item_allergens (
    menu_item_id uuid NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
    allergen     text NOT NULL,
    PRIMARY KEY (menu_item_id, allergen)
);

CREATE TABLE option_groups (
    id           uuid PRIMARY KEY,
    menu_item_id uuid NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
    name         text NOT NULL,
    multi        boolean NOT NULL DEFAULT false,
    position     integer NOT NULL DEFAULT 0
);

CREATE INDEX option_groups_menu_item_id_idx ON option_groups (menu_item_id);

CREATE TABLE options (
    id              uuid PRIMARY KEY,
    option_group_id uuid NOT NULL REFERENCES option_groups (id) ON DELETE CASCADE,
    label           text NOT NULL,
    price_delta_cents integer NOT NULL DEFAULT 0,
    position        integer NOT NULL DEFAULT 0
);

CREATE INDEX options_option_group_id_idx ON options (option_group_id);

CREATE TABLE orders (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    table_id      uuid NOT NULL REFERENCES tables (id) ON DELETE CASCADE,
    comment       text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX orders_restaurant_id_idx ON orders (restaurant_id);
CREATE INDEX orders_table_id_idx ON orders (table_id);

-- Snapshot of a menu item at order time (name/price), plus the source
-- menu_item_id reference. Later edits to menu_items never alter these.
CREATE TABLE order_lines (
    id                uuid PRIMARY KEY,
    order_id          uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    menu_item_id      uuid NOT NULL REFERENCES menu_items (id),
    name              text NOT NULL,
    unit_price_cents  integer NOT NULL,
    qty               integer NOT NULL
);

CREATE INDEX order_lines_order_id_idx ON order_lines (order_id);

-- Snapshot of chosen options on an order line (label/price delta).
CREATE TABLE order_line_options (
    id                uuid PRIMARY KEY,
    order_line_id     uuid NOT NULL REFERENCES order_lines (id) ON DELETE CASCADE,
    label             text NOT NULL,
    price_delta_cents integer NOT NULL DEFAULT 0
);

CREATE INDEX order_line_options_order_line_id_idx ON order_line_options (order_line_id);

CREATE TABLE service_requests (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    table_id      uuid NOT NULL REFERENCES tables (id) ON DELETE CASCADE,
    kind          text NOT NULL, -- 'call_waiter' | 'request_bill'
    status        text NOT NULL DEFAULT 'pending', -- 'pending' | 'acknowledged'
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX service_requests_restaurant_id_idx ON service_requests (restaurant_id);

-- Backs HasOpenServiceRequest(table_id, kind): at most one pending
-- request of a given kind per table at a time.
CREATE UNIQUE INDEX service_requests_open_per_table_kind_idx
    ON service_requests (table_id, kind)
    WHERE status = 'pending';

CREATE TABLE landing_blocks (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    type          text NOT NULL, -- 'banner' | 'free_text' | 'opening_hours' | 'location' | 'social_links' | 'contact'
    position      integer NOT NULL DEFAULT 0,
    data          jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX landing_blocks_restaurant_id_idx ON landing_blocks (restaurant_id);

-- One channel per restaurant for v1 (see docs/adr/0001, docs/adr/0003).
-- key_version identifies which master key version encrypted
-- encrypted_bot_token, so a future key rotation can re-wrap rows without
-- a schema migration.
CREATE TABLE notification_channels (
    restaurant_id       uuid PRIMARY KEY REFERENCES restaurants (id) ON DELETE CASCADE,
    telegram_chat_id    text NOT NULL DEFAULT '',
    encrypted_bot_token bytea NOT NULL,
    key_version         integer NOT NULL
);
