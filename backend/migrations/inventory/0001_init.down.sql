-- Rollback of inventory 0001 (manual use only). Drop in FK order.
DROP TABLE IF EXISTS stocktake_lines;
DROP TABLE IF EXISTS stocktakes;
DROP TABLE IF EXISTS write_off_lines;
DROP TABLE IF EXISTS write_offs;
DROP TABLE IF EXISTS goods_receipt_lines;
DROP TABLE IF EXISTS goods_receipts;
DROP TABLE IF EXISTS stock_on_hand;
DROP TABLE IF EXISTS stock_moves;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS recipe_costings;
DROP TABLE IF EXISTS tech_card_lines;
DROP TABLE IF EXISTS tech_cards;
DROP TABLE IF EXISTS inventory_products;
