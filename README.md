# <img src="docs/assets/icon.svg" alt="aivo" width="28"/> AIVO RMS

Multi-tenant restaurant management SaaS: restaurants self-register, build
a digital menu, manage it from an admin panel, and run service with a
phone POS. One Go binary (`cmd/aivo-server`) serves the whole platform —
see `docs/PLATFORM.md` for the build contract and `internal/menu/CONTEXT.md`
for the menu domain glossary.

## Layout

- `internal/platform` — organizations, users/auth (bcrypt + Postgres
  sessions), subscriptions (fake billing), restaurant provisioning,
  themes, custom domains.
- `internal/menu` — diner menu, orders, service requests (existing
  context, now tenant-scoped under the platform).
- `internal/pos` — shifts, per-table tickets, kitchen firing; talks to
  the menu context via in-process Go interfaces.
- `cmd/aivo-server` — the API (`/api/v1`), legacy menu API, and static
  SPAs (`/admin`, `/pos`, tenant menu routes `/{slug}`, `/{slug}/t/{token}`).
- `cmd/aivo-seed` — demo tenant "Ember & Bone".

## Running locally

1. Start infra + the server (Postgres, MinIO, `aivo-server`):

   ```bash
   docker-compose up -d --build
   ```

   The server applies all SQL migrations itself on startup (tracked in
   `schema_migrations`); no manual `psql` step.

2. Seed the demo tenant:

   ```bash
   DATABASE_URL=postgres://aivo:aivo@localhost:5432/aivo?sslmode=disable go run ./cmd/aivo-seed
   ```

   Prints the table links. Demo logins: `owner@ember.test` /
   `embertest1`, `waiter@ember.test` / `embertest1`. Not idempotent —
   re-running fails on the owner email unique constraint.

3. Open `http://localhost:8080/admin` (admin), `/pos` (POS), or a printed
   table link (diner menu). SPA routes answer 503 until the corresponding
   `web/<app>/dist` exists (`npm run build` in each app).

Env vars (native `go run ./cmd/aivo-server` instead of docker-compose):

| Var | Required | Notes |
|---|---|---|
| `DATABASE_URL` | yes | e.g. `postgres://aivo:aivo@localhost:5432/aivo?sslmode=disable` |
| `TOKEN_ENCRYPTION_KEY` | yes | base64-encoded 32 random bytes (AES-256), e.g. `openssl rand -base64 32` |
| `BASE_URL` | no | defaults to `http://localhost:8080`; used in table links/QRs |
| `PORT` | no | defaults to `8080` |
| `S3_ENDPOINT` | no | e.g. `localhost:9000`; unset disables image uploads (503) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` / `S3_BUCKET` / `S3_PUBLIC_URL` | no | see `docker-compose.yml` for the dev values |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | no | legacy menu notification channel (optional) |
| `THEME_GENERATOR` | no | `claudecli` enables AI theme proposals via the `claude` CLI; unset = endpoint 503s |
| `ASSISTANT` | no | `claudecli` enables the admin AI assistant chat; unset = endpoints 503 |
| `CLAUDE_BIN` | no | path to the `claude` binary (default `claude`), shared by both AI features |
| `RESTAURANT_TZ` | no | IANA timezone for POS display times (e.g. `Europe/Brussels`); default server-local |

## Commands

```bash
go build ./... && go vet ./... && go test ./...   # build + checks
go run ./cmd/aivo-server                          # run the server
go run ./cmd/aivo-seed                            # seed Ember & Bone
```

## Image storage

MinIO (S3-compatible) with a public-read bucket `aivo-menu-images` at
`http://localhost:9000` (console `:9001`, `aivo` / `aivo_minio`).
`POST /api/v1/restaurants/{id}/images` (multipart field `image`) uploads
and returns the public URL.

## Not done / stubs

- Billing is a fake in-memory provider (no real payments); Stripe slots
  in behind `platform/ports.BillingProvider`.
- Custom domains route by Host header only — no DNS verification or
  certificate automation (v1 stub per `docs/PLATFORM.md`).
- Staff invited without a password get an unguessable random one; there
  is no invite email/reset flow yet.
- No gRPC — contexts talk in-process (root `docs/adr/0001`).
