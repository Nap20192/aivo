# Запуск

Полный стек (Postgres, MinIO, сервер + все три SPA):

```bash
docker compose -f deploy/docker-compose.yml up -d --build
go run -C backend ./cmd/aivo-seed   # демо-тенант Ember & Bone
```

Дальше: `http://localhost:8080/admin` (owner@ember.test / embertest1),
`/pos` (waiter@ember.test / embertest1), ссылки столов печатает seed.

Нативный запуск, переменные окружения, команды — в
[README](https://github.com/Nap20192/aivo#readme) и `AGENTS.md`.

## Структура

```
backend/    Go-модуль: cmd/, internal/{platform,menu,pos}, domain, sharedkernel
frontend/   admin, pos, menu (Vite + React + TS) + design-system
deploy/     Dockerfile, docker-compose.yml
```

Архитектурные решения: `CONTEXT-MAP.md`, ADR возле контекстов,
DDD-ресёрч в `docs/research/`. Схема БД: `docs/db/schema.svg` (tbls),
события: [Доменные события](../EVENTS.md).

## Документация

```bash
uvx --from mkdocs-material --with mkdocs-static-i18n mkdocs serve   # http://127.0.0.1:8000
```
