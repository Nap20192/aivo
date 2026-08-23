-- Domain event outbox, shared by all contexts (see internal/sharedkernel
-- and docs/EVENTS.md for the event catalog). Writers insert in the same
-- transaction as the aggregate change; a publisher marks published_at
-- after delivery. NULL published_at = pending.
CREATE TABLE events (
    id             uuid PRIMARY KEY,
    name           text NOT NULL,                  -- e.g. 'OrderPlaced'
    aggregate_type text NOT NULL,                  -- e.g. 'order'
    aggregate_id   uuid NOT NULL,
    restaurant_id  uuid,                           -- tenant scope; NULL for org-level events
    payload        jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz
);

CREATE INDEX events_pending ON events (occurred_at) WHERE published_at IS NULL;
CREATE INDEX events_aggregate ON events (aggregate_type, aggregate_id);
