-- Customer accounts (platform-global diner logins) + light CRM.

CREATE TABLE customers (
    id            uuid PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    password_hash bytea NOT NULL,
    name          text NOT NULL,
    phone         text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Separate session store from staff sessions: a customer cookie can
-- never resolve to a staff user or vice versa.
CREATE TABLE customer_sessions (
    token_hash  bytea PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);

CREATE INDEX customer_sessions_customer_idx ON customer_sessions (customer_id);

-- Now that customers exists, constrain the column menu/0004 added.
ALTER TABLE orders
    ADD CONSTRAINT orders_customer_id_fkey
    FOREIGN KEY (customer_id) REFERENCES customers (id) ON DELETE SET NULL;

-- Light CRM: one row per (restaurant, customer), created lazily on the
-- first linked order/handoff. The privacy boundary: a restaurant sees
-- only customers with a row here, and only its own orders.
CREATE TABLE guest_profiles (
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    customer_id   uuid NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    notes         text NOT NULL DEFAULT '',
    tags          jsonb NOT NULL DEFAULT '[]'::jsonb, -- ["regular", "vip", ...]
    first_seen    timestamptz NOT NULL DEFAULT now(),
    last_seen     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (restaurant_id, customer_id)
);
