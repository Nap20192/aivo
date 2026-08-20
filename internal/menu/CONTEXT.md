# Menu

The diner-facing digital menu, ordering, and landing page for a single restaurant, reached via a per-table link. Excludes staff/admin configuration UI (owned by the future Backoffice context) and POS/till/payment processing (owned by the future POS context).

## Language

**Restaurant**:
The tenant-owning entity that self-registers and runs a Menu.
_Avoid_: Tenant, account, business — "tenant" is a technical isolation term, not domain language.

**Table**:
A physical dining table at a Restaurant, identified by its table link.

**Table link**:
The per-table URL a diner scans (as a QR code, rendered on demand — never stored) to enter a Restaurant's Menu: `{restaurant_slug}/t/{table_token}`, where the restaurant slug is a readable, non-sensitive identifier and the table token is a ~128-bit random, unguessable value regenerable by staff (regeneration invalidates the old token immediately, e.g. after a lost/compromised QR).
_Avoid_: QR code (that's the encoding, not the concept), menu link.

**Menu**:
A Restaurant's set of Categories and Menu items.

**Category**:
A named grouping of Menu items within a Menu.

**Menu item**:
A single purchasable dish or drink, with a name, description, price, image, a set of Option groups, zero or more Allergens, and an availability toggle (86'd).

**Option group**:
A named set of choices on a Menu item (e.g. "Size", "Add-ons"), single-select or multi-select, where each Option carries a price delta.

**Option**:
One choice within an Option group, with a label and a price delta applied on top of the Menu item's base price.

**Allergen**:
A tag on a Menu item drawn from a fixed set (the standard EU 14 allergen categories), not free text — lets diners filter/search by allergen.

**Order line**:
A snapshot of a Menu item (name, price, chosen Options) captured at the moment an Order is submitted, plus a reference to the source `menu_item_id`. A later price or content change to the Menu item never retroactively alters an existing Order line.

**Cart**:
A diner's in-progress selection of Menu items before submitting an Order. Ephemeral and client-side only — never persisted server-side.
_Avoid_: Basket, order (before submission it is not yet an Order).

**Order**:
A diner's submitted Cart, persisted as a record made of Order lines plus one optional free-text comment (diner-wide, not per line, e.g. "no onion on anything"). Carries no payment fields — payment happens in person, outside the Menu context.
_Avoid_: Ticket, check.

**Service request**:
A diner-initiated action with no items, e.g. "call waiter" or "request bill". Distinct from Order — different fields, different lifecycle (`pending → acknowledged` vs an Order's fulfillment states).
_Avoid_: Order (an Order always has items; a Service request never does).

**Notification channel**:
A Restaurant's configured way to receive alerts about new Orders and Service requests. "Telegram bot" is the current concrete channel type, modeled as a named subtype so a second channel type later isn't a rename.

**Landing page**:
The diner-facing entry page for a Restaurant's Menu: toggleable/reorderable sections and on-brand restyling, built from a fixed catalog of Landing blocks. Distinct from Menu — it's presentation/entry, not the item catalog.

**Landing block**:
One placeable unit on a Landing page, from a closed catalog (no page builder): **Banner** and **Free-text** (repeatable — a Restaurant may place several), and **Opening hours**, **Location** (address + external map link), **Social links**, **Contact** (phone, click-to-call) (single instance each — these are facts about the Restaurant, not repeatable content).

**Diner session**:
A lightweight, anonymous token (httpOnly secure cookie, ~4-6h sliding TTL) issued silently when a diner opens a Table link. Not an account or login — exists only to rate-limit and dedupe Order/Service request submissions: Order submits are debounced per session (max 1 per 30s); Service requests are deduped per Table, not per session (max one open request of a given kind per Table at a time, since multiple diners share a Table). A fixed-window IP-level rate limit backstops both against scripted abuse.

**Staff account**:
A restaurant staff member's login, used to configure Menu content and the Landing page. Detailed ownership (roles, permissions) belongs to the future Backoffice context — Menu only depends on "a Staff account exists and can be authenticated."
