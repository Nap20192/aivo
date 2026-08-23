DROP INDEX IF EXISTS tickets_restaurant_customer_idx;
ALTER TABLE tickets DROP COLUMN IF EXISTS customer_id;
ALTER TABLE shifts DROP COLUMN IF EXISTS cashier;
