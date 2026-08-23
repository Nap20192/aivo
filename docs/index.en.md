# AIVO RMS

Restaurant management platform: a digital menu with table-side ordering, a
POS for waiters, and an admin panel with an AI assistant. A restaurant
registers itself, gets its own address (`/{slug}` or a custom domain), and
styles the menu to match its brand.

## Where to go

- **[For restaurants](user/owner.md)** — registration, menu, themes, QR tables, subscription, guest CRM.
- **[For waiters](user/waiter.md)** — shifts, tickets, accepting orders by code.
- **[For developers](dev/setup.md)** — running locally, architecture, API.

## How it works

A guest scans the QR code on the table → opens the restaurant's menu →
builds a cart → shows a short code to the waiter. The waiter accepts the
code in the POS — the items land on the table's ticket. At the end of the
shift the till reconciles automatically.
