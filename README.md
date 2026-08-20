# Menu

The `web-menu` satellite service: the diner-facing digital menu, ordering,
and landing page for a single restaurant, reached via a per-table link. See
the wayfinder map ([issue #12](https://github.com/Nap20192/aivo/issues/12))
for how this fits the rest of AIVO RMS, and `CONTEXT.md` for the domain
glossary (Restaurant, Table, Menu, Order, Service request, etc.) used
throughout this codebase.

## Running locally

1. Start Postgres:

   ```bash
   docker-compose up -d
   ```

2. Set environment variables:

   | Var | Required | Notes |
   |---|---|---|
   | `DATABASE_URL` | yes | e.g. `postgres://aivo_menu:aivo_menu@localhost:5432/aivo_menu?sslmode=disable` (matches `docker-compose.yml`) |
   | `TOKEN_ENCRYPTION_KEY` | yes | base64-encoded 32 random bytes (AES-256), e.g. `openssl rand -base64 32` |
   | `TELEGRAM_BOT_TOKEN` | no | for the seed script's demo `NotificationChannel`; skipped with a warning if unset |
   | `TELEGRAM_CHAT_ID` | no | see above |
   | `BASE_URL` | no | defaults to `http://localhost:8080`; used to print the table link |
   | `PORT` | no | defaults to `8080` |

3. Run the migration:

   ```bash
   psql "$DATABASE_URL" -f internal/menu/adapters/postgres/migrations/0001_init.sql
   ```

4. Seed a demo restaurant (there is no admin API/UI in this MVP — see
   `AGENTS.md` — so this is the only way to get data in):

   ```bash
   go run ./cmd/menu-seed
   ```

   Not idempotent — re-running against an already-seeded database fails on
   the `restaurants.slug` unique constraint.

5. Run the server:

   ```bash
   go run ./cmd/menu-server
   ```

6. Open the table link the seed script printed (`Table link: ...`) in a
   browser.

## What's not done

- Never run end-to-end against real infra: no live Postgres or Telegram bot
  was available in the build sandbox, so the migration, seed script, and
  server have not actually been exercised together.
- No admin API/UI — the seed script is the only way to populate a
  Restaurant, per the wayfinder map's decision.
- No gRPC/proto layer — deferred per the `ponytail:` note near `main.go`
  (see root `docs/adr/0001`) until a second internal service needs to call
  this one.
- Currency/locale and image hosting are still "Not yet specified" in the
  wayfinder map.
