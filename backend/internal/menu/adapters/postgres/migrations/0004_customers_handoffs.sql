-- Customer linkage + cart handoff codes.
-- orders.customer_id references the platform context's customers table,
-- which is created in a LATER migration batch (platform runs after
-- menu) — the FK constraint is added there (platform/0003), the column
-- here.

ALTER TABLE orders ADD COLUMN customer_id uuid;

CREATE INDEX orders_customer_id_idx ON orders (customer_id) WHERE customer_id IS NOT NULL;

-- A cart handed to the waiter as a short pickup code. Single-use,
-- 15-minute TTL, at most one active per table (creating a new one
-- deletes the previous active). lines stores the validated snapshot
-- (names/prices for the POS preview) plus the source ids so accepting
-- re-runs the normal add-lines path.
CREATE TABLE cart_handoffs (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    table_id      uuid NOT NULL REFERENCES tables (id) ON DELETE CASCADE,
    customer_id   uuid, -- platform customers.id, no FK (see header note)
    code          text NOT NULL,
    lines         jsonb NOT NULL,
    note          text NOT NULL DEFAULT '',
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Codes are unique among unused ones (expired-unused rows age out of
-- relevance; the lookup always also checks expires_at).
CREATE UNIQUE INDEX cart_handoffs_code_active_idx ON cart_handoffs (code) WHERE used_at IS NULL;
CREATE INDEX cart_handoffs_table_idx ON cart_handoffs (table_id);
