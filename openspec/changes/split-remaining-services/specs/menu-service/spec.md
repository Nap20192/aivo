## Purpose

Defines menu's behavior as an independently deployable service: the existing menu/table/service-request REST surface, owner of the `restaurants` table other domains depend on, now running as its own binary and serving pos's menu-item/table reads over gRPC instead of an in-process bridge.

## ADDED Requirements

### Requirement: Menu's REST API surface is behavior-preserving
Every endpoint callers use today under menu (menu items, categories, tables, service requests, restaurant records) SHALL continue to exist with the same request/response shape and business behavior when served by `cmd/aivo-menu` on its own port, rather than by `cmd/aivo-server`.

#### Scenario: Existing caller against the new service
- **WHEN** the admin frontend or pos calls a menu endpoint it called before this change
- **THEN** the response shape and business outcome are unchanged, only the serving process/port differ

### Requirement: Menu remains the owner of restaurants; other services reference it across schemas
`restaurants` SHALL remain owned by menu's schema, not moved. Other domains' foreign keys into `restaurants` SHALL continue to be enforced as cross-schema foreign keys on the shared Postgres instance, the same pattern inventory already uses.

#### Scenario: Another service writes a row referencing a restaurant
- **WHEN** platform, ledger, or pos inserts a row that references a restaurant ID
- **THEN** the foreign key constraint against menu's `restaurants` table is enforced by Postgres across schemas

### Requirement: Menu creates a restaurant's default data by consuming RestaurantCreated
Menu SHALL create the `restaurants` row and seed a new restaurant's default menu by consuming the `RestaurantCreated` outbox event published by platform (see `service-events`), committing that write in its own transaction as part of platform's provisioning saga.

#### Scenario: Organization created, restaurant provisioning begins
- **WHEN** menu receives a `RestaurantCreated` event carrying a restaurant ID it has not provisioned before
- **THEN** it creates the `restaurants` row and default menu items/categories in its own transaction

### Requirement: pos reads menu items and tables from menu over gRPC, not an in-process call
pos's reads of menu items, tables, and service-request state, and its writes to service requests, SHALL go through a gRPC call to `cmd/aivo-menu` rather than pos's in-process `menubridge` calling menu's application layer directly. pos's application code MUST NOT import menu's domain package.

#### Scenario: pos looks up a menu item while placing an order
- **WHEN** pos needs a menu item's price and name to add an order line
- **THEN** it calls menu's gRPC surface and receives the same data the in-process bridge returned before this change

#### Scenario: menu is unreachable while pos is taking an order
- **WHEN** pos calls menu's gRPC surface and menu is unreachable
- **THEN** pos returns a clear error to the caller for that specific menu lookup, without pos itself becoming unavailable for unrelated operations
