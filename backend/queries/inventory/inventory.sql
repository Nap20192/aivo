-- name: InsertProduct :exec
INSERT INTO inventory_products (id, restaurant_id, sku, name, type, stock_unit, menu_item_id, min_stock)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ProductsByRestaurant :many
SELECT id, restaurant_id, sku, name, type, stock_unit, menu_item_id, min_stock, archived, created_at
FROM inventory_products WHERE restaurant_id = $1 ORDER BY name;

-- name: ProductByID :one
SELECT id, restaurant_id, sku, name, type, stock_unit, menu_item_id, min_stock, archived, created_at
FROM inventory_products WHERE restaurant_id = $1 AND id = $2;

-- name: ProductHasMoves :one
SELECT count(*) FROM stock_moves WHERE product_id = $1;

-- name: InsertTechCard :exec
INSERT INTO tech_cards (id, restaurant_id, product_id, valid_from, valid_to, consumption, yield_milli, created_by,
                         format, scope_note, presentation_note, storage_note, organoleptic_note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: CloseTechCard :exec
UPDATE tech_cards SET valid_to = $2 WHERE id = $1;

-- name: ActiveTechCard :one
SELECT id, restaurant_id, product_id, valid_from, valid_to, consumption, yield_milli, created_by, created_at,
       format, scope_note, presentation_note, storage_note, organoleptic_note
FROM tech_cards
WHERE restaurant_id = $1 AND product_id = $2 AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3);

-- name: TechCardLines :many
SELECT id, tech_card_id, ingredient_product_id, qty, seq, yield_permille FROM tech_card_lines
WHERE tech_card_id = $1 ORDER BY seq;

-- name: InsertRecipeCosting :exec
INSERT INTO recipe_costings (id, tech_card_id, cost_cents, method, computed_by)
VALUES ($1, $2, $3, $4, $5);

-- name: LatestCosting :one
SELECT cost_cents FROM recipe_costings WHERE tech_card_id = $1 ORDER BY computed_at DESC LIMIT 1;

-- name: InsertStockMove :exec
INSERT INTO stock_moves
    (id, restaurant_id, product_id, kind, qty, cost_cents, estimated, business_date, doc_kind, doc_id, source_event_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: LockOnHand :one
SELECT restaurant_id, product_id, qty, value_cents, last_avg_cents
FROM stock_on_hand WHERE restaurant_id = $1 AND product_id = $2 FOR UPDATE;

-- name: UpsertOnHand :exec
INSERT INTO stock_on_hand (restaurant_id, product_id, qty, value_cents, last_avg_cents, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (restaurant_id, product_id)
DO UPDATE SET qty = EXCLUDED.qty, value_cents = EXCLUDED.value_cents,
              last_avg_cents = EXCLUDED.last_avg_cents, updated_at = now();

-- name: MaxMoveDate :one
SELECT max(business_date)::date FROM stock_moves WHERE restaurant_id = $1 AND product_id = $2;

-- name: InsertSupplier :exec
INSERT INTO suppliers (id, restaurant_id, name, contacts, note) VALUES ($1, $2, $3, $4, $5);

-- name: InsertReceipt :exec
INSERT INTO goods_receipts (id, restaurant_id, supplier_id, status, business_date, note, reversal_of)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: MarkReceiptPosted :exec
UPDATE goods_receipts SET status = 'posted', posted_at = now(), posted_by = $3
WHERE restaurant_id = $1 AND id = $2 AND status = 'draft';
