-- Backs the CRM grouped visits/spend aggregation
-- (orders WHERE restaurant_id = $1 AND customer_id = ANY(...)).
CREATE INDEX orders_restaurant_customer_idx
    ON orders (restaurant_id, customer_id)
    WHERE customer_id IS NOT NULL;
