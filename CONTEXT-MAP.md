# Context Map

## Contexts

- [Menu](./backend/internal/menu/CONTEXT.md) — diner-facing digital menu, ordering, and landing page for a single restaurant
- POS (`backend/internal/pos`) — cash shifts, per-table tickets, tenders/payments, in-shift cash movements, and shift acceptance (D6). Reads Menu through an in-process bridge.
- [Ledger](./backend/internal/ledger/CONTEXT.md) — append-only, double-entry GL core (chart of accounts, journal documents, GL-semantics config). Consumed by POS and Inventory.
- [Inventory](./backend/internal/inventory/CONTEXT.md) — nomenclature, calendar-versioned tech cards + costing, perpetual weighted-average stock ledger, stock documents (receipt/write-off/stocktake), COGS on sale. Downstream of Menu; posts to Ledger; consumed by POS at ticket close.

## Relationships

- **POS → Inventory → Ledger** (synchronous, in-process, increment-2): at ticket
  close POS calls `inventory.ConsumeForSale` (via `pos/adapters/inventorybridge`)
  in the same transaction; inventory depletes stock by the active tech card and
  posts a COGS journal to the ledger (via `inventory/adapters/ledgerbridge`).
  Inventory stock documents (receipt/write-off/stocktake) also post GL through
  that bridge. The chain is `pos → inventory → ledger` — acyclic; `inventory → pos`
  and `inventory → menu` in code are forbidden (`menu_item_id` is a bare uuid,
  pos sales are read through a `SalesReader` port for the food-cost report).
- **POS → Ledger** (synchronous, in-process): on shift close POS builds a draft
  acceptance journal, and on acceptance posts it, through `pos/ports.Ledger`
  (implemented by `pos/adapters/ledgerbridge` over `ledger/app`). Both writes run
  in one Postgres transaction (documented cross-context `*sql.Tx`; the whole
  backend is one module/database). The variance between declared and expected
  cash is a mandatory GL posting (`cash_over_short`), not a soft field; the GL
  account each tender lands on is per-restaurant config (`ledger_account_map`).
- **POS → Menu** (in-process bridge): item lookups for line snapshots, tables for
  the floor view, the service-request inbox. POS never mutates Menu orders — the
  POS/Menu-order decoupling stands (`backend/internal/menu/docs/adr/0002-menu-order-decoupled-from-pos.md`).

## Code layout

All satellite services share one Go module at the repo root (module `aivo`) rather than one module per service — see `README.md`. One command per binary under `cmd/<service>-<binary>/`, one domain-scoped package tree per service under `backend/internal/<service>/`, static frontends under `frontend/<service>/`. A service's context docs (`CONTEXT.md`, `docs/adr/`) live beside its code at `backend/internal/<service>/`.

Domain model lives apart from the services: `backend/internal/sharedkernel/` holds the DDD building blocks shared by every context (ID, Entity, AggregateRoot, DomainEvent), and `backend/internal/domain/{platform,menu,pos}/` holds each context's business entities. Contexts (`backend/internal/<service>/{app,ports,adapters}`) import their domain package; domain packages import only `backend/internal/sharedkernel` and the standard library.
