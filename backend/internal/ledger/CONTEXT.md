# Ledger

The general-ledger (GL) context: an append-only, double-entry accounting core
consumed by the POS context through a synchronous in-process port. Owns the
chart of accounts, journal documents, and the per-restaurant GL-semantics
configuration. Excludes anything that produces journal sources (shifts, stock,
purchases) — those live in their own contexts and hand the ledger a draft to
post. Increment-1 scope: money and live shift acceptance (see
`docs/research/rms/impl-contract.md`).

## Language

**Account**:
A chart-of-accounts entry (code, name, type, normal side). Only a `postable`
leaf account takes lines. Type/normal-side are frozen once the account has a
posting.
_Avoid_: category, bucket.

**Cost center**:
A flat per-restaurant dimension on every line (seed: one `main`). No tree, no
allocation engine until a named requirement.

**Journal document**:
The aggregate root — a balanced set of one-sided lines with a lifecycle
**draft → posted → cancelled** (the posting gate, D4) and two dates:
`accounting_date` (business date of the fact) and `recorded_at` (wall clock of
the record) — D7. A posted document is immutable.
_Avoid_: transaction, entry (ambiguous with line).

**Journal line**:
One side of a document: strictly a debit **or** a credit, amount > 0, carrying
its account and cost center. Append-only once the document is posted.

**Reversal (storno)**:
The only correction path (D1): cancelling a posted document creates a new
`reversal` document with mirrored lines, revalidated at the current date (a
closed period never blocks its own reversal — refuted §15.1), and marks the
original cancelled. The original is never edited.

**Balance / auto-balance**:
Σ debits = Σ credits per document. A residual difference is caught by a single
balancing line on the `rounding_unassigned` account (a safety net, not an
error). Manual journals reject an imbalance instead.

**Account map (GL semantics)**:
Per-restaurant `purpose → account` configuration (e.g. `tender:cash → 1000`).
Changing it changes what posting does — this is the per-deployment GL-treatment
knob (refuted §15.6), not a fixed property of the system.

**Period gate**:
`periodOpen(restaurant, accounting_date)` guards posting. A stub returning
`true` in increment-1; the close-snapshot (D8) lands later. The extension point
is fixed.

## Boundaries

- Consumed by **POS** through `pos/ports.Ledger` (implemented by
  `pos/adapters/ledgerbridge`): POS builds and posts a shift-acceptance draft in
  its own transaction (a documented cross-context `*sql.Tx` — both tables live in
  one Postgres monolith). Reversals and account-map edits are ledger back-office
  operations, called directly by the HTTP layer, not through the bridge.
- Structure: `internal/ledger/{app,ports,adapters/postgres}` with the domain in
  `internal/domain/ledger` (repo convention: domain packages live under
  `internal/domain/<ctx>`).
