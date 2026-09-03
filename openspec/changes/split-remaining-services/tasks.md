## 1. Shared foundation

- [ ] 1.1 Add an edge reverse-proxy (path-prefix routing) to `deploy/docker-compose.yml`, sitting in front of `aivo-server`/its successors and `aivo-inventory`; route `/api/v1/restaurants/{id}/inventory/*` to `aivo-inventory` and everything else to `aivo-server` for now (this also finally finishes the inventory split's frontend wiring)
- [ ] 1.2 Point every SPA's dev proxy (`frontend/{admin,pos,menu}/vite.config.ts`) and prod base URL at the reverse-proxy instead of `aivo-server` directly
- [ ] 1.3 Remove `frontend/admin/src/api/inventoryMock.ts`'s fallback now that inventory calls actually reach `aivo-inventory` through the proxy; verify inventory pages work against the real service, not the mock
- [ ] 1.4 Confirm platform's existing schema convention (check whether `migrations/platform` is already its own Postgres schema from the inventory split, or still `public`) — reuse that convention for the new services' migrations directories, don't invent a second one

## 2. Extract menu (`cmd/aivo-menu`)

- [ ] 2.1 Create `internal/menu/{app,adapters/http,adapters/postgres,adapters/grpcserver}` mirroring `internal/inventory`'s shape; move menu's existing app/adapters code out of the shared packages
- [ ] 2.2 Move `migrations/menu/*` (or split them out if not already separate) into menu's own schema, `search_path`-scoped the way `internal/inventory/adapters/postgres/schema.go` does it; keep `restaurants`' FKs from other schemas working unqualified through `search_path`
- [ ] 2.3 Add menu's `events` table + outbox poller (mirrors `migrations/inventory/0003_events.up.sql` and `pkg/outbox`)
- [ ] 2.4 Implement menu's `RestaurantCreated` consumer: create `restaurants` row + seed default menu, idempotent on restaurant ID
- [ ] 2.5 Implement menu's gRPC surface for pos's reads: menu item lookup, table lookup, service-request read/write (the eventual replacement for `menubridge`) — build it now, pos keeps using the in-process bridge until phase 4
- [ ] 2.6 Create `cmd/aivo-menu/main.go` + `deploy/Dockerfile.menu`, wire into `deploy/docker-compose.yml` (REST + gRPC ports, `DATABASE_URL`, `AUTH_PUBLIC_KEY`) and the reverse-proxy from 1.1
- [ ] 2.7 Route `/api/v1/restaurants/{id}/menu/*` (and tables/service-requests) through the proxy to `aivo-menu`; remove menu's routes from `aivo-server`
- [ ] 2.8 Write table-driven tests covering every function in menu's domain/app packages, all edge cases (per repo-wide testing standard); target 90%+ overall, 100% on domain
- [ ] 2.9 `docker compose up --build`, smoke-test: menu CRUD via admin SPA, create a restaurant and confirm menu's `RestaurantCreated` consumer provisions default menu items

## 3. Extract ledger (`cmd/aivo-ledger`)

- [ ] 3.1 Create `internal/ledger/{app,adapters/http,adapters/postgres,adapters/grpcserver}` mirroring `internal/inventory`'s shape; move `LedgerService`'s existing gRPC server implementation out of `cmd/aivo-server`
- [ ] 3.2 Move `migrations/ledger/*` into ledger's own schema, `search_path`-scoped; keep `journal_documents.created_by`/`cost_centers` FKs into `users`/`restaurants` working across schemas
- [ ] 3.3 Add ledger's `events` table + outbox poller; add a `HandleRestaurantCreated` RPC alongside the existing `PostCOGSJournal` etc.
- [ ] 3.4 Implement ledger's `RestaurantCreated` consumer: seed default accounts/cost centers/account map, idempotent on restaurant ID (reusing the seeding logic `provisioning.RestaurantProvisioner` calls today)
- [ ] 3.5 Add `HandleShiftClosed`/`HandleShiftAccepted` RPCs (or equivalent) for pos's future outbox producer to call — build now, pos keeps using in-process `ledgerbridge` until phase 4
- [ ] 3.6 Create `cmd/aivo-ledger/main.go` + `deploy/Dockerfile.ledger`, wire into `deploy/docker-compose.yml`; keep gRPC port `9080` (same address inventory's `LEDGER_GRPC_ADDR` already points at — only the hostname changes, from `aivo-server` to `aivo-ledger`)
- [ ] 3.7 Update `aivo-inventory`'s `LEDGER_GRPC_ADDR` to point at `aivo-ledger:9080`; confirm inventory's existing COGS/receipt/write-off/stocktake edge still works unchanged
- [ ] 3.8 Write table-driven tests covering every function in ledger's domain/app packages, all edge cases; target 90%+ overall, 100% on domain
- [ ] 3.9 `docker compose up --build`, smoke-test: post a receipt in inventory, confirm the GL entry lands via `aivo-ledger`; create a restaurant, confirm ledger's `RestaurantCreated` consumer provisions default accounts

## 4. Extract pos (`cmd/aivo-pos`)

- [ ] 4.1 Create `internal/pos/{app,adapters/http,adapters/postgres}` mirroring `internal/inventory`'s shape (the existing `TicketClosed` outbox producer to inventory moves as-is, unchanged)
- [ ] 4.2 Move `migrations/pos/*` into pos's own schema, `search_path`-scoped; keep `shifts.opened_by`/`cash_operations.recorded_by` FKs into `users` working across schemas
- [ ] 4.3 Drop the FK on `ticket_lines.menu_item_id`; make it a plain UUID column, no constraint (matches inventory's existing `menu_item_id` convention)
- [ ] 4.4 Replace `menubridge.Bridge` call sites with a gRPC client against `aivo-menu` (built in 2.5); remove pos's `internal/domain/menu` import, add pos-local read types for what it needs from menu
- [ ] 4.5 Replace `ledgerbridge.Bridge` call sites (shift-close, shift-accept) with an outbox producer delivering to `aivo-ledger`'s RPCs from 3.5; delete `internal/pos/adapters/ledgerbridge`
- [ ] 4.6 Implement pos's `RestaurantCreated` consumer: seed default payment methods, idempotent on restaurant ID (reuses `pospg.SeedDefaultPaymentMethods`)
- [ ] 4.7 Create `cmd/aivo-pos/main.go` + `deploy/Dockerfile.pos`, wire into `deploy/docker-compose.yml` and the reverse-proxy
- [ ] 4.8 Route pos's REST routes through the proxy to `aivo-pos`; remove them from `aivo-server`
- [ ] 4.9 Write table-driven tests covering every function in pos's domain/app packages, all edge cases; target 90%+ overall, 100% on domain
- [ ] 4.10 `docker compose up --build`, smoke-test: place an order (calls menu), close a ticket (still triggers inventory's `TicketClosed` consumer unchanged), close a shift (now posts to ledger via outbox), create a restaurant and confirm pos's `RestaurantCreated` consumer provisions default payment methods

## 5. Extract platform (`cmd/aivo-platform`) and cut over provisioning

- [ ] 5.1 Create `internal/platform/{app,adapters/http,adapters/postgres}` mirroring `internal/inventory`'s shape; move the session-cookie login flow, `authclient`/`TokenMinter` wiring, and org/restaurant CRUD as-is
- [ ] 5.2 Add platform's `events` table + outbox poller; publish `RestaurantCreated` (restaurant ID, organization ID) in the same transaction as `CreateOrgWithOwner`/`CreateRestaurant`
- [ ] 5.3 Delete `internal/provisioning`'s shared-transaction hook and its call sites in platform's postgres adapter and `cmd/aivo-seed/main.go`; `cmd/aivo-seed` now waits for (or explicitly triggers and polls) the saga to complete before seeding further, or seeds each domain directly against its own service — pick one and document it in the seed tool's doc comment
- [ ] 5.4 Create `cmd/aivo-platform/main.go` (serves the admin SPA, same as `aivo-server` does today) + `deploy/Dockerfile.platform`, wire into `deploy/docker-compose.yml` and the reverse-proxy as the default route
- [ ] 5.5 Retire `cmd/aivo-server`: confirm every route/RPC it served now lives in `aivo-platform`/`aivo-menu`/`aivo-ledger`/`aivo-pos`, then delete the binary and its now-empty shared `internal` packages
- [ ] 5.6 Write table-driven tests covering every function in platform's domain/app packages, all edge cases; target 90%+ overall, 100% on domain
- [ ] 5.7 `docker compose up --build`, full end-to-end smoke test: sign up an organization, create a restaurant, confirm menu/ledger/pos all provision their default data via the saga, then run a full order-to-GL flow (place order → close ticket → inventory consumes → close shift → ledger consumes)

## 6. Cleanup and documentation

- [ ] 6.1 Update `AGENTS.md`'s command list with `go run -C backend ./cmd/aivo-{menu,ledger,pos,platform}` and the new compose service names
- [ ] 6.2 Update `docs/EVENTS.md` with `RestaurantCreated` and the two new outbox edges (pos→ledger, pos→menu)
- [ ] 6.3 Update `docs/STACK.md` if the reverse-proxy introduces a new pinned dependency (e.g. an nginx/Caddy image tag)
- [ ] 6.4 Run `go build -C backend ./... && go test -C backend ./...` and confirm repo-wide coverage still meets the 90%/100%-domain bar with the four new service packages included
- [ ] 6.5 Confirm no remaining references to the retired `cmd/aivo-server` in docs, compose, or CI
