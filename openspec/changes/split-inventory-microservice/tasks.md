## 1. Typed-enum refactor (independent of everything else — land first)

- [ ] 1.1 Inventory context: `ProductType` (default `goods`), `TechCardFormat` (default `simple`), `ConsumeStrategy`, `CostMethod` — replace existing string consts with typed enums + `Default()`/`Valid()`, update all call sites and sqlc-generated scan/insert code
- [ ] 1.2 Ledger context: typed enums for journal document status, account type, and any other string-const groups in `internal/domain/ledger`
- [ ] 1.3 Pos context: typed enums for `DocStatus` (draft/posted/cancelled) and ticket/shift state constants in `internal/domain/pos`
- [ ] 1.4 Platform + menu contexts: typed enums for any remaining string-const groups (roles, menu item status, etc.)
- [ ] 1.5 Table-driven tests per new enum type: `Valid()` true/false cases including empty string and unknown values, `Default()` returns the documented default
- [ ] 1.6 `go build ./... && go vet ./... && go test ./...` green; no behavior change — existing integration tests must still pass unmodified

## 2. Outbox/events infrastructure (service-events capability)

- [ ] 2.1 Migration: `backend/migrations/inventory/000X_events.up.sql`/`.down.sql` mirroring platform's `events` table shape
- [ ] 2.2 Shared outbox package (e.g. `backend/pkg/outbox`): `Publish(ctx, tx, event)` helper for producers to call inside their own transaction; delivery interface (`Deliverer`) so the transport is swappable later
- [ ] 2.3 In-process poller: scans unpublished rows on an interval, calls the configured `Deliverer`, marks `published_at` on ack, retries with exponential backoff (capped) on failure
- [ ] 2.4 Idempotency: consumer-side unique constraint on `(source_document_id, event_type)` per consuming table; helper to make a handler's write path a no-op on conflict
- [ ] 2.5 Tests: publish-then-rollback leaves no event row; poller delivers and marks published on ack; poller retries and does not mark published on failure; redelivery of the same idempotency key is a no-op on the consumer side; poller survives a consumer that's down for N polls then recovers

## 3. gRPC/buf scaffolding (shared by sections 4 and 5)

- [ ] 3.1 Add `buf` config (`buf.yaml`, `buf.gen.yaml`) at `backend/proto/`; wire `buf generate` into the repo's build docs
- [ ] 3.2 `backend/proto/inventory/v1/inventory.proto`: `InventoryService.HandleTicketClosed(TicketClosedRequest) returns (HandleTicketClosedResponse)`
- [ ] 3.3 `backend/proto/ledger/v1/ledger.proto`: `LedgerService.PostCOGSJournal`, `PostReceiptJournal`, `PostWriteOffJournal`, `PostStocktakeJournal` (and their reversal equivalents) request/response messages
- [ ] 3.4 `backend/proto/auth/v1/auth.proto`: `AuthService.Mint(MintRequest) returns (MintResponse)`
- [ ] 3.5 Generate Go code via `buf generate`; commit generated code under each context's own adapter package (not a shared proto-consumer package)
- [ ] 3.6 Add a Makefile/script target for regenerating proto code, documented in AGENTS.md commands section

## 4. `cmd/aivo-auth` (service-auth capability)

- [ ] 4.1 Ed25519 keypair generation/loading (env var or file path for the private key; public key distributed to verifying services)
- [ ] 4.2 `Mint` gRPC handler: builds and signs a JWT with claims (user_id, tenant_id, roles, app_id, exp, iss) from the request; rejects any request shape that isn't the narrow `Mint` signature — no credential field exists to accept
- [ ] 4.3 App-ID registry for the 4 client surfaces (admin, pos, waiter, menu) with per-surface default expiry
- [ ] 4.4 `cmd/aivo-auth/main.go`: starts the gRPC listener, reads its port from env, no HTTP/REST surface, no database
- [ ] 4.5 Platform wiring: after existing session-cookie login succeeds, platform calls `Mint` and returns the token to the frontend alongside/instead of when it needs to call a non-platform service
- [ ] 4.6 Shared JWT verification helper package (importable by inventory and future services): verify signature + expiry + issuer, return typed claims
- [ ] 4.7 Tests: mint produces a token verifiable with the public key; tampered token fails verification; expired token fails verification; token minted for one tenant is rejected when checked against another tenant's resource; each app_id gets its documented default expiry

## 5. `cmd/aivo-inventory` extraction (inventory-service capability)

- [ ] 5.1 New binary `backend/cmd/aivo-inventory/main.go`: own config (DB DSN pointing at the `inventory` schema, REST port, gRPC port, auth public key path)
- [ ] 5.2 Migration: move inventory's tables into their own `inventory` schema (or provision the schema and re-point existing inventory migrations at it) on the shared Postgres instance; grant inventory's own DB role only what it needs
- [ ] 5.3 Move/mount inventory's existing REST handlers onto `cmd/aivo-inventory`'s own mux at `:8081`, unchanged endpoint shapes
- [ ] 5.4 Replace inventory's session-cookie auth middleware with JWT verification (using the section 4.6 helper) on all inventory REST routes
- [ ] 5.5 gRPC server on `:9081`: implements `InventoryService.HandleTicketClosed`, applying stock consumption idempotently (section 2.4) and publishing inventory's own outbox event for the resulting COGS posting
- [ ] 5.6 gRPC client from inventory to ledger's `:9080` (used by inventory's outbox `Deliverer` for all inventory→ledger events)
- [ ] 5.7 Remove inventory's old in-process `ledgerbridge` port/adapter and pos's old in-process `inventorybridge` port/adapter once the outbox path is proven (see section 6)
- [ ] 5.8 Tests: each inventory→ledger flow (sale COGS, receipt, write-off, stocktake, reversals) publishes the correct event shape; REST endpoints reject requests with no/invalid/expired/wrong-tenant tokens; REST endpoints behave identically to today given a valid token (regression coverage over the existing integration test suite, re-pointed at the new binary)

## 6. `cmd/aivo-server` side of the split

- [ ] 6.1 Add `:9080` gRPC listener to `cmd/aivo-server`, implementing `LedgerService.Post*Journal` methods, consuming inventory's outbox events idempotently
- [ ] 6.2 `pos/app/app.go` `CloseTicket`: replace the in-process `inventorybridge.ConsumeForSale` call with publishing a `TicketClosed` outbox event in the same transaction as ticket close
- [ ] 6.3 gRPC client from `cmd/aivo-server` to inventory's `:9081` (used by pos's outbox `Deliverer`)
- [ ] 6.4 Tests: ticket close commits and returns success even when inventory is unreachable (event stays pending); once inventory becomes reachable, a previously-pending `TicketClosed` event is processed and stock/COGS reflect it correctly

## 7. Cutover and local dev

- [ ] 7.1 `deploy/docker-compose.yml`: add `aivo-inventory` and `aivo-auth` services, each on their documented ports, against the existing Postgres container; health checks and restart policies for both
- [ ] 7.2 Point admin frontend's inventory API calls at `cmd/aivo-inventory:8081` directly instead of through `cmd/aivo-server`
- [ ] 7.3 Full integration test pass across all three binaries running together (docker-compose up, exercise: login → mint token → create receipt on inventory → verify GL entry appears on ledger → close a POS ticket → verify stock deducted and COGS posted)
- [ ] 7.4 Update `AGENTS.md` / `CONTEXT-MAP.md` / `docs/EVENTS.md` to reflect the new service topology, ports, and the now-live event catalog entries
- [ ] 7.5 Delete now-dead in-process bridge code (`inventory/adapters/ledgerbridge`, `pos/adapters/inventorybridge`) once section 6/7.3 prove the new paths end-to-end
