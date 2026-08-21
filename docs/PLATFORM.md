# AIVO Platform — build contract (v1)

Shared contract for the platform build-out. Four workstreams build against this
document in parallel: `backend` (Go core), `admin` (backoffice web app),
`menu` (diner-facing menu site), `pos` (waiter POS PWA). The lead session
integrates and arbitrates. If reality forces a deviation, the deviating agent
updates this file and flags it to the lead.

## Product

Multi-tenant SaaS: a restaurant organization self-registers, gets its own
routes (slug now, custom domain later), builds a fully customizable digital
menu, manages it from an admin panel (with subscription management), and runs
service with a phone POS for waiters.

## Architecture

- One Go module (`aivo`), one HTTP server binary `cmd/aivo-server` (grows out
  of `cmd/menu-server`). DDD contexts under `internal/<context>/` with the
  existing hexagonal layout (`domain/`, `app/`, `ports/`, `adapters/`).
- Contexts:
  - `internal/platform` — NEW. Organizations, users, auth, subscriptions,
    restaurants (provisioning), tenant resolution, theme/design storage.
  - `internal/menu` — EXISTS. Diner menu, orders, service requests. Becomes
    tenant-scoped via `restaurant_id` (already keyed by restaurant).
  - `internal/pos` — NEW. Shifts, tickets, firing, requests inbox (waiter
    surface). Reads menu context via Go interfaces in-process, not gRPC
    (per ADR 0001 — no gRPC until a second process needs it).
- Postgres (pgx), MinIO/S3 for images, plain `net/http` + stdlib routing
  (Go 1.22+ pattern matching). No new frameworks.
- Migrations: numbered SQL files in `internal/<context>/adapters/postgres/migrations/`.
  Platform migrations start at `internal/platform/adapters/postgres/migrations/0001_init.sql`.
- Frontends: static SPAs under `web/<name>/` (vanilla Vite + React + TypeScript),
  served by the Go binary (`/admin`, `/pos`) or at tenant routes (menu).
  Design tokens imported from `web/design-system/` (do not fork token values).

## Tenancy & routing

- `Organization` (billing/auth boundary) 1—N `Restaurant` (operational tenant).
- Public menu routes: `/{restaurant_slug}` (landing), `/{restaurant_slug}/menu`,
  `/{restaurant_slug}/t/{table_token}` (table entry, sets table session).
- Custom domains: `custom_domains(domain, restaurant_id, verified_at)` table;
  Host-header resolution middleware falls back to slug routing. Certificate
  automation is out of scope for v1 (documented stub).
- Admin app: `/admin` (org-scoped after login). POS app: `/pos` (restaurant-scoped).
- Tenant isolation is security-critical: every query filters by
  `restaurant_id`/`org_id` derived from the authenticated session or table
  token — never from client-supplied IDs.

## Auth

- Platform users (owners/managers/waiters): email+password (bcrypt via
  `golang.org/x/crypto`), server-side sessions in Postgres, HttpOnly cookie
  `aivo_session`. Roles: `owner`, `manager`, `waiter` (per restaurant).
- Diners: anonymous; table token IS the credential (existing menu model).
- POS login: same user session; waiters see only their restaurant.

## Subscriptions

- Plans: `free` (1 restaurant, 30 menu items), `pro` (unlimited items, custom
  domain, theming), `business` (multi-restaurant, POS seats). Enforcement =
  plan checks in platform app layer.
- Billing provider: port `ports.BillingProvider` with a fake in-memory
  implementation now; Stripe adapter later. No real payment processing in v1.
  Subscription state machine: `trialing → active → past_due → canceled`.

## Menu customization ("design.md builder")

- `restaurant_themes` row per restaurant: JSON config — brand name, accent
  (Blood red/Olive/Wine/Fire), bold flag, banner image URL, font overrides,
  section layout, custom CSS variables. The menu app applies it as CSS custom
  properties over the design-system tokens (exactly how the prototype's
  `themeVars` works).
- `design_md` text column: a restaurant can paste a design brief (design.md /
  Claude design output). v1: stored + rendered in admin as the theme's source
  of truth; a "builder" panel edits the structured theme JSON. AI translation
  of design.md → theme JSON is a later feature (log it, don't build it).

## API surface (JSON, `/api/v1`)

Public (diner, table-token scoped — existing menu handlers keep their shapes):
- `GET  /api/v1/t/{table_token}` → restaurant, table, theme, menu
- `POST /api/v1/t/{table_token}/orders`
- `POST /api/v1/t/{table_token}/requests` (waiter|bill)

Platform (session cookie):
- `POST /api/v1/auth/register` {org_name, restaurant_name, email, password} → creates org+owner+restaurant+slug, starts `free`
- `POST /api/v1/auth/login` / `POST /api/v1/auth/logout` / `GET /api/v1/auth/me`
- `GET/PATCH /api/v1/org` — org settings, `GET /api/v1/org/subscription`, `POST /api/v1/org/subscription` {plan}
- `GET/POST /api/v1/restaurants`, `GET/PATCH /api/v1/restaurants/{id}` (slug, name, hours, address, contacts)
- `GET/PUT  /api/v1/restaurants/{id}/theme` — theme JSON + design_md
- CRUD `/api/v1/restaurants/{id}/categories`, `/api/v1/restaurants/{id}/items`
  (items: name, desc, price cents, image_url, allergens[], option_groups[], available)
- `GET/POST /api/v1/restaurants/{id}/tables`, `POST .../tables/{id}/regenerate` (token), `GET .../tables/{id}/qr`
- `POST /api/v1/restaurants/{id}/images` — multipart upload → S3, returns URL
- Staff: `GET/POST /api/v1/restaurants/{id}/staff` {email, role}

POS (session cookie, waiter+):
- `GET  /api/v1/pos/state` — restaurant, open shift, tables w/ tickets, requests,
  plus (pos-stream deviation, needs backend ack): `menu` (categories → items with
  `price_cents`, optional `mods`), `till`, `cashier`, `other_till_shift` (for the
  one-open-shift-per-till card); shift carries server-computed `expected_cents`.
  Client types: `web/pos/src/types.ts`.
- `POST /api/v1/pos/shifts` {opening_float_cents} / `POST /api/v1/pos/shifts/{id}/close` {declared_cents}
- `POST /api/v1/pos/tables/{table_id}/lines` — add order lines (menu_item_id, qty, options)
- `POST /api/v1/pos/tickets/{id}/fire`
- `POST /api/v1/pos/requests/{id}/ack|dismiss`
- Poll `GET /api/v1/pos/state` every 5s for v1; SSE later.

Money: integer cents everywhere. IDs: uuid strings. Errors:
`{"error": {"code": "...", "message": "..."}}`, 401/403/404/422.

## Design source of truth

- Tokens/bundle: `web/design-system/` (styles.css, tokens/, _ds_bundle.js, support.js).
- Screen specs: `docs/prototypes/aivo-menu-prototype.dc.html` (diner menu),
  `docs/prototypes/aivo-pos-prototype.dc.html` (waiter POS),
  `docs/prototypes/aivo-menu-screen-board.dc.html` (screen board),
  design-system readme rules (voice, casing, no emoji, JetBrains Mono for all
  figures, sentence case, warm paper surfaces).
- Admin panel: same tokens, desktop layout: 236px sidebar, 60px topbar,
  content max 1180px.

## Ownership

| Stream | Owns | Must not touch |
|---|---|---|
| backend | `internal/platform`, `internal/pos`, `cmd/aivo-server`, menu context changes, migrations, docker-compose | `web/*` except serving |
| admin | `web/admin/` | Go code, other SPAs |
| menu | `web/menu/` (rebuild as Vite SPA) | Go code, other SPAs |
| pos | `web/pos/` | Go code, other SPAs |

Frontends develop against this contract with a local mock (msw or a tiny
fetch wrapper with fixtures) so they never block on backend; wire to real API
at integration. Backend seeds a demo tenant (`cmd/aivo-seed`) matching the
prototype fixtures (Ember & Bone).

## Working rules

- Follow repo AGENTS.md: boring code, validate at boundaries, no secrets,
  smallest useful test per non-trivial behavior, update docs.
- Commit per meaningful slice on branch `feat/platform` (backend) /
  `feat/admin` / `feat/menu-app` / `feat/pos` — lead merges.
- `go build ./... && go vet ./...` must pass; SPAs: `npm run build` must pass.
