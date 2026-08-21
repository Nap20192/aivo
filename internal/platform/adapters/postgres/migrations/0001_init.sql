-- Platform context schema. Runs after the menu context's migrations
-- (restaurants table already exists there); this file adds the org/auth/
-- billing layer and extends restaurants with platform-owned columns.

CREATE TABLE organizations (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- restaurant_id is NULL for org-wide users (owners) and set for
-- restaurant-scoped staff (managers, waiters).
CREATE TABLE users (
    id            uuid PRIMARY KEY,
    org_id        uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email         text NOT NULL UNIQUE,
    password_hash bytea NOT NULL,
    role          text NOT NULL, -- 'owner' | 'manager' | 'waiter'
    restaurant_id uuid REFERENCES restaurants (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_org_id_idx ON users (org_id);

-- Server-side sessions; token_hash is SHA-256 of the aivo_session cookie
-- value (the raw token is never stored).
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- One subscription row per org. Status follows the state machine
-- trialing -> active -> past_due -> canceled (see platform domain).
CREATE TABLE subscriptions (
    org_id     uuid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    plan       text NOT NULL, -- 'free' | 'pro' | 'business'
    status     text NOT NULL, -- 'trialing' | 'active' | 'past_due' | 'canceled'
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Platform-owned columns on the menu context's restaurants table.
-- org_id is nullable only for rows that predate the platform (the menu
-- demo seed); every platform-provisioned restaurant sets it.
ALTER TABLE restaurants
    ADD COLUMN org_id   uuid REFERENCES organizations (id) ON DELETE CASCADE,
    ADD COLUMN address  text NOT NULL DEFAULT '',
    ADD COLUMN hours    text NOT NULL DEFAULT '',
    ADD COLUMN contacts jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX restaurants_org_id_idx ON restaurants (org_id);

-- Theme JSON + design.md source per restaurant (see docs/PLATFORM.md
-- "Menu customization").
CREATE TABLE restaurant_themes (
    restaurant_id uuid PRIMARY KEY REFERENCES restaurants (id) ON DELETE CASCADE,
    theme         jsonb NOT NULL DEFAULT '{}'::jsonb,
    design_md     text NOT NULL DEFAULT '',
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Custom domains -> restaurant, for Host-header routing. verified_at NULL
-- means claimed but not serving yet; certificate automation is out of
-- scope for v1 (documented stub, see docs/PLATFORM.md).
CREATE TABLE custom_domains (
    domain        text PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    verified_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX custom_domains_restaurant_id_idx ON custom_domains (restaurant_id);
