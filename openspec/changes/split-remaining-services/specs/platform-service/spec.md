## Purpose

Defines platform's behavior as an independently deployable service: the existing org/user/session/restaurant-provisioning REST surface, unchanged for callers, now running as its own binary and remaining the sole issuer of `aivo-auth` tokens and the initiator of restaurant provisioning.

## ADDED Requirements

### Requirement: Platform's REST API surface is behavior-preserving
Every endpoint callers use today under platform (organization signup, session login/logout, restaurant creation, restaurant settings) SHALL continue to exist with the same request/response shape and business behavior when served by `cmd/aivo-platform` on its own port, rather than by `cmd/aivo-server`.

#### Scenario: Existing caller against the new service
- **WHEN** a caller invokes a platform endpoint it called before this change (e.g. login, create restaurant)
- **THEN** the response shape and business outcome are unchanged, only the serving process/port differ

### Requirement: Platform remains the only issuer of session cookies and the only caller of Mint
Platform SHALL continue to own the `users`/`sessions` tables and the existing session-cookie login flow, and SHALL remain the only service that calls `aivo-auth`'s `Mint` RPC. Splitting platform out of `cmd/aivo-server` MUST NOT change this.

#### Scenario: A downstream service needs a token
- **WHEN** ledger, pos, or menu needs a caller to hold a verifiable token for a service-to-service or frontend-to-service call
- **THEN** the token is obtained via platform calling `Mint`, never by another service calling `aivo-auth` directly

### Requirement: Platform initiates restaurant provisioning as a saga, not a shared transaction
Creating an organization or a restaurant SHALL commit platform's own rows (and, for restaurant creation, menu's `restaurants` row and default menu) in platform's own transaction, then publish a `RestaurantCreated` outbox event that menu, ledger, and pos each consume to provision their own default data (see `service-events`).

#### Scenario: Restaurant created while ledger is unreachable
- **WHEN** a restaurant is created and `cmd/aivo-ledger` is unreachable
- **THEN** the restaurant is created successfully and immediately usable for menu/ordering purposes
- **AND** ledger's default accounts/cost centers are seeded once ledger becomes reachable and processes the pending event

### Requirement: Restaurant ownership stays with menu; platform references it across schemas
`restaurants` SHALL remain owned by `menu`'s schema. Platform's tables that reference a restaurant (e.g. `restaurant_themes`, `custom_domains`) SHALL continue to do so via a cross-schema foreign key on the shared Postgres instance, the same pattern already used by inventory's foreign keys into `restaurants`/`users`.

#### Scenario: Platform writes a row referencing a restaurant
- **WHEN** platform inserts a row that references a restaurant ID
- **THEN** the foreign key constraint against menu's `restaurants` table is enforced by Postgres across schemas, not re-validated in application code
