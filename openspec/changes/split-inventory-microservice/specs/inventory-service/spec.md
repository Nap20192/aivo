## Purpose

Defines inventory's behavior as an independently deployable service: the same REST surface the admin frontend already relies on, now authenticated by a verifiable token instead of an in-process session lookup, plus its role as an event consumer/producer for the two edges it shares with pos and ledger.

## ADDED Requirements

### Requirement: Inventory's REST API surface is behavior-preserving
Every endpoint the admin frontend calls today under inventory (products, tech cards, receipts, write-offs, stocktakes, suppliers) SHALL continue to exist with the same request/response shape and business behavior when served by `cmd/aivo-inventory` on its own port, rather than by `cmd/aivo-server`.

#### Scenario: Existing frontend call against the new service
- **WHEN** the admin frontend calls an inventory endpoint it called before this change (e.g. list products, create a receipt)
- **THEN** the response shape and business outcome are unchanged, only the serving process/port differ

### Requirement: Inventory verifies a caller's token locally, without calling platform per request
`cmd/aivo-inventory`'s REST handlers SHALL authenticate requests by verifying a signed token's signature and claims locally (using a public key), and MUST NOT perform a network call to platform to validate each request.

#### Scenario: Valid token
- **WHEN** a request arrives with a token signed by the trusted signer, not expired, and scoped to a tenant/restaurant the caller belongs to
- **THEN** the request is authorized without any call to platform

#### Scenario: Invalid or expired token
- **WHEN** a request arrives with a token that fails signature verification, is expired, or is scoped to a different tenant than the resource being accessed
- **THEN** the request is rejected as unauthorized

#### Scenario: Platform is unreachable
- **WHEN** platform (`cmd/aivo-server`) is down but a caller already holds a valid, unexpired token
- **THEN** inventory continues to authorize and serve requests normally

### Requirement: Sale-triggered stock consumption is driven by the TicketClosed event, not a direct call from pos
Inventory SHALL consume stock for a sale by processing the `TicketClosed` event (see `service-events` capability) rather than by exposing an in-process call pos invokes synchronously.

#### Scenario: Normal sale
- **WHEN** inventory receives a `TicketClosed` event for a ticket it has not processed before
- **THEN** it deducts the sold items' stock per the recipe/tech-card in effect and publishes its own event for the resulting COGS posting

### Requirement: Every inventory action that posts to the ledger does so via an outbox event
Sale COGS, receipt, write-off, stocktake, and their reversals SHALL each publish an outbox event carrying enough data for ledger to construct the correct journal entry, rather than calling ledger's posting API in-process.

#### Scenario: Receipt posting
- **WHEN** a receipt document is posted in inventory
- **THEN** the receipt's stock movement commits in the same transaction as an outbox event for its GL entry, and the GL entry itself is posted asynchronously by ledger consuming that event
