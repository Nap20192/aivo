-- Cashier display name denormalized onto the shift at open time (kills
-- the per-poll user lookup in pos state), and the customer link on
-- tickets (set when a customer's cart handoff is accepted) so CRM spend
-- can include handoff sales.

ALTER TABLE shifts ADD COLUMN cashier text NOT NULL DEFAULT '';

ALTER TABLE tickets ADD COLUMN customer_id uuid; -- platform customers.id, no FK (cross-batch)

CREATE INDEX tickets_restaurant_customer_idx
    ON tickets (restaurant_id, customer_id)
    WHERE customer_id IS NOT NULL;
