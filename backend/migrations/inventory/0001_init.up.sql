-- Inventory context (increment-2): nomenclature, calendar-versioned tech
-- cards with an append-only cost series, a perpetual weighted-average stock
-- ledger, and stock documents (receipt / write-off / stocktake). Quantities
-- are bigint milli-units of the product's base unit (the "cents analog" —
-- domain.md §0); amounts are bigint cents -- single currency (company base);
-- multicurrency deferred (reference §16.4).

-- Nomenclature. menu_item_id is a bare uuid (conformist to menu, no FK).
CREATE TABLE inventory_products (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    sku           text NOT NULL,
    name          text NOT NULL,
    type          text NOT NULL, -- goods|dish|prepared|modifier
    stock_unit    text NOT NULL, -- g|ml|pcs (base unit)
    menu_item_id  uuid,          -- only for dish; no FK (cross-context)
    min_stock     bigint,        -- milli-units, low-stock alert threshold
    archived      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX inventory_products_sku_idx ON inventory_products (restaurant_id, sku);
CREATE INDEX inventory_products_restaurant_idx ON inventory_products (restaurant_id);
-- At most one dish product per menu item.
CREATE UNIQUE INDEX inventory_products_menu_item_idx
    ON inventory_products (restaurant_id, menu_item_id)
    WHERE menu_item_id IS NOT NULL;

-- Calendar-versioned tech cards (D5). Interval [valid_from, valid_to);
-- open version has valid_to NULL.
CREATE TABLE tech_cards (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    product_id    uuid NOT NULL REFERENCES inventory_products (id) ON DELETE CASCADE,
    valid_from    date NOT NULL,
    valid_to      date,
    consumption   text NOT NULL, -- assemble|deplete_finished
    yield_milli   bigint NOT NULL DEFAULT 1000,
    created_by    uuid NOT NULL REFERENCES users (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tech_cards_product_from_idx ON tech_cards (restaurant_id, product_id, valid_from);
-- At most one open (current) version per product.
CREATE UNIQUE INDEX tech_cards_open_per_product_idx
    ON tech_cards (restaurant_id, product_id)
    WHERE valid_to IS NULL;

CREATE TABLE tech_card_lines (
    id                    uuid PRIMARY KEY,
    tech_card_id          uuid NOT NULL REFERENCES tech_cards (id) ON DELETE CASCADE,
    ingredient_product_id uuid NOT NULL REFERENCES inventory_products (id),
    qty                   bigint NOT NULL, -- milli-units, gross
    seq                   integer NOT NULL,
    CONSTRAINT tech_card_lines_qty_positive CHECK (qty > 0)
);

CREATE UNIQUE INDEX tech_card_lines_ingredient_idx ON tech_card_lines (tech_card_id, ingredient_product_id);
CREATE INDEX tech_card_lines_card_idx ON tech_card_lines (tech_card_id);

-- Append-only recipe cost series (Domain 3): current cost = latest entry.
CREATE TABLE recipe_costings (
    id           uuid PRIMARY KEY,
    tech_card_id uuid NOT NULL REFERENCES tech_cards (id) ON DELETE CASCADE,
    cost_cents   bigint NOT NULL, -- single currency (§16.4)
    method       text NOT NULL DEFAULT 'weighted_avg',
    computed_at  timestamptz NOT NULL DEFAULT now(),
    computed_by  uuid NOT NULL REFERENCES users (id)
);

CREATE INDEX recipe_costings_card_idx ON recipe_costings (tech_card_id, computed_at);

-- Minimal supplier reference.
CREATE TABLE suppliers (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    name          text NOT NULL,
    contacts      jsonb NOT NULL DEFAULT '{}'::jsonb,
    note          text NOT NULL DEFAULT '',
    archived      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX suppliers_name_idx ON suppliers (restaurant_id, lower(name));

-- Perpetual append-only stock book (D2). qty and cost are signed.
CREATE TABLE stock_moves (
    id              uuid PRIMARY KEY,
    restaurant_id   uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    product_id      uuid NOT NULL REFERENCES inventory_products (id),
    kind            text NOT NULL, -- receipt|sale|writeoff|stocktake_surplus|stocktake_shortage|reversal
    qty             bigint NOT NULL, -- signed milli-units
    cost_cents      bigint NOT NULL, -- signed cents (§16.4)
    estimated       boolean NOT NULL DEFAULT false,
    business_date   date NOT NULL,   -- D7 business date
    recorded_at     timestamptz NOT NULL DEFAULT now(), -- D7 record time
    doc_kind        text NOT NULL,
    doc_id          uuid NOT NULL,
    source_event_id uuid, -- sale idempotency (nullable)
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX stock_moves_product_date_idx ON stock_moves (restaurant_id, product_id, business_date);
CREATE INDEX stock_moves_doc_idx ON stock_moves (doc_kind, doc_id);
CREATE UNIQUE INDEX stock_moves_source_event_idx ON stock_moves (source_event_id) WHERE source_event_id IS NOT NULL;

-- Materialized weighted-average position (cache; source of truth = moves).
CREATE TABLE stock_on_hand (
    restaurant_id  uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    product_id     uuid NOT NULL REFERENCES inventory_products (id) ON DELETE CASCADE,
    qty            bigint NOT NULL DEFAULT 0, -- signed milli-units
    value_cents    bigint NOT NULL DEFAULT 0, -- value of stock on hand
    last_avg_cents bigint NOT NULL DEFAULT 0, -- avg per base unit at last positive qty
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (restaurant_id, product_id)
);

-- Stock documents (D4). status draft → posted → cancelled.
CREATE TABLE goods_receipts (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    supplier_id   uuid REFERENCES suppliers (id),
    status        text NOT NULL DEFAULT 'draft',
    business_date date NOT NULL,
    note          text NOT NULL DEFAULT '',
    posted_at     timestamptz,
    posted_by     uuid REFERENCES users (id),
    reversal_of   uuid REFERENCES goods_receipts (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX goods_receipts_restaurant_date_idx ON goods_receipts (restaurant_id, business_date);

CREATE TABLE goods_receipt_lines (
    id               uuid PRIMARY KEY,
    receipt_id       uuid NOT NULL REFERENCES goods_receipts (id) ON DELETE CASCADE,
    product_id       uuid NOT NULL REFERENCES inventory_products (id),
    qty_base_milli   bigint NOT NULL,
    input_unit       text NOT NULL,
    unit_price_cents bigint NOT NULL, -- per input unit (§16.4)
    line_cost_cents  bigint NOT NULL,
    seq              integer NOT NULL
);

CREATE INDEX goods_receipt_lines_receipt_idx ON goods_receipt_lines (receipt_id);

CREATE TABLE write_offs (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    reason        text NOT NULL,
    status        text NOT NULL DEFAULT 'draft',
    business_date date NOT NULL,
    note          text NOT NULL DEFAULT '',
    posted_at     timestamptz,
    posted_by     uuid REFERENCES users (id),
    reversal_of   uuid REFERENCES write_offs (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX write_offs_restaurant_date_idx ON write_offs (restaurant_id, business_date);

CREATE TABLE write_off_lines (
    id             uuid PRIMARY KEY,
    write_off_id   uuid NOT NULL REFERENCES write_offs (id) ON DELETE CASCADE,
    product_id     uuid NOT NULL REFERENCES inventory_products (id),
    qty_base_milli bigint NOT NULL,
    input_unit     text NOT NULL,
    seq            integer NOT NULL
);

CREATE INDEX write_off_lines_write_off_idx ON write_off_lines (write_off_id);

CREATE TABLE stocktakes (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    status        text NOT NULL DEFAULT 'draft',
    business_date date NOT NULL,
    note          text NOT NULL DEFAULT '',
    posted_at     timestamptz,
    posted_by     uuid REFERENCES users (id),
    reversal_of   uuid REFERENCES stocktakes (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- At most one open (draft) stocktake per restaurant.
CREATE UNIQUE INDEX stocktakes_open_per_restaurant_idx
    ON stocktakes (restaurant_id)
    WHERE status = 'draft';

CREATE TABLE stocktake_lines (
    id                  uuid PRIMARY KEY,
    stocktake_id        uuid NOT NULL REFERENCES stocktakes (id) ON DELETE CASCADE,
    product_id          uuid NOT NULL REFERENCES inventory_products (id),
    counted_qty_milli   bigint NOT NULL,
    expected_qty_milli  bigint, -- fixed at post
    variance_qty_milli  bigint NOT NULL DEFAULT 0,
    variance_cost_cents bigint NOT NULL DEFAULT 0,
    seq                 integer NOT NULL
);

CREATE INDEX stocktake_lines_stocktake_idx ON stocktake_lines (stocktake_id);
