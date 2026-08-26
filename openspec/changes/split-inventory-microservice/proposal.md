## Why

The backend is one binary (`cmd/aivo-server`) wiring all 5 bounded contexts (platform, menu, pos, ledger, inventory) against one shared Postgres pool, with in-process calls between contexts and several places relying on one DB transaction spanning multiple contexts (e.g. ticket-close chains pos→inventory→ledger). This blocks independent deployment/scaling of any one context and couples every context's uptime to every other's. We're extracting inventory as a pilot to prove a real (not modular-monolith) microservices split — outbox-based eventing, gRPC between services, and a dedicated token-minting auth service — before repeating the pattern on the remaining contexts. Alongside this, the domain layer's untyped string constants (nomenclature type, tech-card format, document status, etc.) become typed enums, since the split makes each context's public contract (proto messages, REST DTOs) a good forcing function to stop passing bare strings across boundaries.

## What Changes

- Re-wire the existing (currently dead) `events` outbox table into a real outbox pattern: each producing service polls its own schema's `events` table in-process and delivers pending events to the consumer over gRPC, with retry/backoff and idempotency keyed on the existing domain document ID. **BREAKING**: `internal/pos/ports` and `internal/inventory/ports` lose their in-process `ledger`/`inventory` call interfaces — pos→inventory (ticket close) and *every* inventory→ledger GL posting (sale COGS, receipts, write-offs, stocktake, and their reversals) become asynchronous, since ledger stops being in-process for inventory the moment inventory is its own binary.
- Extract inventory into a new binary, `cmd/aivo-inventory`, same Go module/monorepo, own Postgres schema on the existing shared instance, own REST port (`:8081`, same admin-frontend-facing API surface as today) and own gRPC port (`:9081`, receives `TicketClosed` from pos, calls out to ledger's new gRPC port `:9080` to post COGS and all other inventory-originated GL entries). `cmd/aivo-server` keeps platform+menu+pos+ledger and gains a `:9080` gRPC listener for ledger's inbound side.
- Add `cmd/aivo-auth`, a new thin token-minting service: `Mint(user_id, tenant_id, roles, app_id) → JWT`, Ed25519-signed (stdlib `crypto/ed25519`), scoped by `app_id` to AIVO's 4 client surfaces (admin, pos, waiter, menu). It owns no credentials — platform's existing session-cookie login/registration/invite flow is unchanged and is the only caller of `Mint`. Inventory's REST handlers verify this JWT locally (public key) instead of the in-process session lookup they'd otherwise need.
- Introduce gRPC + `buf` (codegen/lint/breaking-change detection) as the wire protocol and tooling for all new inter-service calls, with bespoke typed RPCs per event/command (no generic envelope RPC). Proto sources under `backend/proto/<context>/v1/*.proto`.
- Replace untyped string constants across all 5 contexts' domain packages with typed string enums (e.g. `ProductType`, `TechCardFormat`, `DocStatus`, `ConsumeStrategy`, `CostMethod`), each owned by its context, each with a `Default()` and `Valid()`. Pure refactor, no behavior change — not tracked as a spec capability.
- Update `deploy/docker-compose.yml` and local-dev docs for 3 binaries (`aivo-server`, `aivo-inventory`, `aivo-auth`) on distinct ports against one Postgres container.

Explicitly out of scope: splitting platform/menu/pos/ledger into their own binaries; any message broker (Kafka etc.); moving `users`/credentials out of platform; changing restaurant provisioning (confirmed it never touches inventory today); separate Postgres instances per service.

## Capabilities

### New Capabilities
- `inventory-service`: inventory as an independently deployable service — its REST API surface (unchanged endpoints, now JWT-authenticated), its gRPC inbound (`TicketClosed` from pos) and outbound (COGS posting to ledger) contracts, and its own outbox producer for the ledger edge.
- `service-events`: the outbox/eventing infrastructure shared by producing services — event persistence, idempotency-key semantics, in-process poller/relay behavior, delivery retry/backoff, and the pos→inventory `TicketClosed` event contract specifically.
- `service-auth`: the `aivo-auth` token-minting service — `Mint` RPC contract, JWT claim shape, Ed25519 signing/verification, `app_id` scoping to the 4 client surfaces, and how downstream services (inventory) verify tokens.

### Modified Capabilities
(none — no existing `openspec/specs/` capabilities predate this change)

## Impact

- New binaries: `backend/cmd/aivo-inventory`, `backend/cmd/aivo-auth`.
- New packages: `backend/proto/**` (buf-managed), an outbox poller/relay package (shared by pos and inventory), inventory's gRPC server/client adapters, auth's Ed25519 signer + JWT verification helper (importable by inventory).
- Modified: `internal/pos/app/app.go` (`CloseTicket` publishes an outbox event instead of calling inventory in-process), `internal/inventory/app/*` (consumes `TicketClosed`; `cogs.go`, `documents.go`, `stocktake.go` all publish an outbox event instead of calling ledger's `PostInventoryJournal`/`CancelJournalForSource` in-process — every inventory→ledger GL posting crosses the new service boundary once ledger lives in `cmd/aivo-server` and inventory doesn't, not just the sale-COGS path), `internal/platform/adapters/http/auth.go` (calls `aivo-auth`'s `Mint` after session login succeeds), inventory's REST handlers (JWT verification), `deploy/docker-compose.yml`, `backend/migrations/inventory/` (new `events` table, mirroring platform's), all 5 contexts' domain packages (typed-enum refactor).
- New dependencies: `buf` (dev/build tooling only), `google.golang.org/grpc` + `google.golang.org/protobuf` (runtime).
- Provisioning and ledger's own flows (shift acceptance) are unaffected — pos still calls ledger in-process for those since both stay inside `cmd/aivo-server`.
