# Running locally

Full stack (Postgres, MinIO, server + all three SPAs):

```bash
docker compose -f deploy/docker-compose.yml up -d --build
go run -C backend ./cmd/aivo-seed   # demo tenant Ember & Bone
```

Then: `http://localhost:8080/admin` (owner@ember.test / embertest1),
`/pos` (waiter@ember.test / embertest1); the seed prints table links.

Native run, environment variables, commands — see the
[README](https://github.com/Nap20192/aivo#readme) and `AGENTS.md`.

## Layout

```
backend/    Go module: cmd/, internal/{platform,menu,pos}, domain, sharedkernel
frontend/   admin, pos, menu (Vite + React + TS) + design-system
deploy/     Dockerfile, docker-compose.yml
```

Architecture decisions: `CONTEXT-MAP.md`, ADRs next to their contexts,
DDD research in `docs/research/`. DB schema: `docs/db/schema.svg` (tbls),
events: [Domain events](../EVENTS.md).

## Documentation

```bash
uvx --from mkdocs-material --with mkdocs-static-i18n mkdocs serve   # http://127.0.0.1:8000
```
