-- Inventory's own outbox (service-events capability), mirroring
-- platform's events table shape (migrations/platform/0004_events.up.sql).
-- Inventory's transaction that inserts a row here also has this
-- connection's search_path set to "inventory, public" (see
-- cmd/aivo-inventory/main.go), so this table and the outbox.Publish/Poller
-- helpers (backend/pkg/outbox) that read/write it land in the inventory
-- schema, isolated from platform's own events table.
CREATE TABLE events (
    id             uuid PRIMARY KEY,
    name           text NOT NULL,                  -- e.g. 'InventoryReceiptPosted'
    aggregate_type text NOT NULL,                  -- e.g. 'goods_receipt'
    aggregate_id   uuid NOT NULL,
    restaurant_id  uuid,                           -- tenant scope; NULL for org-level events
    payload        jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz
);

CREATE INDEX events_pending ON events (occurred_at) WHERE published_at IS NULL;
CREATE INDEX events_aggregate ON events (aggregate_type, aggregate_id);
