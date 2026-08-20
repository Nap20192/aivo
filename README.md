# Menu

The `web-menu` satellite service: the diner-facing digital menu, ordering,
and landing page for a single restaurant, reached via a per-table link. See
the wayfinder map ([issue #12](https://github.com/Nap20192/aivo/issues/12))
for how this fits the rest of AIVO RMS, and `CONTEXT.md` for the domain
glossary (Restaurant, Table, Menu, Order, Service request, etc.) used
throughout this codebase.

## Running locally

1. Start infra + the server (Postgres, MinIO for image storage, and
   `menu-server` itself, built from the root `Dockerfile`):

   ```bash
   docker-compose up -d --build
   ```

   `menu-server` will be up but empty — run the migration and seed step
   below against it before it's useful. Env vars for the containerized
   server live in `docker-compose.yml` (dev-only values, safe to commit);
   see the table below if running `go run` natively instead.

2. Set environment variables (only needed if running natively, i.e. not
   via `docker-compose`):

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

5. Run the server (skip if you started it via `docker-compose` in step 1):

   ```bash
   go run ./cmd/menu-server
   ```

6. Open the table link the seed script printed (`Table link: ...`) in a
   browser.

## Image storage

`docker-compose.yml` provisions MinIO (S3-compatible) with a public-read
bucket, `aivo-menu-images`, at `http://localhost:9000` (console:
`http://localhost:9001`, `aivo_menu` / `aivo_menu_minio`) — diners' browsers
load images directly from it, no signed URLs. `domain.MenuItem.ImageURL`
and Landing banner blocks are already just URL strings, so pointing one at
`http://localhost:9000/aivo-menu-images/<key>` works today. There is no
upload endpoint yet (nothing in the app writes to the bucket) — this only
provisions where those URLs will eventually point; building an actual
upload path is future work, not speculated on here.

## What's not done

- Never run end-to-end against real infra: no live Postgres or Telegram bot
  was available in the build sandbox, so the migration, seed script, and
  server have not actually been exercised together.
- No admin API/UI — the seed script is the only way to populate a
  Restaurant, per the wayfinder map's decision.
- No gRPC/proto layer — deferred per the `ponytail:` note near `main.go`
  (see root `docs/adr/0001`) until a second internal service needs to call
  this one.
- Currency/locale is still "Not yet specified" in the wayfinder map.
- Image *storage* is provisioned (MinIO, see "Image storage" above) but
  there's no upload endpoint — images still have to land in the bucket by
  hand (or via the S3 API directly) and get their URL pasted into
  `ImageURL`/banner data manually.
