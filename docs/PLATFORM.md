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
- `GET  /api/v1/t/{table_token}` → restaurant, table, theme (flat),
  `menus`: `[{id, slug, name, is_default, categories: [...]}]` (default
  first, then position — replaces the old flat `menu` array),
  `open_requests`. Client types: `web/menu/src/types.ts`.
- `GET  /api/v1/m/{restaurant_slug}/{menu_slug}` — public read-only
  browse of one menu (no table, no session): `{restaurant, theme, menu:
  {id, slug, name, categories}}`, 404 unknown. Served by the diner SPA
  at `/{restaurant_slug}/m/{menu_slug}` in browse mode.
- `POST /api/v1/t/{table_token}/orders` — 204 on success; 429 with
  `error.retry_after_seconds` + `Retry-After` header on the order cooldown
- `POST /api/v1/t/{table_token}/requests` {type: waiter|bill} → {request};
  409 `already_open` for a duplicate open request on the table

Platform (session cookie):
- `POST /api/v1/auth/register` {org_name, restaurant_name, email, password} → creates org+owner+restaurant+slug, starts `free`
- `POST /api/v1/auth/login` / `POST /api/v1/auth/logout` / `GET /api/v1/auth/me`
- `GET/PATCH /api/v1/org` — org settings, `GET /api/v1/org/subscription`, `POST /api/v1/org/subscription` {plan}
- `GET/POST /api/v1/restaurants`, `GET/PATCH /api/v1/restaurants/{id}` —
  admin-stream shapes (implemented): `hours` is `[{label, open, close}]`,
  `phone`/`instagram`/`custom_domain` are flat fields; lists are bare
  arrays; auth responses are `{user, org, restaurants}` (+ `restaurant`
  for POS). Client types: `web/admin/src/api/types.ts`.
- `GET/PUT  /api/v1/restaurants/{id}/theme` — flat Theme object
  `{brand_name, accent, bold, banner_url, css_vars, design_md}` (stored
  as theme JSON + design_md text)
- `POST /api/v1/restaurants/{id}/theme/generate` (manager+) — AI theme
  proposal from the stored design_md: `{proposal: <Theme>, based_on:
  "design_md"}`. Never saves — applying is the PUT above. 409
  `no_design_md` when the brief is empty; 503 `generator_unconfigured`
  unless the server runs with `THEME_GENERATOR=claudecli` (shells out to
  the `claude` CLI, `CLAUDE_BIN` overrides the binary path); 502
  `generation_failed` on CLI/validation failure. Model output is strictly
  validated (accent enum, `--name` css var keys, value injection guard,
  banner_url always kept from the current theme); proposals are logged
  server-side.
- Menus (1..N per restaurant, exactly one default, auto-created on
  provisioning as "menu"/"Menu"): `GET/POST /api/v1/restaurants/{id}/menus`,
  `PATCH/DELETE .../menus/{menu_id}` `{name, slug, position, is_default}` —
  promoting a menu clears the old default atomically; deleting the default
  or last menu is 422; deleting a non-empty menu needs `?force=1`
  (categories + items cascade).
- CRUD `/api/v1/restaurants/{id}/categories` (categories belong to a menu:
  `menu_id` field on create — default menu when omitted — and `?menu_id=`
  filter on list), `/api/v1/restaurants/{id}/items`
  (items: name, desc, price cents, image_url, allergens[], option_groups[], available)
- `GET/POST /api/v1/restaurants/{id}/tables`, `POST .../tables/{id}/regenerate` (token), `GET .../tables/{id}/qr`
- `POST /api/v1/restaurants/{id}/images` — multipart upload → S3, returns URL
- Staff: `GET/POST /api/v1/restaurants/{id}/staff` {email, role}

Customer accounts (diner logins — optional, anonymous flow stays; cookie
`aivo_customer`, HttpOnly, sessions fully separate from staff: neither
cookie ever resolves in the other's session store):
- `POST /api/v1/customer/register` `{email, password, name}` → 201
  `{customer: {id, email, name, phone}}` + session cookie
- `POST /api/v1/customer/login` / `POST /api/v1/customer/logout`
- `GET  /api/v1/customer/me` → `{customer, orders: [{restaurant_name,
  created_at, total_cents, lines: [{name, qty, unit_price_cents,
  total_cents, options: [label]}]}]}` (own history, newest first)
- Diner order submit and cart handoff attach `customer_id` automatically
  when the cookie is present.

Cart handoff (diner stores the cart under a short pickup code and shows
it to the waiter; coexists with direct kitchen send):
- `POST /api/v1/t/{table_token}/handoff` `{lines: same shape as order
  submit, note}` → 201 `{code, qr_url, expires_at}`. Code: 6 chars from
  A-Z2-9 minus 0/O/1/I, unique among active, TTL 15 min, single-use,
  restaurant-scoped; a new handoff replaces the table's previous active
  one (no stacking); same per-session cooldown as order submit (429 +
  retry_after_seconds).
- `GET /api/v1/t/{table_token}/handoff/qr?code=X` → PNG QR of the code.
- POS (waiter+): `GET /api/v1/pos/handoff/{code}` (case-insensitive) →
  `{code, table_id, table_number, customer_name|null, note|null, lines:
  [ticket-line shape], total_cents, expires_at}` — 404 for unknown/
  expired/used/foreign, all identical. `POST
  /api/v1/pos/handoff/{code}/accept` `{table_id?}` (defaults to the
  diner's table) → appends snapshot lines to that table's ticket via the
  normal add-lines path, consumes the code (single-use; double accept
  404), returns the updated ticket.

CRM (manager+, restaurant-scoped; a restaurant sees ONLY customers with
a guest_profile row — created lazily on first linked order/handoff — and
only its own orders; waiters see the name only via the handoff preview):
- `GET /api/v1/restaurants/{id}/guests?query=&limit=` → sorted by
  last_seen desc: `[{customer: {id, name, email, phone}, visits,
  total_spent_cents, last_seen, tags}]`
- `GET /api/v1/restaurants/{id}/guests/{customer_id}` → `{customer,
  visits, total_spent_cents, first_seen, last_seen, notes, tags, orders:
  [{id, created_at, table_label, total_cents, lines: [{name, qty,
  total_cents}]}]}`
- `PATCH /api/v1/restaurants/{id}/guests/{customer_id}` `{notes?, tags?}`
  → same shape as GET.

Admin AI assistant (manager+, restaurant-scoped chat; nothing applies
without explicit confirm — proposals and applied sets are slog-logged):
- `GET  /api/v1/restaurants/{id}/assistant/messages?limit=50` — history
  (oldest first), message: `{id, role, text, attachments, actions,
  action_status, created_at}`.
- `POST /api/v1/restaurants/{id}/assistant/messages` — multipart `text` +
  `files[]` (images stored in S3 and listed to the model as usable
  `image_url`s; `.md/.txt/.csv` ≤64KB inlined into the prompt and stored
  too). Returns the stored assistant message with `actions` proposed, NOT
  executed. Action allowlist: create/rename/delete_category, create/
  update/delete_item, set_item_available, update_theme, create_menu —
  hard-validated at the boundary (unknown type or any invalid action
  drops the whole list but keeps the reply; referenced ids must belong to
  the restaurant; `price_cents` int ≥ 0; `image_url` only on our S3 public
  host; css_vars same injection guard as the theme generator).
- `POST .../assistant/messages/{msg_id}/apply` `{action_indexes?: [int]}`
  — executes selected actions via the existing commands (sequential,
  stop-on-first-failure with per-action results), marks `applied`.
  `POST .../discard` marks `discarded`. Both 409 if already decided.
- 503 `assistant_unconfigured` unless `ASSISTANT=claudecli` (shares
  `CLAUDE_BIN` with the theme generator; 120s timeout; 502
  `assistant_failed` on CLI/parse failure).

POS (session cookie, waiter+):
- `GET  /api/v1/pos/state` — restaurant, open shift, tables w/ tickets, requests,
  plus (pos-stream extension, implemented by backend): `menu` (categories →
  items with `price_cents`, optional `mods` = single-select option labels;
  union of every menu's categories, labels prefixed "Dinner · Starters"
  when the restaurant has more than one menu),
  `till` (always 1 in v1), `cashier`, `other_till_shift` (always null in v1 —
  one till per restaurant); shift carries server-computed running
  `expected_cents` and a display `number` ("shift-N"). Display times are
  local "HH:MM" strings. Line `options` are labels; `unit_price_cents`
  includes option deltas. Client types: `web/pos/src/types.ts`.
  Close returns the PostedShift shape (`number`, `expected_cents`,
  `declared_cents`, `variance_cents`, `posted_at`, `gl_lines`).
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
