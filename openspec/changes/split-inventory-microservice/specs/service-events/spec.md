## Purpose

Defines the outbox/eventing contract that lets services publish domain events durably and deliver them to another service at least once, without a message broker, so cross-service side effects (like posting a GL entry after a sale) happen reliably even though they're no longer in the same database transaction as the write that triggered them.

## ADDED Requirements

### Requirement: Events are published in the same transaction as the write that causes them
A service SHALL insert an outbox row into its own `events` table in the same database transaction as the business write it represents. If the transaction rolls back, no event row exists. If it commits, the event row is guaranteed to exist and will eventually be delivered.

#### Scenario: Business write succeeds
- **WHEN** pos closes a ticket and the ticket-close transaction commits
- **THEN** a `TicketClosed` event row exists in pos's `events` table with the ticket ID as its source document ID

#### Scenario: Business write fails
- **WHEN** pos attempts to close a ticket and the transaction fails/rolls back for any reason
- **THEN** no `TicketClosed` event row is created

### Requirement: Unpublished events are delivered at least once
A background poller in the producing service SHALL periodically scan its `events` table for rows with no `published_at`, and attempt delivery to the configured consumer for each. A row is marked `published_at` only after the consumer acknowledges receipt. A delivery attempt that fails (network error, consumer error response, timeout) SHALL be retried with backoff; the row remains unpublished until a delivery succeeds.

#### Scenario: Consumer is reachable
- **WHEN** the poller finds an unpublished event and the consumer's RPC returns success
- **THEN** the event row is marked `published_at` with the current time and is not retried again

#### Scenario: Consumer is temporarily unreachable
- **WHEN** the poller attempts delivery and the consumer is unreachable or returns an error
- **THEN** the event remains unpublished and is retried on a later poll with backoff, until it succeeds

### Requirement: Delivery is idempotent on the consumer side
Every event carries a stable idempotency key equal to the existing domain document ID it concerns (e.g. ticket ID, receipt ID, write-off ID, stocktake ID). A consumer receiving an event whose key matches one it has already processed SHALL treat the delivery as a no-op — it MUST NOT apply the event's effect twice.

#### Scenario: First delivery of an event
- **WHEN** a consumer receives an event with a source document ID it has not seen before
- **THEN** it applies the event's effect (e.g. posts a GL journal entry) and records the source document ID as processed

#### Scenario: Redelivery of an already-processed event
- **WHEN** a consumer receives an event with a source document ID it has already processed (e.g. because the producer retried after a delivery it couldn't confirm)
- **THEN** it does not apply the effect again and still returns a success acknowledgement to the producer

### Requirement: pos publishes a TicketClosed event on ticket close instead of calling inventory in-process
Closing a ticket SHALL commit independently of inventory's availability. Stock consumption for the sale SHALL happen asynchronously, triggered by inventory consuming the `TicketClosed` event.

#### Scenario: Ticket closed while inventory service is down
- **WHEN** a ticket is closed and `cmd/aivo-inventory` is unreachable
- **THEN** the ticket-close transaction still commits successfully and the ticket is marked closed
- **AND** stock is consumed once inventory becomes reachable again and processes the pending event

### Requirement: inventory publishes an event for every GL posting it needs from ledger
Every inventory action that today results in a ledger journal entry — sale COGS, receipt, write-off, stocktake adjustment, and reversal of any of those — SHALL publish an outbox event rather than calling ledger in-process, since ledger is not in-process for inventory once inventory is its own service.

#### Scenario: Receipt posted while ledger is unreachable
- **WHEN** a storekeeper posts a receipt document and `cmd/aivo-server`'s gRPC listener is unreachable
- **THEN** the receipt's stock movement still commits successfully in inventory
- **AND** the corresponding GL entry posts once ledger becomes reachable and processes the pending event
