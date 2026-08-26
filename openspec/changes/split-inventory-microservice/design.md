## Context

See proposal.md - Why. Current state, confirmed by code inspection before this design was drafted:

- One binary (`cmd/aivo-server`), one shared `*sql.DB`, cross-context calls via `ports` interfaces wired only in `main.go` — app/domain packages never import each other directly today.
- Shared-transaction spots that a process split must not silently break: restaurant provisioning (`platform/adapters/postgres/postgres.go:44-83,260-292`) spans platform+menu+ledger+pos in one tx — **confirmed it never touches inventory**, so it's unaffected by this change. Ticket close (`pos/app/app.go:301-315`) spans pos+inventory+ledger in one tx via `ConsumeForSale` → `PostInventoryJournal`. Inventory's own document flows (`inventory/app/documents.go:218,273,349`, `stocktake.go:155`) each call ledger's `PostInventoryJournal`/`CancelJournalForSource` inside their own tx.
- Auth today is an opaque session token in an HttpOnly cookie, resolved by a DB lookup against platform (`Platform.UserByToken`) — not JWT. Fine in-process; would make every split-out service synchronously depend on platform per request if left as-is.
- Migrations are already namespaced per context (`backend/migrations/<context>/`); cross-context FKs exist (every context → platform's `restaurants`/`users`; pos → menu via `ticket_lines.menu_item_id`). None of inventory's tables FK into pos or menu, and nothing FKs into inventory — it's a clean extraction target on the schema level.
- The `events` outbox table exists (`migrations/platform/0004_events.up.sql`, sqlc queries generated) but has zero call sites — fully dead code today, matching `docs/EVENTS.md`'s own admission.
- sqlc.yaml already generates one package per context; inventory's schema block is additive over the others (cumulative FK resolution), not independent — this doesn't block a services split, it's just how sqlc resolves types today and stays as-is for `cmd/aivo-server`'s remaining contexts.

## Goals / Non-Goals

**Goals:**
- Prove the full pattern (schema-per-service, outbox+poller, gRPC via buf, JWT auth) on one real, already-money-adjacent context (inventory/COGS) before repeating it.
- Keep `cmd/aivo-server`'s remaining flows (provisioning, shift accounting) byte-for-byte behaviorally unchanged.
- Zero data loss / zero silent double-posting on the inventory↔ledger GL edge — this is accounting data (D1: append-only ledger).

**Non-Goals:**
- Splitting platform, menu, pos, or ledger into their own binaries.
- Any message broker. The poller/relay is built behind a small interface specifically so a broker can be swapped in later without touching producers or consumers, but no broker ships in this change.
- Moving `users`/credential storage out of platform.
- Solving provisioning's cross-context tx problem (out of scope — provisioning doesn't touch inventory).
- General-purpose event bus / pub-sub for arbitrary future events — only the two edges this change actually needs (`TicketClosed`, and inventory's GL-posting events) get producers/consumers. No speculative event catalog beyond what's used.

## Decisions

### D1. Schema-per-service on one shared Postgres instance, not separate DB servers
Keeps the existing FK from inventory's tables to platform's `restaurants`/`users` alive (same physical database, cross-schema FK works fine in Postgres) so this change doesn't also have to solve "how does inventory validate a restaurant_id exists without calling platform." Local dev stays one Postgres container. Inventory's tables move from the `public` schema (or wherever they live today) into an `inventory` schema with their own DB role/credentials, giving a real logical/credential boundary without the operational cost of a second database server. Promotable to a separate instance later by changing a connection string, not a redesign.
Alternative rejected: separate DB per service now — correct end-state, but forces solving the cross-schema-FK-to-platform problem immediately, which isn't this change's job.

### D2. Outbox + in-process poller, both edges, uniform (no synchronous leg)
The `events` table becomes real: producer writes a row in its own transaction (same tx as the business write, so publishing is exactly as durable as the write itself), a goroutine in the same process polls unpublished rows on an interval, and delivers each via a bespoke typed gRPC call to the consumer, marking `published_at` only on a successful (ack'd) delivery. Both the pos→inventory edge (`TicketClosed`) and *every* inventory→ledger edge (sale COGS, receipt, write-off, stocktake, and their reversals — not just the sale path, since ledger is out-of-process for inventory regardless of which document type triggered the posting) use this same mechanism. No special-cased synchronous call anywhere in the inventory↔ledger boundary, even though inventory's poller runs in a background goroutine and could technically make a safely-retryable blocking call — uniformity was chosen over the marginal latency win of a hybrid, since a hybrid means two failure modes to reason about instead of one.
Alternative rejected: synchronous HTTP/gRPC call with app-level compensation on failure — rejected because it re-couples the caller's success to the callee's availability, exactly what the split is trying to remove; also rejected a saga-orchestrator library, since two edges don't justify new orchestration machinery.

### D3. Idempotency key = existing domain document ID
Every event's key is the ID already in play (ticket ID, receipt/write-off/stocktake ID). Consumers enforce a unique constraint on `(source_document_id, event_type)` in their own write path — a repeat delivery (at-least-once) is a no-op, not a duplicate GL line. No new ID-generation scheme.

### D4. gRPC + buf, bespoke typed RPC per event, no generic envelope
`buf` handles codegen + lint + breaking-change detection in one tool (first protobuf toolchain in this repo — deliberately not hand-rolling `protoc` + two plugins). Each event/command gets its own RPC method with a typed request message (`InventoryService.HandleTicketClosed(TicketClosedRequest)`, `LedgerService.PostCOGSJournal(PostCOGSJournalRequest)`, etc.) rather than one `Deliver(bytes payload, string event_type)` RPC — matches the typed-everything direction of the enum refactor, and two real edges don't yet justify the indirection a generic envelope buys for many edges. Proto sources live at `backend/proto/<context>/v1/*.proto`; buf-generated Go code lands under each context's own package (e.g. `internal/inventory/adapters/grpc/inventoryv1`), imported only by the owning context and its direct callers — never a shared "proto" import used by unrelated contexts, to avoid rebuilding the coupling this change removes.
Alternative rejected: generic envelope RPC — smaller proto surface today, but pushes payload typing to runtime (JSON/Any unmarshal + validation on every consumer), the opposite of D-driven typed enums elsewhere in this change.

### D5. Auth: separate `cmd/aivo-auth`, thin token-minter only, Ed25519, additive
Modeled on the *shape* of github.com/GolangLessons/sso (dedicated gRPC auth service, JWT minting, `app_id`-scoped tokens) but not its ownership model — that reference project's `Register`/`Login(email,password,app_id)` owns credentials itself because it's a standalone identity provider with no other identity source. AIVO already has one (platform's `users` table + working self-registration/invite flow); duplicating credential verification into a second service would create two password-check code paths to keep in sync forever. Instead: `aivo-auth` exposes `Mint(user_id, tenant_id, roles, app_id) → token`, no password ever reaches it, and platform is its only caller — invoked after platform's existing session-cookie login already succeeded, when the frontend needs a token to call a non-platform service (inventory) directly. Ed25519 (stdlib `crypto/ed25519`, no new dependency) is asymmetric: `aivo-auth` (or platform, holding the private key — see Open Questions) signs, every downstream service ships only the public key and can verify but never mint. `app_id` is kept and mapped to AIVO's 4 real client surfaces (admin, pos, waiter, menu) so per-surface token scope/expiry is a config change later, not a second migration. Platform's existing cookie-based browser login is untouched by this change.
Alternative rejected: mirror the reference project's `Login(email,password,app_id)` RPC on `aivo-auth` itself — rejected during the design's grilling session specifically to avoid a second credential-verification path.

### D6. Typed enums: one per concept, owned by its context, no shared enum package
`type ProductType string` (etc.) with `Default()` and `Valid()`, living in the owning context's domain package (`internal/domain/inventory`, `internal/domain/pos`, ...). A shared cross-context "enums" package was considered and rejected — every context depending on one shared low-level package is exactly the kind of coupling the services split exists to remove, even though these are pure value types with no behavior.

## Risks / Trade-offs

- **[Risk] Eventual consistency on the money path.** Stock deduction and its GL entry are no longer atomic — a sale can show as consumed in inventory before its COGS journal line appears, or (transiently, until the poller retries) not at all. → Mitigation: D3's idempotency key plus at-least-once delivery with backoff guarantees the GL entry *eventually* posts exactly once; D1's append-only ledger (existing invariant) means a late-arriving entry is still correct, just late. Add a lag metric (`events` rows with `published_at IS NULL` older than N seconds) so a stuck poller is observable, not silent.
- **[Risk] Two new operational processes (`aivo-inventory`, `aivo-auth`) to keep running.** A crashed `aivo-inventory` means the admin frontend's inventory pages go down even though `aivo-server` is fine; a crashed `aivo-auth` means no new cross-service tokens can be minted (existing tokens keep working until they expire). → Mitigation: both are stateless-enough to restart cleanly (inventory's state is entirely in its own Postgres schema; auth has no state at all); docker-compose health checks and restart policies cover local/staging, documented in the compose update this change makes.
- **[Risk] `buf`/protobuf is genuinely new tooling for this repo's contributors and CI.** → Mitigation: confined to the two new services' proto contracts; existing REST-based contexts are untouched, so this isn't a repo-wide tooling migration, just a new corner of it.
- **[Trade-off] Uniform async-both-edges (D2) costs a small amount of latency on the inventory→ledger leg that a synchronous call from inventory's own consumer could have avoided.** Accepted deliberately for one failure mode instead of two — see D2.

## Migration Plan

1. Add inventory's own `events` table migration (mirrors platform's `0004_events.up.sql`), plus the outbox-poller package (shared by pos and inventory, no context-specific logic in the shared part).
2. Land the typed-enum refactor first, independent of everything else (pure refactor, easiest to review/revert in isolation, and every later step benefits from typed proto request fields matching typed Go domain types).
3. Stand up `cmd/aivo-auth` and platform's `Mint` call, additive — ships without inventory depending on it yet, verifiable in isolation (mint a token, verify it, nothing downstream consumes it yet).
4. Stand up `cmd/aivo-inventory` reading from a *copy* of the inventory schema behind a feature flag / dual-write off, prove it boots and serves its existing REST surface against the new schema before cutting traffic.
5. Wire the two outbox edges (pos→inventory, inventory→ledger), initially observed in a lower environment with both old (in-process) and new (outbox) paths behind a flag, then cut over and delete the in-process call sites.
6. Cut the admin frontend's inventory calls from `cmd/aivo-server`'s proxy (if any) to `cmd/aivo-inventory:8081` directly, switching to JWT verification.
7. Rollback strategy per step: steps 1-4 are additive and can be reverted independently; step 5's cutover is the only step that removes old code paths, so it's the one done behind a flag with both paths live briefly rather than a hard cutover.

## Open Questions

- Which process physically holds the Ed25519 private key — `aivo-auth` itself (cleanest ownership, matches "auth service signs") or platform (which would make `aivo-auth` a pure pass-through and defeats having a separate signer)? Doesn't change the spec (the `Mint` contract is identical either way) or the task breakdown (key generation/storage is one task regardless) — resolve during implementation of the `service-auth` capability.
- Exact poll interval and backoff ceiling for the outbox relay (a reasonable default like 2s/60s-cap was assumed during design) — tunable without changing the delivery contract or specs, safe to leave as an implementation default.
