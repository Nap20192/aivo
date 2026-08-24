-- Payments/tenders on tickets, in-shift cash movements, and the shift
-- acceptance state (Open → Closed → Accepted, contract §3). Amount
-- columns are bigint cents -- single currency (company base);
-- multicurrency deferred (reference §16.4).

-- Payment methods per restaurant, grouped for GL semantics. Seed: cash,
-- card (done in cmd/aivo-seed / provisioning).
CREATE TABLE payment_methods (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    code          text NOT NULL,
    name          text NOT NULL,
    payment_group text NOT NULL, -- cash|card|gift_card|comp|void|house_account
    active        boolean NOT NULL DEFAULT true
);

CREATE UNIQUE INDEX payment_methods_restaurant_code_idx ON payment_methods (restaurant_id, code);

-- Tenders recorded at ticket close. payment_group snapshots the method's
-- group at the moment of payment.
CREATE TABLE ticket_payments (
    id            uuid PRIMARY KEY,
    ticket_id     uuid NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    method_id     uuid NOT NULL REFERENCES payment_methods (id),
    payment_group text NOT NULL,
    amount_cents  bigint NOT NULL, -- single currency (§16.4)
    tip_cents     bigint NOT NULL DEFAULT 0,
    recorded_at   timestamptz NOT NULL DEFAULT now(),
    recorded_by   uuid NOT NULL REFERENCES users (id)
);

CREATE INDEX ticket_payments_ticket_idx ON ticket_payments (ticket_id);

-- In-shift cash movements (pay-in / pay-out / drop), reference §7.
CREATE TABLE cash_operations (
    id            uuid PRIMARY KEY,
    shift_id      uuid NOT NULL REFERENCES shifts (id) ON DELETE CASCADE,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    kind          text NOT NULL, -- pay_in|pay_out|drop
    amount_cents  bigint NOT NULL, -- single currency (§16.4)
    reason        text NOT NULL DEFAULT '',
    recorded_by   uuid NOT NULL REFERENCES users (id),
    recorded_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cash_operations_shift_idx ON cash_operations (shift_id);

-- Shift acceptance state (D6). journal_document_id links the posted GL
-- document (no FK -- cross-context, like tickets.customer_id).
ALTER TABLE shifts ADD COLUMN accepted_at          timestamptz;
ALTER TABLE shifts ADD COLUMN accepted_by          uuid;
ALTER TABLE shifts ADD COLUMN journal_document_id  uuid;

-- One open shift per cashier (reference §5/§7 — per till AND per cashier).
CREATE UNIQUE INDEX shifts_open_per_cashier_idx
    ON shifts (restaurant_id, opened_by)
    WHERE closed_at IS NULL;

-- Ticket close timestamp (immutability guards key off status='open').
ALTER TABLE tickets ADD COLUMN closed_at timestamptz;
