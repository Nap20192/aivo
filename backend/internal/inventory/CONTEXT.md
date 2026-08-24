# Inventory

The stock, nomenclature, and recipe-costing context (increment-2): what a
restaurant holds, what things cost, and how a sale depletes them. Downstream of
menu (conformist: reads `menu_item_id` as a bare uuid) and of ledger (posts GL
through a port, like pos). Excludes purchasing/AP settlement, FIFO/lot costing,
multi-warehouse, and recursive semi-product explosion (see the anti-scope in
`docs/research/rms/impl-contract-2.md`).

## Language

**Product (nomenclature)**:
A single stock entity with a closed type — `goods` (raw), `dish` (sold, linked to
a menu item), `prepared` (in-house semi-product), `modifier` (add-on). Kept in a
base unit (`g|ml|pcs`); `kg`/`l` are compatible display units.
_Avoid_: ingredient (a role, not a type), SKU (that's the code, not the thing).

**Unit / base unit / milli-unit**:
Quantities live in the product's base unit as `int64` milli-units (1 base unit =
1000 milli; the "cents analog"). Input arrives as a display number + unit and is
converted at the boundary; cross-dimension conversion (kg for a pcs product) is
rejected.

**Tech card (recipe)**:
A calendar-versioned recipe for a dish/prepared product: an interval
`[valid_from, valid_to)`, at most one open (current) version per product, ≤1
version starting per day. A backdated create closes the version active on that
date. Recipe cycles are rejected.
_Avoid_: BOM, formula.

**Recipe costing**:
A tech card's cost is an **append-only series** (`recipe_costings`), never an
in-place field. Current cost = the latest entry. Cost = Σ line qty × ingredient
cost per base unit — moving-average for goods, recursive recipe cost for nested
prepared/dish (theoretical only).

**Consumption strategy**:
On a tech card: `assemble` (a sale depletes the recipe's ingredients, one level)
or `deplete_finished` (a sale depletes the finished product itself).

**Stock move / on-hand (weighted average)**:
`stock_moves` is the append-only book (signed qty + cost, two dates). `stock_on_hand`
is a materialized weighted-average cache (`qty`, `value_cents`, `last_avg_cents`)
that must equal the fold of the moves — updated in the same transaction under a
row lock. Correction is a mirror reversal move, never an edit. A sale against
insufficient stock is not blocked; its cost is `estimated`.

**Stock document (receipt / write-off / stocktake)**:
`draft → posted → cancelled(=reversal)`. Posting is atomic: stock moves +
`stock_on_hand` + a GL journal (via the ledger port) in one transaction.
Stocktake is server-computed with a **dry-run** (variance preview, no save) and
fixes `expected_qty` at post, not at count.

**COGS**:
Cost of goods sold, booked at ticket close (not deferred to consolidation): debit
COGS / credit Inventory, one journal per ticket, in the pos close transaction.

## Boundaries

- **inventory → ledger** (synchronous, in-process, via `adapters/ledgerbridge`
  over `ledger/app.PostInventoryJournal` / `CancelJournalForSource`): stock
  documents post GL inside their own transaction; correction is a reversal.
- **pos → inventory** (`pos/adapters/inventorybridge`): `CloseTicket` calls
  `ConsumeForSale` in its transaction. Direction is `pos → inventory → ledger`;
  no cycles. `inventory → pos` / `inventory → menu` in code are forbidden —
  `menu_item_id` is a bare uuid, and pos sales for the food-cost report are read
  through `ports.SalesReader` (a pos-side adapter), not a raw cross-context join.
- Structure: `internal/inventory/{app,ports,adapters/postgres,adapters/ledgerbridge}`
  with the domain in `internal/domain/inventory` (repo convention). Amounts are
  bigint cents, quantities bigint milli-units — single currency (§16.4).
