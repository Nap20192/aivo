-- name: OpenShiftByRestaurant :one
SELECT id, restaurant_id, opened_by, opened_at, opening_float_cents, cashier
FROM shifts WHERE restaurant_id = $1 AND closed_at IS NULL;

-- name: OpenTickets :many
SELECT id, shift_id, table_id, status, note, created_at
FROM tickets WHERE restaurant_id = $1 AND status = 'open' ORDER BY created_at;
