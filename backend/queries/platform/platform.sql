-- name: UserByEmail :one
SELECT id, org_id, email, password_hash, role, restaurant_id, created_at
FROM users WHERE email = $1;

-- name: InsertEvent :exec
INSERT INTO events (id, name, aggregate_type, aggregate_id, restaurant_id, payload)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: PendingEvents :many
SELECT id, name, aggregate_type, aggregate_id, restaurant_id, payload, occurred_at
FROM events WHERE published_at IS NULL ORDER BY occurred_at LIMIT $1;

-- name: MarkEventPublished :exec
UPDATE events SET published_at = now() WHERE id = $1;
