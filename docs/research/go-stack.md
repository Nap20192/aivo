# Research: Go stack for the platform core

Resolves [#5](https://github.com/Nap20192/aivo/issues/5). Part of #1.
Research date: 2026-08-17. All claims verified against primary sources (go.dev, pkg.go.dev, project repos/docs, GitHub release data).

## Recommended stack (one choice per area)

| Area | Pick | Runner-up | Close call? |
|---|---|---|---|
| Go version | **Go 1.26.x** (1.26.6 current) | — | — |
| HTTP router | **stdlib `net/http` ServeMux** (Go 1.22+ patterns) | go-chi/chi v5 | Yes — chi is close |
| DB access | **sqlc + pgx/v5** (`sql_package: "pgx/v5"`) | plain pgx/v5 | No |
| Migrations | **pressly/goose** (embedded, as library) | golang-migrate | Yes — migrate is close |
| Layout | **`cmd/` + `internal/<module>` per domain** (official layout doc) | — | — |
| Config | **stdlib `os.Getenv` + tiny helper** | caarlos0/env | Yes — env is close |
| Testing | **stdlib `testing`/`httptest` + testcontainers-go for Postgres** | docker-compose Postgres | No |

---

## 0. Go version and support policy

- Current stable: **Go 1.26.0, released February 10, 2026**; latest patch **1.26.6 (August 13, 2026)**. Go 1.27 was not yet released as of this date. Source: <https://go.dev/doc/devel/release>
- Support policy: "each major Go release is supported until there are two newer major releases" — so 1.26 and 1.25 (1.25.13) receive security fixes today. Source: <https://go.dev/doc/devel/release>
- Relevant recent stdlib gains for a SaaS backend (Go 1.25 release notes, <https://go.dev/doc/go1.25>): container-aware `GOMAXPROCS` (cgroup CPU limits respected automatically), `net/http.CrossOriginProtection` (token-less CSRF protection — directly useful for self-serve tenant registration forms), `testing/synctest` graduated to stable, `encoding/json/v2` available as `GOEXPERIMENT=jsonv2`.

## 1. HTTP router — pick: stdlib `net/http` ServeMux

Go 1.22 (Feb 2024) gave `http.ServeMux` method matching and wildcards, which removes most of the historical reason to import a router:

- Patterns like `"GET /posts/{id}"`; unmatched methods get automatic `405`; `{id}` single-segment and `{path...}` multi-segment wildcards; `{$}` for exact trailing-slash match; values read via `r.PathValue("id")`. Most-specific-pattern-wins precedence, with conflicting patterns panicking at registration. Source: "Routing Enhancements for Go 1.22", Jonathan Amsterdam, go.dev blog, Feb 13, 2024 — <https://go.dev/blog/routing-enhancements> ; API docs: <https://pkg.go.dev/net/http#ServeMux>
- What stdlib still lacks: middleware chaining helpers and route groups/subrouters. Neither needs a dependency: middleware is `func(http.Handler) http.Handler` composition (a 5-line `chain()` helper), and "groups" are string prefixes or a second `ServeMux` mounted with `mux.Handle("/api/v1/", http.StripPrefix(...))`. For one modular monolith with a known route surface, this does not matter.

Alternatives (both healthy, neither needed):

- **go-chi/chi**: v5.3.1 (July 6, 2026), repo pushed Aug 15, 2026, ~22.7k stars, not archived. Source: <https://github.com/go-chi/chi>. chi is itself stdlib-compatible (`http.Handler` everywhere), so adopting it later is a mechanical change — another reason not to pre-commit.
- **labstack/echo**: now on **v5** — v5.3.1 (July 21, 2026), repo pushed Aug 4, 2026, ~32.6k stars. Source: <https://github.com/labstack/echo>. Echo brings its own `echo.Context`/`echo.HandlerFunc` types, i.e., a framework lock-in surface you don't need.

**Rationale**: zero dependencies, zero upgrade treadmill, and the missing pieces are one-liners. **Runner-up flag**: chi is genuinely close — if route count grows past ~50 with per-group middleware stacks (tenant auth vs public vs admin), chi's `Route`/`Use` groups pay for themselves and the migration is cheap.

## 2. DB access — pick: sqlc generating pgx/v5 code

- **sqlc** (sqlc-dev/sqlc): v1.31.1 (April 22, 2026), repo pushed Aug 17, 2026, ~18.2k stars. Source: <https://github.com/sqlc-dev/sqlc>. You write real SQL; sqlc compile-time-checks it against your schema and generates typed Go. Config reference confirms `sql_package` accepts `"pgx/v4"`, `"pgx/v5"`, or `"database/sql"` (default `database/sql`) — set `sql_package: "pgx/v5"`. Source: <https://docs.sqlc.dev/en/latest/reference/config.html>
- **jackc/pgx**: latest tag **v5.10.0**, repo pushed Aug 16, 2026, ~14.1k stars — actively maintained. Source: <https://github.com/jackc/pgx>. It's the de-facto Postgres driver, with a native protocol implementation and `pgxpool` for pooling; sqlc rides on top of it.
- **GORM**: rejected. Its Traditional API accumulates errors on the chained `*gorm.DB` and requires checking `.Error` at runtime after each chain — "After a chain of methods, it's crucial to check the `Error` field" — i.e., query mistakes surface at runtime, not compile time (sqlc catches them at generation time). Source: <https://gorm.io/docs/error_handling.html>. It's also a large feature surface (hooks, associations, polymorphism, auto-migrations — <https://gorm.io/docs/>) that hides SQL, which is exactly the "magic" the project's working rules say to avoid, and auto-migration is a liability for tenant-critical schemas.

**Rationale**: sqlc is the boring middle path — plain SQL you can read in a code review (important for tenant-isolation `WHERE tenant_id = $1` auditing), typed Go, no runtime query builder. pgx/v5 underneath is the fastest, best-maintained Postgres driver. Not a close call.

## 3. Migrations — pick: pressly/goose

- **goose**: v3.27.3 (July 22, 2026), repo pushed Aug 8, 2026, ~11.3k stars. Source: <https://github.com/pressly/goose>. README confirms: embedded SQL migrations via `embed.FS` with `goose.SetBaseFS(embedMigrations)`, works as an importable library (`goose.Up(db, "migrations")`) not just a CLI, and supports Postgres.
- **golang-migrate/migrate**: v4.19.1 (Nov 29, 2025), repo pushed July 5, 2026, ~18.8k stars, 487 open issues. Also supports `io/fs` embedded sources and library use; README notes the v4 API is "stable and frozen". Source: <https://github.com/golang-migrate/migrate>. Perfectly fine, but release cadence is slower and its up/down file-pair + drivers-for-everything design is heavier than needed for Postgres-only.
- **ariga/atlas**: v1.3.0 (Aug 2, 2026). The CLI core is open source (inspection, diffing, migration planning/execution), but the value proposition — CI/CD integration, migration linting/safety checks, drift detection, schema governance — sits in **Atlas Pro at $9/dev/month** plus paid Pipelines/Monitoring tiers, and the free tier lists only "Basic" database features vs Pro's "All". Source: <https://atlasgo.io/pricing>. Declarative schema-as-code is also a bigger conceptual commitment than versioned SQL files. Rejected: too much toolchain/cloud gravity for a boring stack.

**Rationale**: goose gives exactly what a monolith wants — plain versioned `.sql` files embedded in the binary, run at startup or via a `cmd/migrate` subcommand, one dependency, no SaaS attachment. **Runner-up flag**: golang-migrate is close; pick it instead only if you need its multi-database source/driver matrix (you don't — Postgres is assumed).

## 4. Project layout — modular monolith

- Official guidance exists: **"Organizing a Go module"** (<https://go.dev/doc/modules/layout>). For server projects it recommends: `go.mod` at root, all server logic in `internal/` packages (so nothing becomes an accidental public API and you can refactor freely), binaries under `cmd/<name>/main.go`. It explicitly says server projects usually export no packages.
- **golang-standards/project-layout is not a standard.** Russ Cox (then Go tech lead) in issue #117 (Apr 28, 2021): "these are in no way official standards… the project-layout standard it puts forth is far too complex and not a standard," and "the minimal standard layout for an importable Go repo is really: put a LICENSE file in your root, put a go.mod file in your root, put Go code in your repo… That's it." He then lists ~18 "It is not required to put X in Y/" bullets (no required `pkg/`, `cmd/`, `configs/`, etc.). Source: <https://github.com/golang-standards/project-layout/issues/117> (comment by @rsc). Do not cargo-cult that repo.
- Modular-monolith pattern that follows from the official doc: one module, one binary, one package per domain module under `internal/` — e.g. `internal/tenant`, `internal/menu`, `internal/orders`, `internal/inventory`, `internal/staff`, `internal/billing` — each owning its handlers, sqlc queries, and service code, with cross-module calls going through ordinary Go function calls (and the compiler's `internal/` visibility rule enforcing the boundary against external reuse). Start flat; add sub-packages only when a module actually grows.

Concrete skeleton:

```
go.mod
cmd/aivo/main.go          # wires config, pool, mux, modules
internal/tenant/          # registration, tenant resolution middleware
internal/menu/ ...        # one package per product module
internal/platform/        # db pool, migrations embed, shared middleware (only when duplicated 3x)
migrations/*.sql          # embedded via goose
```

## 5. Config — pick: stdlib `os.Getenv` (+ one small helper)

For a 12-factor SaaS, config is a flat list of environment variables read once at startup (DATABASE_URL, LISTEN_ADDR, a few API keys). That is ~30 lines of stdlib: a `Config` struct populated in one `LoadConfig() (Config, error)` function using `os.Getenv`/`os.LookupEnv`, failing fast on missing required values. No file formats, no watching, no precedence rules — so no library.

Ecosystem status, for the record:

- **spf13/viper**: v1.21.0 (Sep 8, 2025), last push Jan 12, 2026 — notably quiet for eight months, and it drags in a large dependency tree plus features you won't use (config files, remote K/V, live watch). <https://github.com/spf13/viper>
- **knadh/koanf**: v2.3.6 (Aug 4, 2026), active, only 4 open issues — the good "viper replacement" if you ever need layered file+env config. <https://github.com/knadh/koanf>
- **caarlos0/env**: v11.4.1 (May 1, 2026), active — a single-purpose struct-tag env parser. <https://github.com/caarlos0/env>

**Rationale**: stdlib does it. **Runner-up flag**: caarlos0/env is close; adopt it if the config struct passes ~15 fields and the hand-rolled parsing starts accumulating type-conversion boilerplate.

## 6. Testing — pick: stdlib `testing` + `httptest` + testcontainers-go for Postgres

- Stdlib `testing` and `net/http/httptest` cover unit and handler tests with zero deps (`httptest.NewServer`, `httptest.NewRequest` against your `ServeMux`). <https://pkg.go.dev/net/http/httptest>
- `t.Context()` (added Go 1.24) gives a per-test context canceled at test end — pass it to pgx pools in tests. Source: Go 1.24 release notes, <https://go.dev/doc/go1.24>
- **`testing/synctest` is stable as of Go 1.25** (graduated from the 1.24 `GOEXPERIMENT`): `synctest.Test` runs goroutines in a bubble with virtualized time — relevant only if you build time-dependent logic (reservation expiry, stock-alert debouncing); ignore until then. Source: <https://go.dev/doc/go1.25>
- **testcontainers-go**: v0.44.0 (Aug 7, 2026), actively maintained. Source: <https://github.com/testcontainers/testcontainers-go>. Its Postgres module starts a real Postgres per test package via `postgres.Run(ctx, "postgres:16-alpine", ...)`, and — the killer feature for migration-heavy tests — supports **snapshot/restore** so each test resets to a clean migrated state without container restarts (`postgres.WithSQLDriver("pgx")` to speed it up). Source: <https://golang.testcontainers.org/modules/postgres/>
- vs plain docker-compose: compose means out-of-band lifecycle (developers must remember `docker compose up`, CI needs orchestration, port conflicts, stale state). Testcontainers keeps the database lifecycle inside `go test`, which is the boring option operationally even though it's one more dependency — and for a multi-tenant system, integration tests that prove tenant isolation against real Postgres (RLS or `WHERE tenant_id` behavior) are security-critical, not optional.

**Rationale**: stdlib for everything in-process; one well-maintained dependency to make `go test ./...` self-contained against real Postgres. No assertion library needed — `if got != want` and `go-cmp` only if diffs get painful.

---

## Summary rationale, per YAGNI

The whole stack is **four direct dependencies**: `jackc/pgx/v5`, `pressly/goose/v3`, `testcontainers-go` (test-only), and `sqlc` (build-time tool, not even a runtime import). Everything else — routing, middleware, config, JSON, testing — is Go 1.26 stdlib. Every dependency chosen shipped a release in the last four months and follows stdlib interfaces, so each is individually replaceable. The two genuinely close calls are chi (adopt when route groups hurt) and caarlos0/env (adopt when the config struct gets big); both migrations are mechanical, which is precisely why deferring them is safe.
