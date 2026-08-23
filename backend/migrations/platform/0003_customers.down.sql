ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_customer_id_fkey;
DROP TABLE IF EXISTS guest_profiles, customer_sessions, customers CASCADE;
