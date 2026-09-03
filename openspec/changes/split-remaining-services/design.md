## Context

See proposal.md - Why. Current state, confirmed by reading the code before writing this design:

- **Provisioning is one shared transaction today.** `provisioning.RestaurantProvisioner` (`internal/provisioning/provisioning.go`) is invoked inside platform's own `*sql.Tx` in `internal/platform/adapters/postgres/postgres.go` (`CreateOrgWithOwner`, `CreateRestaurant`). In that single transaction: menu's `restaurants` row + default menu is inserted, platform's `organizations`/`users`/`subscriptions` are inserted, then the hook seeds ledger's `accounts`/`cost_centers`/`account_map` and pos's `payment_methods`.
- **Two in-process bridges exist inside pos**: `ledgerbridge.Bridge` (shift-close/shift-accept journal calls, called with pos's own tx) and `menubridge.Bridge` (menu item/table/service-request reads, plain Go calls). pos's `app` package also imports menu's domain types directly (`internal/domain/menu`), not just through the bridge interface.
- **Every domain has hard FKs into `restaurants` (owned by menu) and `users` (owned by platform)**, including already-split inventory, which resolves these through `search_path` on the same shared Postgres instance rather than breaking them.
- **The inventory split's frontend wiring was never finished.** Despite `aivo-inventory` running on its own origin (`:8081`) since the prior change, `frontend/admin/src/api/client.ts` still calls `/api/v1/restaurants/{id}/inventory/...` against the single `:8080` origin (falling back to a mock), and no service has CORS middleware. This proposal's migration plan has to actually finish that wiring, for inventory as well as the four new services, or none of the split services are reachable from a browser.

## Goals / Non-Goals

**Goals:**
- Same behavior-preservation bar as the inventory split: every existing endpoint keeps its request/response shape.
- One consistent pattern for the provisioning saga, reused across all three new consumers (menu, ledger, pos) rather than three bespoke mechanisms.
- Resolve the frontend-reachability gap left by the inventory split, for all five split services at once, not just the four new ones.

**Non-Goals:**
- Moving `restaurants` or `users` to a different owner (decided: leave with menu/platform respectively).
- A message broker (Kafka etc.) — stays deferred, as it was for the inventory split; outbox+gRPC is sufficient at this scale.
- Physically separate Postgres instances per service — stays one shared instance, schema-per-service.
- Redesigning `aivo-inventory` or `aivo-auth`'s own contracts — they're unaffected consumers/producers on the new edges.

## Decisions

### D1: Restaurant provisioning becomes a saga on `RestaurantCreated`
Platform's own transaction writes only platform's tables (`organizations`, `users`, `subscriptions`) plus the outbox row, then publishes `RestaurantCreated`. Menu, ledger, and pos each independently consume it and provision their own default data (menu's `restaurants` row + default menu, ledger's default accounts, pos's default payment methods) — menu is a consumer here too, not a participant in platform's transaction, since it's now its own service (see `menu-service` spec, "Menu creates a restaurant's default data by consuming RestaurantCreated"). This means a restaurant is not fully usable (no menu, no default accounts, no default payment methods) until all three consumers have processed the event — an eventual-consistency window that did not exist before this change.

Alternatives considered:
- *Synchronous orchestration with compensation* (platform calls each service's gRPC provision endpoint in sequence, rolling back on failure): rejected per your answer — compensation logic across three services is a new failure class this codebase has never had to handle, versus outbox delivery which is already implemented, tested, and running in production for the `TicketClosed`/COGS edges.
- *Leave provisioning co-located*: rejected — you chose the full four-domain split, so there's no single "core" binary left to co-locate it in.

### D2: Shared Postgres instance, schema-per-service, cross-schema FKs preserved
Each new service gets its own schema (`ledger`, `pos`, `menu` — platform likely already has a dedicated schema from the inventory split's convention; confirm and reuse rather than assume in tasks.md) on the same Postgres instance, `search_path` set per service the same way `internal/inventory/adapters/postgres/schema.go` does it. FKs from any schema into `restaurants`/`users` stay as real Postgres foreign keys.

Alternatives considered: app-level integrity only, and per-service Postgres instances — both rejected per your answer; this is the lowest-risk option and matches the one precedent already in production.

### D3: `restaurants` and `users` stay where they are
No table move. Rejected alternative (moving `restaurants` to platform) per your answer — an unforced migration on top of an already-large change.

### D4: Extraction order — menu, then ledger, then pos, then platform
Chosen so each step only requires the *next* service to speak the new contracts, never a service extracted later:
1. **menu** first: no outbound dependency on any other domain being split (its only coupling is inbound FKs and pos's in-process reads, both of which become cross-schema/gRPC regardless of what's split next).
2. **ledger** second: already gRPC-exposed as `LedgerService` for inventory; only needs pos's shift-posting edge converted to outbox, which can happen at this step.
3. **pos** third: by now both menu and ledger already speak gRPC/outbox, so pos's extraction is "convert two in-process bridges to clients" with no new server-side work on the other end.
4. **platform** last: stays in `cmd/aivo-server` as the saga initiator and `aivo-auth` caller throughout steps 1–3, so provisioning keeps working via the *old* shared-transaction path for whichever domains haven't been extracted yet, and only flips to the full saga once all three consumers exist. Extracting platform last means the saga is fully wired and tested (steps 1-3 already added `RestaurantCreated` consumers) before platform's own transaction changes.

### D5: pos's menu reads move to gRPC; ticket_lines.menu_item_id FK is dropped
Matches the "no FK, cross-context" convention inventory already established for its own `menu_item_id` column — this proposal makes pos consistent with that precedent rather than inventing a second convention.

### D6: A single edge reverse-proxy fixes the unfinished frontend wiring, for all five split services
Add one reverse-proxy (path-prefix routing, e.g. `/api/v1/restaurants/{id}/inventory/*` → `aivo-inventory`, `/api/v1/restaurants/{id}/menu/*` → `aivo-menu`, etc.) in front of all backend services, in `deploy/docker-compose.yml`. Every SPA keeps calling one relative origin (`/api/...`), exactly as `frontend/admin/vite.config.ts`'s dev proxy already assumes — no per-service base URLs, no CORS middleware needed anywhere.

Alternatives considered:
- *Per-service base URLs + CORS in each Go service*: rejected — five services would each need CORS middleware (a new security-relevant surface per service, "treat tenant isolation... as security-critical" per AGENTS.md), and every SPA's api client would need per-route origin selection. More moving parts than one boring, well-understood proxy.
- *Leave it unfixed, scope it out*: rejected — without this, the split services are unreachable from a real browser, which makes "behavior-preserving" unverifiable end-to-end; this is existing debt from the inventory split that this change is the right place to close, since it's touching every service's deployment wiring anyway.

## Risks / Trade-offs

- **[Risk] Provisioning's eventual-consistency window is new user-visible behavior** (a restaurant created "now" may not have default accounts/menu/payment-methods for a few seconds) → Mitigation: this mirrors the exact window `TicketClosed`/COGS already has in production; document it in each consumer's onboarding UX (e.g. "setting up your restaurant..." state) if product wants it surfaced — out of scope for this backend change.
- **[Risk] Four more binaries, four more Dockerfiles/compose blocks, four more `AUTH_PUBLIC_KEY` wirings** — the inventory split already once produced a real deployment-blocking gap (`aivo-inventory` compose block missing `AUTH_PUBLIC_KEY`) that had to be caught by actually running `docker compose up` → Mitigation: tasks.md's migration steps end each phase with an actual `docker compose up --build` smoke test, not just `go build`.
- **[Risk] The reverse-proxy is a new single point of failure and a new component to operate** → Mitigation: it's stateless and trivial to run in dev (one more compose service); acceptable given it removes CORS/multi-origin complexity from every other service instead.
- **[Trade-off] Extracting platform last means steps 1–3 temporarily run a hybrid**: menu/ledger/pos already consume `RestaurantCreated`, but platform hasn't started publishing it yet (it still calls the old in-process hook for whichever domains are already extracted, via a thin adapter that publishes the event on their behalf) until platform's own extraction in step 4. This hybrid period needs an explicit task-level plan (see tasks.md) — accepted as the cost of a safe, incremental order over a risky big-bang cutover.

## Migration Plan

Phased, one domain per phase, each phase independently shippable and behavior-verified before the next starts (same discipline as the inventory split):

1. **menu**: extract `cmd/aivo-menu`, own schema, `RestaurantCreated` consumer, gRPC surface for pos's reads (built but not yet called by pos). Add the reverse-proxy in front of all services. Verify via `docker compose up --build` + smoke test.
2. **ledger**: extract `cmd/aivo-ledger` (move existing `LedgerService` off `cmd/aivo-server`), own schema, `RestaurantCreated` consumer, pos's shift-posting outbox edge (producer side lands in pos, consumer side in ledger). Verify.
3. **pos**: extract `cmd/aivo-pos`, own schema, switch `menubridge`/`ledgerbridge` call sites to the gRPC client / outbox producer built in steps 1-2, drop `ticket_lines.menu_item_id` FK, `RestaurantCreated` consumer for default payment methods. Verify, including the existing `TicketClosed` edge to inventory still works unchanged.
4. **platform**: extract `cmd/aivo-platform`, flip provisioning from the shared-transaction hook to publishing `RestaurantCreated`, delete `internal/provisioning`'s old shared-tx hook entirely. Verify end-to-end: create a restaurant, confirm menu/ledger/pos all provision their default data.

Rollback per phase: each phase's extracted domain can be reverted independently by keeping its code in `cmd/aivo-server` until that phase's PR merges — no phase depends on a later phase's code existing, only on earlier phases' consumers already being in place (per D4's ordering).
