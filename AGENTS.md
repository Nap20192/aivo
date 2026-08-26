# AGENTS.md

## Project

**AIVO RMS** is an open-source, AI-powered restaurant management system SaaS.

Goal: help restaurants manage operations with simple, reliable software and useful AI automation.

## Product scope

Business logic is not yet decided. The domain model is being charted via the wayfinder map in GitHub Issues; do not assume a fixed module list until decisions land there and in `CONTEXT.md`.

Direction so far: a platform where restaurants self-register and integrate; Go backend core; satellite services (web backoffice, POS terminal, waiter app, web menu); an AI agent (or MCP tools/skills) assisting management, analysis, and forecasting.

## Working rules for agents

- Keep the codebase boring and maintainable.
- Prefer existing code, standard libraries, and framework defaults before adding dependencies.
- Do not add speculative abstractions or placeholder systems.
- Treat tenant isolation, permissions, payments, and customer data as security-critical.
- Validate all external input at boundaries.
- Never commit secrets, API keys, credentials, or real customer data.
- Add the smallest useful test for non-trivial behavior.
- Update docs when behavior, commands, setup, or configuration changes.

## AI feature rules

AI must assist, not silently control business-critical actions.

- Show confidence, reasoning summary, or source data where useful.
- Require confirmation for destructive or financial actions.
- Log AI-generated operational recommendations.
- Keep prompts and model calls tenant-safe.
- Do not train or leak data across tenants.

## Agent skills

### Issue tracker

Issues live in this repo's GitHub Issues (`Nap20192/aivo`), via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.

## Commands

Layout: `backend/` — the Go module (`cmd/`, `internal/`, `pkg/`); `frontend/` — the three Vite SPAs (`admin`, `pos`, `menu`) plus `design-system/`.

```bash
docker compose -f deploy/docker-compose.yml up -d --build   # run the full stack
go build -C backend ./... && go test -C backend ./...   # build + test backend
go run -C backend ./cmd/aivo-server                # run server natively
go run -C backend ./cmd/aivo-seed                  # seed demo tenant
cd frontend/admin && npm ci && npm run build       # build a SPA (same for pos, menu)
cd backend && sqlc generate                        # regen query code (backend/sqlc.yaml)
cd backend/proto && buf generate                    # regen gRPC code (backend/proto/**/*.proto)
uvx --from mkdocs-material --with mkdocs-static-i18n mkdocs serve   # docs site (ru/en)
```
