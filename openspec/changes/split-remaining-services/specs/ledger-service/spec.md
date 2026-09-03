## Purpose

Defines ledger's behavior as an independently deployable service: the existing `LedgerService` gRPC surface (already called by inventory) plus any REST surface ledger has, now running as its own binary with its own schema, provisioning its own default accounts off the restaurant-provisioning saga instead of an in-process shared transaction.

## ADDED Requirements

### Requirement: Ledger's gRPC and REST surfaces are behavior-preserving
`LedgerService`'s existing RPCs (posting and reversing COGS/receipt/write-off/stocktake journals) and any REST endpoints ledger serves today SHALL continue to exist with the same request/response shape and business behavior when served by `cmd/aivo-ledger` on its own port, rather than by `cmd/aivo-server`.

#### Scenario: Existing caller against the new service
- **WHEN** inventory calls a `LedgerService` RPC it called before this change
- **THEN** the response shape and business outcome are unchanged, only the serving process/port differ

### Requirement: Ledger provisions default accounts by consuming RestaurantCreated, not an in-process call
Ledger SHALL seed a new restaurant's default accounts, cost centers, and account map by consuming the `RestaurantCreated` outbox event (see `service-events`) rather than via `provisioning.RestaurantProvisioner` being invoked in platform's transaction.

#### Scenario: Restaurant created
- **WHEN** ledger receives a `RestaurantCreated` event for a restaurant it has not provisioned before
- **THEN** it creates that restaurant's default accounts, cost centers, and account map in its own transaction

### Requirement: pos's shift journal postings reach ledger via outbox, not an in-process call
Shift-close and shift-accept journal postings SHALL be delivered from pos to ledger as outbox events, replacing pos's in-process `ledgerbridge` call.

#### Scenario: Shift closed while ledger is unreachable
- **WHEN** a shift is closed and `cmd/aivo-ledger` is unreachable
- **THEN** the shift-close transaction still commits successfully in pos
- **AND** the shift's draft journal posts once ledger becomes reachable and processes the pending event
