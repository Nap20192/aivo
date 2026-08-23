# Context Map

## Contexts

- [Menu](./backend/internal/menu/CONTEXT.md) — diner-facing digital menu, ordering, and landing page for a single restaurant

## Relationships

_None yet — Menu is the first context. Future satellite services (backoffice, POS, waiter app) will be added here as their contexts are charted, along with how they relate to Menu (e.g. Backoffice configures Menu's landing/content; POS/till integration with Menu orders is explicitly deferred — see `backend/internal/menu/docs/adr/0002-menu-order-decoupled-from-pos.md`)._

## Code layout

All satellite services share one Go module at the repo root (module `aivo`) rather than one module per service — see `README.md`. One command per binary under `cmd/<service>-<binary>/`, one domain-scoped package tree per service under `backend/internal/<service>/`, static frontends under `frontend/<service>/`. A service's context docs (`CONTEXT.md`, `docs/adr/`) live beside its code at `backend/internal/<service>/`.

Domain model lives apart from the services: `backend/internal/sharedkernel/` holds the DDD building blocks shared by every context (ID, Entity, AggregateRoot, DomainEvent), and `backend/internal/domain/{platform,menu,pos}/` holds each context's business entities. Contexts (`backend/internal/<service>/{app,ports,adapters}`) import their domain package; domain packages import only `backend/internal/sharedkernel` and the standard library.
