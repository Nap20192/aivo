## Purpose

Defines pos's behavior as an independently deployable service: the existing shift/ticket/payment REST surface and its `TicketClosed` outbox edge to inventory (already specified), now also reaching ledger and menu over gRPC/outbox instead of in-process calls, and provisioning its own default payment methods off the restaurant-provisioning saga.

## ADDED Requirements

### Requirement: pos's REST API surface is behavior-preserving
Every endpoint callers use today under pos (shifts, tickets, cash operations, payment acceptance) SHALL continue to exist with the same request/response shape and business behavior when served by `cmd/aivo-pos` on its own port, rather than by `cmd/aivo-server`.

#### Scenario: Existing caller against the new service
- **WHEN** the POS frontend calls a pos endpoint it called before this change
- **THEN** the response shape and business outcome are unchanged, only the serving process/port differ

### Requirement: pos provisions default payment methods by consuming RestaurantCreated
pos SHALL seed a new restaurant's default payment methods by consuming the `RestaurantCreated` outbox event (see `service-events`) rather than via `provisioning.RestaurantProvisioner` being invoked in platform's transaction.

#### Scenario: Restaurant created
- **WHEN** pos receives a `RestaurantCreated` event for a restaurant it has not provisioned before
- **THEN** it creates that restaurant's default payment methods in its own transaction

### Requirement: pos's ticket lines reference menu items without a cross-schema foreign key
`ticket_lines.menu_item_id` SHALL be a plain identifier with no foreign-key constraint into menu's `menu_items` table, the same "no FK, cross-context" convention already used for inventory's `menu_item_id`, since pos and menu are independently deployable and menu items can only be validated by calling menu.

#### Scenario: A ticket line is created
- **WHEN** pos records a ticket line for a menu item
- **THEN** the menu item's existence and validity were already confirmed via menu's gRPC surface at the time the line was added, not by a database constraint
