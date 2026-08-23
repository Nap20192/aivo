-- name: RestaurantBySlug :one
SELECT id, slug, name FROM restaurants WHERE slug = $1;

-- name: MenusByRestaurant :many
SELECT id, restaurant_id, slug, name, position, is_default
FROM menus WHERE restaurant_id = $1 ORDER BY position;

-- name: CategoriesByMenu :many
SELECT id, restaurant_id, menu_id, name, position
FROM categories WHERE menu_id = $1 ORDER BY position;

-- name: ItemsByCategory :many
SELECT id, category_id, name, description, price_cents, image_url, available
FROM menu_items WHERE category_id = $1;
