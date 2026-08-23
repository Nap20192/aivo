-- POS context schema: shifts and tickets. Service requests live in the
-- menu context (POS reads them through the in-process bridge).

-- A cash shift. Closing posts expected/declared/variance immutably
-- (closed shifts are never updated again — enforced in the store by
-- "WHERE closed_at IS NULL").
CREATE TABLE shifts (
    id                  uuid PRIMARY KEY,
    restaurant_id       uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    opened_by           uuid NOT NULL REFERENCES users (id),
    opened_at           timestamptz NOT NULL DEFAULT now(),
    opening_float_cents integer NOT NULL,
    closed_at           timestamptz,
    declared_cents      integer,
    expected_cents      integer,
    variance_cents      integer
);

CREATE INDEX shifts_restaurant_id_idx ON shifts (restaurant_id);

-- At most one open shift per restaurant.
CREATE UNIQUE INDEX shifts_open_per_restaurant_idx
    ON shifts (restaurant_id)
    WHERE closed_at IS NULL;

-- One ticket per table per shift while open. Lines snapshot menu items
-- at add time (same principle as menu order_lines).
CREATE TABLE tickets (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    shift_id      uuid NOT NULL REFERENCES shifts (id) ON DELETE CASCADE,
    table_id      uuid NOT NULL REFERENCES tables (id) ON DELETE CASCADE,
    status        text NOT NULL DEFAULT 'open', -- 'open' | 'closed'
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tickets_restaurant_id_idx ON tickets (restaurant_id);
CREATE INDEX tickets_shift_id_idx ON tickets (shift_id);

CREATE UNIQUE INDEX tickets_open_per_table_idx
    ON tickets (table_id)
    WHERE status = 'open';

CREATE TABLE ticket_lines (
    id               uuid PRIMARY KEY,
    ticket_id        uuid NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    menu_item_id     uuid NOT NULL REFERENCES menu_items (id),
    name             text NOT NULL,
    unit_price_cents integer NOT NULL,
    qty              integer NOT NULL,
    options          jsonb NOT NULL DEFAULT '[]'::jsonb, -- [{label, price_delta_cents}]
    fired_at         timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ticket_lines_ticket_id_idx ON ticket_lines (ticket_id);
