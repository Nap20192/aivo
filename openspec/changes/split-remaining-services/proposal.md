## Why

Inventory was split out as the pilot for AIVO's microservices migration (schema-per-service on shared Postgres, gRPC+outbox for cross-service edges, Ed25519 tokens for auth). `cmd/aivo-server` still bundles the other four business domains — platform, ledger, pos, menu — as one binary. Finishing the split lets each domain be deployed, scaled, and iterated on independently, and removes the last in-process couplings (a shared restaurant-provisioning transaction, pos's direct import of menu's domain types, a ledger call pos makes in-process) that block that independence.

## What Changes

- **BREAKING**: `platform`, `ledger`, `pos`, and `menu` each become their own binary (`cmd/aivo-platform`, `cmd/aivo-ledger`, `cmd/aivo-pos`, `cmd/aivo-menu`), each on its own REST/gRPC ports, each with its own Postgres schema (same instance, same cross-schema-FK pattern inventory already uses for `restaurants`/`users`).
- Restaurant provisioning (today one DB transaction writing menu, platform, ledger, and pos tables together) becomes a saga: platform commits its own write and publishes a `RestaurantCreated` outbox event; menu, ledger, and pos each provision their own default rows asynchronously off that event, the same at-least-once/idempotent delivery pattern already specified for `TicketClosed`/COGS posting.
- pos's in-process `ledgerbridge` (shift-close/shift-accept journal calls) is replaced by outbox events to `aivo-ledger`, mirroring how inventory already talks to ledger.
- pos's in-process `menubridge` (menu item/table/service-request reads) becomes gRPC calls to `aivo-menu`; pos's app layer stops importing menu's domain package (`internal/domain/menu`) directly and gets its own local read types.
- pos's `ticket_lines.menu_item_id` FK into menu's `menu_items` table is dropped in favor of the same "bare UUID, no FK, cross-context" convention inventory already uses for `menu_item_id`.
- `restaurants` (owned by menu) and `users`/`sessions` (owned by platform) stay where they are — every other domain keeps referencing them via cross-schema foreign keys through `search_path`, not by moving the tables.
- Platform remains the sole caller of `aivo-auth`'s `Mint` RPC and the only service serving the existing session-cookie login; that contract (`service-auth`) does not change.
- Rollout is phased, one domain extracted at a time, in dependency order: **menu** first (only inbound dependencies once its FK from pos is dropped), then **ledger** (already gRPC-exposed as `LedgerService`, only inbound dependents), then **pos** (depends on menu+ledger, both already split by this point), then **platform** last (stays as the provisioning-saga initiator and auth-issuing caller throughout, so it's safest to extract once everything downstream already speaks the new contracts).

## Capabilities

### New Capabilities
- `menu-service`: menu's REST surface, own schema, and its side of the provisioning saga, as an independently deployable service.
- `ledger-service`: ledger's REST/gRPC surface (`LedgerService` already exists, now the whole domain moves), own schema, and its side of the provisioning saga.
- `pos-service`: pos's REST surface, own schema, `TicketClosed` outbox producer (unchanged from the inventory split), new outbox-based edges to ledger and menu, and its side of the provisioning saga.
- `platform-service`: platform's REST surface (org/user/session/provisioning), own schema, as the initiator of the restaurant-provisioning saga and the sole `aivo-auth` caller.

### Modified Capabilities
- `service-events`: adds the restaurant-provisioning saga (`RestaurantCreated` event, consumed by menu/ledger/pos) and the new pos→ledger, pos→menu outbox edges, alongside the existing `TicketClosed` and inventory→ledger requirements.

## Impact

- **Code**: new `cmd/aivo-platform`, `cmd/aivo-ledger`, `cmd/aivo-pos`, `cmd/aivo-menu` binaries; `internal/{platform,ledger,pos,menu}` restructured into per-service `app`/`adapters` packages (mirroring `internal/inventory`'s shape); `internal/provisioning` becomes an outbox-driven saga instead of a shared-transaction hook; `ledgerbridge`/`menubridge` deleted, replaced by gRPC clients + outbox deliverers.
- **Data**: new migrations directories `migrations/{ledger,pos,menu}` moved into their own Postgres schemas (platform's `migrations/platform` likely already schema-scoped from the inventory split's convention — confirm during design); `ticket_lines.menu_item_id` FK dropped.
- **Deploy**: `deploy/docker-compose.yml` gains four more service blocks and Dockerfiles, following the `Dockerfile.inventory`/`Dockerfile.auth` pattern; each new service needs `AUTH_PUBLIC_KEY` wiring like inventory does.
- **Frontends**: admin/pos/menu SPAs' API base URLs change from one `aivo-server` origin to per-service origins (or stay behind a shared reverse-proxy/gateway — open question for design.md).
- **No change**: `aivo-inventory` and `aivo-auth`, already split, are unaffected consumers/producers on the new edges (inventory keeps talking to whichever binary now serves `LedgerService`).
