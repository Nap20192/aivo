# Domain events

Catalog of domain events the platform will raise. Storage: the `events`
outbox table (platform migration `0004_events.sql`), written in the same
transaction as the aggregate change. Go side: `internal/sharedkernel`
(`DomainEvent`, `AggregateRoot.Raise`). None are wired yet — this is the
contract to implement against.

ERD: [docs/erd.svg](./erd.svg).

## Platform context

| Event | Aggregate | Payload | Raised when |
|---|---|---|---|
| `OrganizationRegistered` | organization | org_id, name, owner_email | self-registration completes |
| `SubscriptionChanged` | subscription | org_id, plan, status (old → new) | plan upgrade/downgrade or status transition |
| `RestaurantProvisioned` | restaurant | restaurant_id, org_id, slug | new restaurant created |
| `ThemeApplied` | theme | restaurant_id, source (`manual` \| `ai_proposal`) | theme saved to a restaurant |
| `CustomerRegistered` | customer | customer_id, email | diner account created |

## Menu context

| Event | Aggregate | Payload | Raised when |
|---|---|---|---|
| `OrderPlaced` | order | order_id, restaurant_id, table_id, total_cents, customer_id? | diner submits an order |
| `ServiceRequested` | service_request | request_id, table_id, kind | call-waiter / request-bill pressed |
| `HandoffCreated` | cart_handoff | handoff_id, table_id, code, total_cents | diner generates a QR handoff code |
| `HandoffAccepted` | cart_handoff | handoff_id, ticket_id, accepted_by | waiter redeems the code into a ticket |

## POS context

| Event | Aggregate | Payload | Raised when |
|---|---|---|---|
| `ShiftOpened` | shift | shift_id, restaurant_id, opened_by, float_cents | waiter/manager opens a shift |
| `ShiftClosed` | shift | shift_id, declared_cents, expected_cents, variance_cents | shift closed (immutable after) |
| `TicketOpened` | ticket | ticket_id, shift_id, table_id | table ticket started |
| `LinesFired` | ticket | ticket_id, line_ids | lines sent to kitchen |
| `TicketClosed` | ticket | ticket_id, total_cents, customer_id? | ticket paid/closed — feeds guest CRM spend |

## Conventions

- Names: past tense, `PascalCase`, stable once published (consumers depend on them).
- `payload` is flat JSON, IDs as strings; no nested aggregates — consumers re-read state they need.
- `restaurant_id` column set for tenant-scoped events; NULL only for org-level (`OrganizationRegistered`, `SubscriptionChanged`).
- Publisher (когда появится): poll `events_pending`, deliver, set `published_at`; at-least-once, consumers idempotent by `id`.
