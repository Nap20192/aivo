# RMS Business Logic — Full Reference for a New Project

Complete engineering reference distilled from a three-system comparative study
of restaurant/retail management platforms: one system's source code (executed
truth), one system's product documentation (documented intent), and one
system's external API contract (externally observed behavior). Business logic
only: entities, lifecycles, invariants, decisions, and rationale — no schema,
no DDL, no code, no source citations.

**Reading key**: for each domain — how each system models the problem, the
cross-system pattern by criterion (integrity/immutability, auditability,
costing correctness, backdating behavior, normalization, cross-module
consistency, extensibility, integration fitness, exception handling,
multi-unit/currency/tax), then a concrete decision for a new RMS with
rationale, then explicit anti-patterns not to copy.

Three reference systems are compared throughout, referred to by role:
- **the executing system** — a general-purpose ERP whose accounting/inventory/
  manufacturing core was read as source code; strongest on mechanics that are
  actually enforced, weakest on restaurant-specific behavior.
- **the documenting system** — a vertical restaurant back-office product,
  read from its own documentation; strongest on restaurant operations
  (cash handling, labor, forecasting), openly honest about its own
  compromises.
- **the contracting system** — a restaurant POS/RMS platform, read from its
  external API contract only; strongest on restaurant-specific temporal
  modeling (recipes, pricing) and on what it exposes as an integration
  surface, but its internal mechanics are mostly invisible — a gap in the
  corpus, not necessarily a gap in the product.

---

## 1. Eight Cross-Cutting Decisions

These recur across every domain below and should be treated as platform-level
commitments made once, not re-decided per domain.

**D1 — The ledger is append-only.** No update, no delete on posted accounting
facts (GL entries, stock ledger entries, payment ledger entries). Corrections
are reversal entries dated at correction time, never edits to history. This is
the executing system's core discipline, and it is what lets integrity and
traceability hold structurally instead of by convention. *Caveat*: the closing
mechanism itself is a privileged, explicitly-modeled exception to this rule
(see D8), not a hole in it.

**D2 — Stock valuation is a ledger, not a running total.** Every stock
movement is a dated, ordered, immutable fact; on-hand quantity and average
cost are derived, not stored as truth. A formalized, resumable, idempotent
recompute mechanism (repost) is a first-class system component, not a
maintenance script. Periodic accounting (the documenting system's default) is
documented to lose precision when an end-of-month invoice is missed — the
COGS of the following period silently absorbs the error.

**D3 — Payments are a projection of the ledger.** Outstanding/paid status is
computed from a payment ledger tied back to GL, never tracked as a second,
independently-mutable source of truth. This is what keeps AP/AR aging from
drifting from the books — the documenting system's own documented failure
mode ("a journal entry posted straight to Accounts Payable breaks AP aging")
is exactly what this decision prevents.

**D4 — Documents have an explicit three-state lifecycle with a submit gate.**
Draft → posted → cancelled. Nothing touches the ledger before the gate;
cancellation reverses, never deletes. The contracting system's alternative —
write, then "unprocess" and reimport with no observable reversal trail — is
explicitly not adopted; keep the gate + reversal shape everywhere a document
produces ledger effects.

**D5 — Recipes are calendar-versioned.** A recipe is valid over a date range
(at most one version active per accounting day, no artificial expiry on the
current version). Costing and food-cost analysis must be able to answer "what
recipe was active on date X," not just "what is it now" — this is the one
place where the contracting system's model is strictly ahead of both the
executing system (versions are separate documents with a manually toggled
default flag, no calendar binding) and the documenting system (no recipe
versioning observed at all).

**D6 — A shift is an accounting document, not a UI session.** Open → Closed →
Accepted. The acceptance step reviews and can adjust line items before the
shift's totals become GL fact. No single source system gets this whole chain
right — it is a synthesis: line-by-line acceptance-before-posting, a
cash-drawer-to-bank reconciliation chain, and transactional opening/closing
integrity (one open shift per till and per cashier, symmetric cancellation).

**D7 — Every fact carries two dates.** The accounting/business date the fact
belongs to, and the real timestamp it was actually recorded. This single
field is what makes backdating auditable after the fact even when the system
does not forbid it — cheap to add, high-value, and stronger than a role-gated
"can backdate" permission with no trace of when the backdated document was
actually created.

**D8 — Period closing is a snapshot document, not a toggle.** Closing
produces a persisted closing-balance record; the next period's opening
balances derive from it. This is the only mechanism in the corpus that gives
a materialized, re-derivable snapshot of "what the books said at close." A
Closed Date-style toggle is a real and useful day-to-day input gate (worth
keeping as the operational layer) — but paired with, not instead of, a
snapshot: a dynamically-recomputed retained-earnings figure that can never be
pinned to a point in time is a genuine weakness, not just a simplification.

---

## 2. Domain 1 — General Ledger & Accounting

### Model
A posting is a document-generated fact tied to an account, a set of fixed
analytical dimensions, and two dates (D7). One-sided posting (each line has a
single debit or credit, sum-of-debits equals sum-of-credits per document,
auto-balanced with a rounding line) is the strongest model observed. An
auto-generated "unassigned" line absorbing document-level imbalance is a
useful safety valve worth keeping. A two-sided posting style (account +
contra-account carrying both quantity and amount in one line) is compact but
conflates monetary and quantity tracking in a single record and works against
multi-currency — not adopted as the core model.

### Lifecycle & posting moment
GL effect should occur at a single, explicit approval/submit moment — never
at save, never implicitly at document completion. Editing after that moment
should never silently rewrite the ledger; it should trigger a formal,
tracked recompute or require an explicit reversal.

### Chart of accounts
A hierarchical chart with per-role freeze is standard. One system's addition
is worth adopting: **Statistical Accounts** — non-monetary, one-sided
postings (e.g. guest counts) that ride in the same book without affecting the
balance and are allowed even in closed periods, useful for restaurant
per-guest metrics that need to live next to the money. A warehouse should not
be modeled as a special case of a GL account — that conflation blocks
multi-currency and multiple per-side analytics.

### Manual entries & outstanding balances
A full manual journal-entry capability (opening entries, multi-currency,
deferred recognition, inter-company entries) plus a separate,
formally-reposted payment-ledger register derived from GL is the strongest
combination observed. Automatic Due-To/Due-From balancing per legal entity
for intercompany entries, so debits equal credits within each legal entity
without manual intervention, is worth adopting as a special case of a general
dimension-offsetting mechanism.

### Period closing & backdating
The strongest closing model combines: a company-wide frozen date with no
superuser exemption; a period-level gate with an explicitly exempted role;
and a hard wall after year-end close, past which entries at or before the
close date are rejected. One honest caveat worth designing around explicitly:
the closing document itself must be exempt from the check it enforces, and
cancelling an old voucher under an immutable-ledger policy should revalidate
at the *current* date so it doesn't get stuck. Editing a submitted document
should auto-trigger a formalized repost job, not fail silently or corrupt
downstream balances.

A softer, reversible per-module status gate (open/limited/closed) is a good
day-to-day operational layer *in addition to* the hard year-end wall — it
absorbs the reality of iterative month-end closes without weakening the
year-end guarantee.

A bare "permission to backdate" gate with no period concept at all is the
weakest option; it should be compensated for by the dual-date field (D7) so
backdated postings remain discoverable after the fact even when not
forbidden outright.

### Year-end close
A snapshot document (closing voucher + closing-balance record) that survives
as a materialized, re-derivable record of "what the books said at close" is
required. A purely dynamic, continuously-recomputed retained-earnings figure
with no closing document at all is elegant (nothing to get wrong, no
duplicate-closing risk) but cannot answer "what did the books say on this
past date" — a real weakness for audit and historical reporting.

### Dimensions
Fully extensible, runtime-attachable dimensions (arbitrary custom fields
propagated across every accounting document type, with per-account mandatory
policy and automatic offsetting-entry balancing) are powerful but expensive.
A fixed, small set of dimensions (unit/location, cost center, plus a handful
of restaurant-specific axes) is the pragmatic default — see the
overcomplications section for when *not* to go fully extensible.

### POS → GL translation
Two patterns are worth combining: an aggregate translation step that turns a
day-times-location's sales activity into a small number of journal entries
(sales, labor, statistical) with an explicit balancing-adjustment line for
any unmapped-tender imbalance; and a human-in-the-loop shift-acceptance
document that lets the back office override individual account/counterparty
assignments on shift-level postings before they become fact.

### Decision for a new RMS
1. Core ledger mechanics: append-only GL as the *only* mode (not an optional
   "immutable" toggle), reversal instead of edit, auto-rounding to a
   dedicated account, a separate payment-ledger register with formalized,
   per-voucher-checkpointed repost auto-triggered on dimension edits.
2. Dual-date posting (D7) on every GL entry.
3. Hybrid closing: a per-module open/limited/closed status as the day-to-day
   gate, plus a hard year-end snapshot document as the record of truth.
4. POS→GL translation: aggregate day×location journal entries with an
   explicit balancing-adjustment line, plus a shift-acceptance document as
   the human control point before posting.
5. Intercompany auto-balancing implemented as a special case of a general
   dimension-offsetting mechanism — one mechanism instead of two.
6. Statistical Accounts for non-financial, one-sided facts that need to live
   in the same book, postable even in closed periods.

### Anti-patterns (do not copy)
- A closing status reversible at any moment with no trace that it was
  reversed.
- Editing a posted document by unprocessing and reimporting it with no
  observable reversal record.
- Allowing an "edit approved entry" path for users with the right
  permission set before reconciliation — it defeats append-only in practice
  even when approval nominally locks the record.

---

## 3. Domain 2 — Stock, Valuation & Costing

### Model
A perpetual model — an atomic ledger entry per movement, carrying both
quantity and cost, valuation computed and re-derivable from history (FIFO /
Moving Average / LIFO / Standard Cost, resolved at item, then company, then
global default) — is the strongest observed. A periodic model — COGS debited
at purchase, corrected only by inventory counts, no ledger concept at all —
is materially weaker for cost accuracy. A *hidden* perpetual model — movements
exist and are observable transaction-by-transaction, but records are
read-only and the valuation engine (a single, non-configurable weighted
average) is not independently verifiable — sits between the two: good
ergonomics, poor auditability of the cost calculation itself.

### Backdating & repost — the central differentiator
Three fundamentally different answers to "the invoice arrived a week late":
- **Full pipeline execution**: detect future-dated ledger entries downstream
  of the correction, create a formal repost job, recompute in the background
  with checkpoints, deduplication, advisory locks, and time-slot batching.
  One elegant refinement: items using a standard (non-fluctuating) cost method
  are explicitly *exempted* from repost by forbidding backdated posting for
  them outright — cost is order-invariant for that valuation method, so cheap
  accounting is bought by restricting input instead of recomputing.
- **Manual workaround**: no repost mechanism at all; correction is a manual
  refresh-and-reapprove cycle. The cost of this choice is usually documented
  honestly: a missed end-of-month invoice inflates the following period's
  COGS.
- **Opaque server-side recompute**: only a target date and a document
  reimport/unprocess mechanism are exposed; the server recomputes downstream
  movements implicitly with no way to observe or control it through the API
  (indirect evidence of correction activity sometimes leaks through as a
  distinct transaction type in reporting).

### Inventory counts, reservations, closing
The strongest combination: absolute-quantity/rate reconciliation documents
that repost all future ledger entries; a materialized on-hand record holding
actual/reserved/ordered/projected quantities; batch and serial tracking with
expiry where needed; and a real closing-snapshot document that freezes ledger
entries at a point in time. A server-computed inventory count — the server
calculates expected quantity and variance and posts automatically to
shortage/surplus accounts — is the best ergonomic pattern observed, especially
paired with a dry-run endpoint that computes the same result without saving,
eliminating a whole class of client-side calculation bugs.

### Landed cost
Additional charges on a receipt (freight, customs, etc.) should be
distributable across the receipt's lines with a formal repost of the
affected entries — and, critically, this distribution must be *writable*
through any exposed integration surface, not read-only. A read-only
allocation-method field that the client can see but not drive is a real
integration defect.

### Decision for a new RMS
1. Perpetual ledger as the core (D2) — the only model observed to make both
   quantity and cost provable back to a source document; periodic accounting
   is not adopted even as an option, given its documented precision loss.
2. Repost as an explicit, first-class entity — statuses, checkpoints,
   deduplication, time-slot batching, failure notification. Restaurant
   reality means backdated invoices are routine; making the cost of that
   visible and manageable (rather than silent or manual) is the point.
3. The standard-cost exemption pattern: for low-value/stable-cost items,
   forbid backdated posting instead of repost — a deliberately cheap default
   for the bulk of a restaurant's small-ticket goods.
4. Server-computed inventory count with a dry-run endpoint: automatic
   surplus/shortage posting plus a preview-without-save call.
5. Costing hygiene as an input-quality layer, not a replacement for the
   ledger: recompute only on approve, exclude zero/negative/stale lines from
   averaging, variance-percentage checks at count time. Without such a
   filter, the average is directly vulnerable to dirty invoice data (free
   items, credit lines).
6. Batch/expiry tracking as an optional module, not baseline — useful for
   alcohol/regulatory-tracking segments, unnecessary noise elsewhere.

### Anti-patterns (do not copy)
- Periodic accounting as the primary model (documented precision loss on
  missed counts).
- A server-side recompute that is not observable or controllable by the
  client.
- Reversible unapprove-and-edit with no ledger trace.

---

## 4. Domain 3 — Nomenclature & Recipes

### Model
A single item entity covering goods/services/assets, with role expressed via
flags, is workable but generic. Separating purchased items from recipes as
distinct entities is cleaner conceptually but costs flexibility when an
item's role needs to change. A single product entity with a mandatory,
closed type enum (goods/dish/prepared/service/modifier/external/rate) plus
independent classification axes for group, category, and accounting category
— serving three different consumers (menu, reports, GL) — is the most
practical compromise for a vertical restaurant product.

### Recipe versioning — see D5
A calendar-versioned technical card (at most one version active per
accounting day, the latest is open-ended, backdated creation closes the
previous version at midnight of the new start date) is the strongest model
for both auditability ("what recipe was active on date X" as a first-class
query) and backdating (retroactive recipe correction is a normal interval
creation, not a special case). Versions as separate documents with a manually
toggled "is default" flag, with no calendar binding, does not scale to a
kitchen that revises costings weekly and after the fact.

### Costing
The strongest costing engine executes multiple raw-material costing methods
with a fallback cascade and recursive upward propagation to parent recipes,
plus a scheduled mass recompute. One real defect to avoid: a recompute that
**mutates an already-submitted parent document's cost field in place** is a
deliberate compromise for cost currency, but it makes that field
non-auditable — a new RMS should keep recipe cost as a separate, append-only
time series, never a document field that silently mutates after submission.

Restaurant-specific costing hygiene worth adding on top: per-item cost-update
method options (weighted average over last count, weighted average over last
N receipts, last received, manual), recompute gated to approval time only,
exclusion of zero/negative/stale-dated lines from averaging, and per-location
cost override. A single, non-configurable weighted-average method that the
client cannot verify (only consume via a "getPrepared"-style computed
endpoint, because the reference formula does not account for
size/portion handling) is honest but limiting — acceptable as a default,
not as the only option.

### Consumption at sale
The recipe should own its own consumption strategy at the point of sale
(deplete the assembled ingredients, or deplete the finished product directly,
per organizational unit) rather than leaving depletion to a downstream sales
document. This closes two real gaps: a "theoretical" depletion model that
depends entirely on a recipe-to-menu-item mapping (useless without it), and a
model where the recipe plays no role at all in the sales cycle and stock only
moves at a later consolidation step.

### Decision for a new RMS
1. Calendar-versioned recipes (D5) as the primary recipe model.
2. Executable lifecycle invariants as code-level checks, not UI convention:
   lock valuation-affecting fields after the first stock movement, forbid
   recipe cycles, cascade item changes to prices/variants automatically,
   auto-maintain "at most one default recipe per item."
3. Restaurant costing hygiene (approve-gated recompute, dirty-data exclusion,
   per-item/per-location override) layered on top of the calendar-versioned
   core.
4. Consumption strategy owned by the recipe (assemble ingredients vs. deplete
   the finished product, per unit) — this removes both the
   theoretical-vs-actual reporting gap and the "stock moves only at
   consolidation" disconnect.

### Anti-patterns (do not copy)
- Mutating submitted-document cost fields in place — keep cost history as
  its own append-only series.
- Deactivation-by-rename conventions — model a real status field.
- Incomplete deletion propagation across a replicated hierarchy — deletion
  events must be complete for any incremental sync to be trustworthy.

---

## 5. Domain 4 — Purchasing & Accounts Payable

### Document chain
A full four-link chain — purchase order, goods receipt, invoice, payment —
with line-level cross-references throughout is the strongest model. A
two-link chain (order, then an invoice that folds receiving in) is workable
for the common restaurant case but loses the ability to represent "received
now, billed later." A single-document model — one artifact *is* both the
receipt and the invoice, with no payment concept at all — is the leanest but
forces receipt and invoice to always arrive together, and provides no
integration surface for the payment side of the process at all.

### The received-not-billed gap
An intermediate liability account — credited at receipt, debited at
invoicing — is the only mechanism that formally guarantees stock and
payables never diverge when receiving and billing happen at different times.
Systems that assume receipt and invoice always arrive together lack this
concept entirely, which is fine for the common case but a real gap for the
exceptions.

### Backdating
The strongest model executes automatic downstream recompute: a backdated
receipt or invoice with stock impact triggers a repost of future ledger
entries and GL, additional-cost vouchers repost backdated receipts with full
future recompute, and future-dated postings are rejected outright. A weaker
but honest alternative is a hard gate instead of a recompute: a closed period
simply refuses entry, and within an open period, cost effects of a backdated
entry are not automatically propagated (manual refresh-and-reapprove only).
The weakest option exposes only a binary "can backdate" permission plus
document reimport, with no visible recompute mechanism at all.

### Payment & vendor discipline
The richest operational layer includes: a mass payment run with automatic
credit-note and early-payment-discount application; a documented,
deterministic tiered auto-link between purchase order and invoice (by total
within a tolerance band, by date within a tolerance window); vendor-specific
price records with acceptable-variance thresholds and on-invoice
highlighting; two distinct, named responses to a receiving discrepancy
(short-pay vs. credit-expected); and a vendor hold mechanism (blocking all
transactions, invoices only, or payments only) plus a vendor-scorecard gate
on future orders.

### Void vs cancel
Creating a mirror "voiding" transaction that unapplies linked payments
**without mutating the original record** is a stronger integrity pattern
than flipping a status flag on the document itself, even when the flag-flip
approach does generate reversing GL entries.

### Decision for a new RMS
1. Four-link skeleton with a restaurant-friendly default merge: keep the
   intermediate liability account for the receive-now/bill-later case, but
   default the entry form to creating both receipt and invoice in one step,
   since that is the common restaurant case, while preserving the ability to
   split them.
2. Backdating: full repost mechanics plus a closed-period gate as the
   two-layer defense. A permission-only gate with no recompute is not
   adopted.
3. Void-as-mirror-transaction instead of status mutation, plus a ban on
   deleting counterparties with existing transactions (deactivate or merge
   only).
4. Full AP automation: mass payment-run auto-application, contract-price
   variance highlighting, short-pay/credit-expected as typed exception
   responses, an explicit "documents to process" queue as a first-class
   entity.
5. Machine-readable procurement intake modeled on a full EDI cycle (create →
   register → send → acknowledge → response → invoice, dated price lists,
   incremental revision-based export) — but with contract flaws fixed:
   business errors must be real error responses, never a success status
   wrapping a validation-failure payload; date formats must be consistent
   between import and export.
6. Configurable valuation with a deliberate, well-scoped choice point on
   returns (purchase price vs. current average cost) — a rare case of a
   genuinely well-designed single-field decision worth copying as-is.

### Anti-patterns (do not copy)
- Periodic cost recompute only at approve as the *only* mechanism — the
  documented "missed invoice inflates next period's COGS" failure mode.
- No error surface distinguishable from success (a success status code
  wrapping an embedded validation failure).

---

## 6. Domain 5 — Sales & POS

### The check-to-ledger pipeline
The strongest transactional frame: a check is a full document with no GL/
stock effect on its own; a consolidation step merges checks by shift × till
× customer × accounting-dimension into a single financial document, which is
where GL/stock effects actually land. One real defect to avoid: if the
consolidated document's posting date is taken from the *last* merged check
rather than the closing time, intra-day cost changes between checks all end
up valued at one moment, and inter-check sequencing (e.g. FIFO) is lost
across the merge. A pure read-only-import model (checks arrive already
finalized from the POS, no consolidation stage at all) is simpler but loses
this granularity entirely. A model where no check/order object is exposed at
all through the back-office API (order creation happens entirely on a
separate front-of-house system) means sales are only visible after the fact
as aggregated report rows.

### Stock depletion timing
Depleting stock at consolidated-document submission (or, in an alternate
per-check mode, at each check individually) is workable. Theoretical
depletion via recipe mapping, with actual stock movement only through
separate inventory counts, is weaker. Depleting stock at the moment of sale
by the recipe's own consumption strategy (see Domain 3) is the strongest and
most auditable option.

### Reservation
Explicitly reserving stock for submitted-but-not-yet-consolidated checks
(on-hand minus quantity on unconsolidated checks) prevents overselling
between the check and the consolidation step — worth adopting even though
not every reference system documents/exposes an equivalent.

### Shift open/close and acceptance
The strongest opening/closing invariants: one open shift per till and per
cashier, closing forced to the current moment (no backdated close). A
separate acceptance step, distinct from open/close, that lets the back office
review and override individual postings line by line with typed error codes
and automatic surplus/shortage posting to dedicated accounts, is the
strongest mechanism for getting cash discipline into the ledger. One flaw to
avoid explicitly: a read endpoint that creates the underlying document as a
side effect if it doesn't already exist — reads must never mutate state.

### Payment mapping to GL
Mapping each payment/tender type to a fixed default account per company is
the simplest model. A richer, declarative mapping — where a tender's payment
*group* (not just its raw type) carries the GL semantics: gift card sales
post to a liability, comps post to contra-revenue or expense, voids post no
journal entry at all, house-account tenders post to unbilled — is a stronger
decoupling than a single account-per-mode and should be preferred.

### Returns & discrepancies
Tight return rules are worth enforcing structurally: negative quantities,
payments capped at or below zero, any serial/tracked unit on a return must
trace to the original check, total refunded amount bounded by the original
total, and a return must consolidate together with or after its original
sale. Cash-drawer discrepancy should never be a purely informational field —
it must produce a real ledger entry, ideally via a required
shortage/surplus account per location that participates in the shift's
journal entry as a mandatory component, not a soft warning. Posting
differences to dedicated accounts with counterparties at acceptance time,
restricting which account types are eligible destinations (and explicitly
forbidding inventory-type accounts as a destination), is the strongest
pattern observed.

### Decision for a new RMS
1. Two-phase check→consolidation, but with cost fixed at the check, not the
   consolidation. Keep the light-check/heavy-post shape and a full
   cancellation cascade, but snapshot valuation (weighted-average or FIFO) at
   the moment of the check, and let consolidation aggregate already-valued
   facts rather than deferring valuation to consolidation time.
2. A shift-acceptance document (line-by-line override, typed error codes,
   automatic shortage/surplus posting) as the mechanism that gets cash
   discrepancy all the way into the GL. Implement the read endpoint without
   any side effect.
3. Payment-group semantics as the tender-to-GL mapping layer — richer than a
   fixed account-per-payment-mode — with an explicit balancing-adjustment
   line for unmapped-tender discrepancies, so reconciliation is never
   blocked.
4. A sales-export contract that is declarative and incremental (revision- or
   cursor-based), with a mandatory date filter — but excluding
   deleted/voided orders **by default**, never left to the client to filter.
5. Shift-opening/closing invariants: one open shift per till and per
   cashier, no backdated close, blocked sale entry against a stale-dated
   open shift.

### Anti-patterns (do not copy)
- Side-effecting read requests (a document-fetch endpoint that creates the
  document if missing).
- Cash-drawer variance as a purely informational field with no ledger
  effect.
- Deferring cost valuation to consolidation time, losing inter-check
  sequencing.

---

## 7. Domain 6 — Shift & Cash Discipline

### Unit of discipline
A document pair (opening entry + closing entry) at shift × till × cashier
granularity is the strongest transactional unit. A separate, coarser
day×location aggregate (a daily sales summary) is useful as a *reporting*
view, but treating it as a second document at a different granularity than
the shift itself creates a structural mismatch that has to be papered over
with balancing-adjustment entries — better to keep one granularity
(shift×till) as the document and treat "business day at a location" as a
derived view, never a separate document.

### Pay-in/pay-out and cash movements
Explicit pay-in/pay-out operations within a shift, plus a distinct
end-of-shift cash pickup/drop operation, are necessary primitives — some
reference systems lack any observed pay-in/pay-out capability at all, which
is a real gap worth avoiding.

### Bank reconciliation
A full cash-to-bank reconciliation chain — deposit slip, routed through an
intermediate "undeposited funds" account, automatic bank-activity matching,
and a formal reconciliation step — is the single largest capability gap
across the reference systems; only one of the three documents anything like
it in full. This is mandatory for any RMS handling physical cash, not
optional polish.

### Backdating within the shift domain
Forbidding backdated shift closes outright (posting time always forced to
the current moment) is simple but brittle against real-world late data. A
managed, explicit multi-step reversal procedure for unwinding an already-
reconciled day (reopen the period, remove from reconciliation, unmatch,
unapprove the deposit, unapprove the summary, then reprocess) is an honest
acknowledgment that point-of-sale data arrives late and sometimes at night,
and is worth adopting over a hard, unconditional forbid.

### Decision for a new RMS
1. A line-by-line shift-acceptance document as the mechanism that gets cash
   discrepancy into the GL (see Domain 5).
2. A transactional opening/closing pair with strict invariants: one open
   shift per till and per cashier, blocked cancellation while dependents
   exist, background consolidation with queued/failed/retry states.
3. A full cash-to-bank reconciliation chain (deposit → undeposited funds →
   auto-match → reconciliation with a managed unwind procedure) — this is
   the domain's single biggest gap elsewhere and is mandatory for any RMS
   handling physical cash.
4. Dual-date posting (D7) combined with three-tier, location-configurable
   variance thresholds: the first makes backdating observable without
   forbidding it; the second turns over/short handling from a bare field
   into a governed policy.

### Anti-patterns (do not copy)
- Two different granularities for the same operational day (a till-level
  document and a separate day-level document) — pick one document
  granularity and make the other a view.
- Side-effecting reads (see Domain 5).

---

## 8. Domain 7 — Workforce & Payroll

*(This domain and the two that follow were mapped at entity level only, not
carried to the same decision-with-full-rationale depth as the six domains
above — treat the findings below as strong, well-evidenced leads for a
dedicated design pass, not as finished decisions.)*

### Punch → timecard pipeline
The strongest pipeline executes fully in code: immutable check-in/check-out
logs, deterministic auto-timecard rebuild from those logs with configurable
absent/half-day/present thresholds and grace periods, and an explicit
"absent — missing check-ins" status rather than a silent gap. A documented
alternative risk to avoid: timecards with missing punches or overlapping
shifts silently excluded from payroll export when unapproved — they simply
don't appear, with no error raised anywhere.

### Rate & schedule model
Separating pay rate from job title — the title is a plain reference, the
money lives in a versioned rate assignment tied to an effective date — is the
cleanest model. A denormalized alternative — a "job" record that is
effectively position × location, duplicated per location, carrying rate, pay
type, GL account, and tip designation together — is a direct and reasonable
consequence of "the point of sale is the source of truth per location," but
it forces a merge step and risks overwriting manual edits when reconciling
against POS-sourced employee data. Compressing pay rate, salary, *and*
schedule-routing logic all into the job-title/role record is the tightest
but riskiest model — changing how someone is paid should never require
changing their job title; route schedule type via the *assignment*, not the
role.

### Payroll calculation
Executing salary-component formulas in a sandboxed evaluator gives a
jurisdiction-neutral core capable of arbitrary accrual/deduction logic
without code changes. A payroll run sourced only from *approved* attendance
data, with approval as a genuine point of no return (real bank transactions
follow), is the right discipline — corrections after that point should be
forward-looking adjustment entries only, never a rewrite of the original run.

### Tips
A rule-based tip-distribution model — contributor/receiver rules, a
pending-then-approved state machine, a hard invariant that distributions sum
to exactly the pool and "tips owed" never goes negative, and a gate that
blocks end-of-day approval while distributions are still pending — is a
necessary primitive with no good default; it has to be designed rather than
assumed away.

### Decision for a new RMS
1. The full punch→auto-timecard pipeline: immutable logs, deterministic
   rebuild, explicit-absence-over-silent-gap.
2. Sandboxed formula-based salary components as the jurisdiction-neutral
   core, with a regional compliance package layered per jurisdiction.
3. A weekly overtime-recompute rule: overtime is a function of the whole
   work-week, so any single-day timecard edit must recompute the full week
   and drop its approval status.
4. Schedule-routing (hourly vs. salaried vs. per-session) as a property of
   the *assignment*, not the job title/role.
5. Interval-versioned pay rate (a start/end date on each rate record, a new
   rate closing the previous one).
6. Approval-as-point-of-no-return, combined with deterministic
   cancel/rebuild *before* that point.
7. A rule-based tip-distribution model as described above.

### Anti-patterns (do not copy)
- Unstable object identifiers and full-replacement update semantics for
  employee/shift records.
- Location-duplicated employee records requiring manual merge, with risk of
  silently overwriting manual edits.
- Silent exclusion of malformed timecards from payroll export.

---

## 9. Domain 8 — Pricing, Discounts & Loyalty

*(Entity-level only. The central finding: no reference system offers a
complete, adoptable discount *engine*; this domain requires genuine new
design work.)*

### What "a discount" is
A discount can be modeled as a rule that reduces price/amount before posting
(computed at transaction time), or as a tender type on the check that simply
maps to a GL treatment after the fact (contra-revenue or expense) without
computing anything itself, or as a finished, already-applied fact visible
only in reporting with the actual application mechanics living entirely
outside the accounting/back-office system. The three roles are not mutually
exclusive — a real system needs a computing engine *and* a GL-mapping layer,
and possibly a reporting-only view for aggregated facts from an external POS.

### Pricing
Document-based pricing is the standout pattern: price changes are their own
ordered documents with effective dates and an explicit lifecycle
(new → processed → deleted), no direct "set price" mutation exists anywhere,
and a processed price change for a past date should be immutable; the only
permitted edit is closing it with a *future* end date. This means price
history on any past date is exactly reconstructible. Built-in time-of-day
pricing (a weekly schedule of windows) and guest-class-differentiated
pricing are both genuinely useful restaurant-specific mechanisms worth
adopting outright.

### Loyalty
A real points ledger — each earn and redeem is its own entry, first-expiring-
first-redeemed ordering, and each redemption linked back to the specific
earning it draws down — gives the strongest auditability. One real defect to
avoid: a return that **deletes and recreates** the ledger entry rather than
posting a reversing entry is a mutation of history that undermines the
ledger's own integrity guarantee — always reverse, never delete-and-recreate.

### Gift cards
The correct accounting treatment is a **liability**: a sale credits an
unredeemed-gift-card liability account, and redemption debits the same
account. Modeling a gift card as a pure discount coupon with no liability
accounting at all is a genuine accounting-correctness gap, not a
simplification.

### Decision for a new RMS
1. Document-based pricing as the canonical price model: ordered price
   changes, no direct mutation, no retroactive edits, built-in time-of-day
   schedules and guest-class categories.
2. A real loyalty ledger, with the return-handling defect fixed: a return
   posts a **reversing** entry, never a delete-and-recreate.
3. The liability model for gift cards as the only correct treatment.
4. A payment-group-style GL-semantics layer for any tender, paired with a
   validation gate at *configuration* time (reject an unmapped tender type
   at setup).
5. Selectively borrow rule-engine primitives (thresholds, priority
   ordering, coupon-code rules) — no reference system offers a complete,
   adoptable engine.

### Anti-patterns (do not copy)
- Delete-and-recreate as a "correction" mechanism for a ledger entity.
- Cartesian materialization of pricing rules from a promotional-scheme
  generator.
- Treating a gift card as a discount coupon instead of a liability.

---

## 10. Domain 9 — Organizational Structure & Multi-Unit

### The unit itself
A near-empty "branch" tag with a single name field is not fit for purpose for
a restaurant group. A real operating-unit entity — mandatory time zone,
address, legal-entity binding, location type, default GL accounts, and a
binding to whichever POS/front-of-house system feeds it — is the right
shape. An explicit deployment topology (centralized vs. replicated vs.
standalone per-location servers) reflects a real requirement worth taking
seriously if offline operation matters.

### Legal entity & ownership changes
Accounting anchors (currency, valuation method) must be hard-locked against
mutation once transactions exist against them. A change of ownership should
create **new** legal-entity and location records rather than repurposing the
old ones — tax reporting is tied to the legal entity's tax identifier, and
reusing the record would blend two owners' reporting.

### Cost centers
A genuine, separate cost-center dimension — its own tree per company,
percentage-based allocation summing to exactly 100%, and a hard rule that
any new allocation's effective date must be *later* than the last posted
entry against the center it reallocates from — prevents retroactive
redistribution of already-closed P&L. Collapsing cost center into location
loses the kitchen/bar/delivery split that real restaurant reporting needs.

### Franchise / brand layer
A franchise model — brand-managed records centrally distributed to
franchisee instances as a *partial*-record distribution (only specific
fields), automatic propagation, "least restrictive wins" on conflict — is
worth designing as an explicit extension boundary if franchise growth is a
real scenario; treat it as documented intent to design toward, not a proven
mechanism.

### Decision for a new RMS
1. A real location entity: mandatory time zone, start-of-business-day,
   legal-entity binding, default accounts; "a transaction's location
   automatically determines its legal entity for GL consolidation" is
   required.
2. Accounting-anchor protection: hard blocks on mutating currency/valuation
   method with existing transactions; no retroactive cost-center
   reallocation before the last posted entry.
3. Ownership-change-as-new-record, enforced in code.
4. A separate cost-center dimension, not collapsed into location.
5. A replication topology as an opt-in module, not baseline.
6. A franchise layer designed as a field-level distribution boundary, scoped
   as a future extension point, not core.

### Gaps not closed by any reference system
Restaurant-tuned multi-currency multi-unit consolidation; franchise +
multi-currency combined; period closing at unit/legal-entity level,
replication mechanics, and an outward-facing org-structure API never
observed together in one system.

---

## 11. Domain 10 — Forecasting & Analytics

*(Entity-level only.)*

### The core asymmetry
The strongest *executable* skeleton (forecast document → production plan →
auto-reorder with real validation) tends to pair with the weakest actual
forecasting algorithm; the richest *documented* forecasting model is not
necessarily backed by verifiable code; a strong general-purpose analytics
builder with no forecasting surface is a different asset entirely.

### Draft/publish semantics
Separating a working draft from a **published** version that downstream
consumers actually see — with accuracy measured against the published
version — is the single most important structural idea in this domain.
Guard against: recalculation silently wiping manual corrections, and
republish overwriting previous published values with no version history.

### Data hygiene
Manage the historical base as an explicit, inspectable, editable set of
facts — capped date ranges, flagged outlier dates, automatic exclusion of
zero-revenue days — never a black box.

### Decision for a new RMS
1. Draft/publish separation with an immutable history of what was published,
   by whom, on what data slice, when.
2. A managed projection-date base with outlier/zero-day exclusion; any
   recalculation requires an explicit confirm-and-diff step, never silent
   discard of manual work.
3. Configuration-over-code for exceptions: named rules with priority,
   closed-business-days overriding everything.
4. A real executable demand chain (forecast → production plan → reorder)
   under a real forecasting model.
5. A self-describing analytics contract: field metadata advertising
   grouping/aggregation/filtering support, validated server-side.

### Anti-patterns (do not copy)
- A forecast document disconnected from any real algorithm.
- Destructive recompute that silently discards manual work.
- Exporting analytics without respecting the recipient's own access scope.

---

## 12. Domain 11 — Integration, Permissions & Audit

### Ledger write protection
The strongest pattern: the ledger has no write/create permission for any
role through the ordinary interface (write access is code-only), individual
entries can never be cancelled directly (only reversible via a new entry),
and bulk deletion is funneled through a single, narrowly-scoped,
self-auditing path where the deletion record *is itself* the audit log.

### Backdating permission model
Multiple independent layers, each with its own role-based exception: frozen
date, frozen-account flag, period-level gate with an exempted role, hard
year-end wall, per-item/per-location backdating role on the stock side. The
platform rule: **the top administrative role must not be automatically
exempt** from freeze checks — exceptions are granted to a named role or
nobody, never implicitly to superuser status. Rejections должны быть
диагностируемыми (why, not just no).

### Permission model shape
A permission is an atom, a role is a set of permissions, dependencies are
explicit, and a delegated credential (API token, impersonating admin) can
never carry more authority than its grantor — uniformly across every
interface. Avoid a permission surface scattered across many independent
settings fields.

### Audit trail
Revision-/cursor-incremental event export, plus a channel for external
integrations to write typed events into the same journal — with an honest
non-guarantee (events may reappear across revisions; clients deduplicate by
stable event id). A UI-only audit log is not a substitute for a
machine-readable contract.

### Decision for a new RMS
1. Ledger write-protection at the code layer; all bulk deletion through one
   self-auditing document type.
2. "The administrative role is not exempt" as a hard platform rule for every
   period/freeze gate.
3. An additive permission model: atomic permissions, explicit dependencies,
   delegated-credential ceiling.
4. A revision-incremental audit export contract with client-side
   deduplication by stable event identifier.
5. A three-way error taxonomy for access denial (missing license/module,
   quota exhausted, missing permission).
6. A dedicated entity-deletion feed as a mandatory part of any incremental
   sync contract.

### Anti-patterns (do not copy)
- A permission surface scattered across dozens of independent settings
  fields.
- A licensing/session model leaking into API contract shape.
- Roles that silently fall behind new features because only a top-tier
  "full access" role auto-receives new permissions.

---

## 13. Cross-Domain Summary

- **GL & Accounting** — executable append-only mechanics, dual-date field,
  hybrid closing model.
- **Stock & Costing** — perpetual ledger with formal repost, server-computed
  counts with dry-run, costing hygiene.
- **Nomenclature & Recipes** — calendar-versioned recipes, executable
  lifecycle locks, recipe-owned consumption strategy.
- **Purchasing & AP** — four-link document chain, intermediate liability
  account, repost-based backdating, AP automation.
- **Sales & POS** — two-phase check-to-consolidation with cost fixed at the
  check, shift-acceptance document, payment-group GL semantics.
- **Shift & Cash** — bank reconciliation + transactional open/close
  invariants + line-level acceptance (synthesis, no single reference).
- **Workforce & Payroll** — punch-to-timecard pipeline, sandboxed formula
  pay components, weekly overtime recompute.
- **Pricing, Discounts, Loyalty** — document-based pricing, liability gift
  cards; discount engine designed from scratch.
- **Org & Multi-Unit** — real location entity, hard-locked accounting
  anchors, separate cost-center dimension.
- **Forecasting & Analytics** — draft/publish with immutable history over an
  executable demand chain.
- **Integration, Permissions, Audit** — code-level ledger protection,
  additive permissions, revision-incremental audit/deletion feed.

---

## 14. Overcomplications — Patterns to Avoid by Default

1. **Fully open-ended accounting dimensions** — fix the dimension set (unit,
   cost center, small enumerated list).
2. **A general percentage-allocation-template engine** — model the
   allocations actually needed.
3. **Two coexisting API protocol generations by domain** — pick one style.
4. **Reversal-of-reversal chains / mass-repost without idempotency** — an
   explicit idempotency contract from day one.
5. **Arbitrary-depth org tree + full replication topology, speculative** —
   shallow hierarchy + cost centers until offline autonomy is a named
   requirement.
6. **Uniform document-lifecycle ceremony on every entity** — GL/AP/stock
   earn it; reference data does not.
7. **Reversible-close-without-a-trace as the only closing mechanism** — pair
   the toggle with a snapshot (D8).
8. **Unrestricted eval-style discount rule engine** — borrow
   threshold/priority primitives selectively.
9. **Licensing/session enforcement leaking into API shape** — keep it out of
   the contract.

---

## 15. Uncovered Areas — Explicit Gaps, Not Assumptions

- Workforce/payroll, forecasting, loyalty/discount engine: entity-level
  only; each needs a dedicated design pass.
- **Multi-currency**: assume single currency per company until deliberately
  scoped; mark amount columns so a later addition doesn't force a rewrite.
- **Multi-tenant isolation / row-level access**: deployment-architecture
  question, not domain-model.
- **Open-ended posting dimensions**: deliberately not adopted by default.
- **External-API ceilings** of any specific system this is layered on:
  treat as system boundaries, not gaps to design around.
- **Six refuted assumptions** (turn each into explicit test cases):
  1. Period closing does not automatically block every backdated entry —
     the closing mechanism is exempt from its own check; cancellation
     revalidates at the current date.
  2. A "read-only" ledger may have narrow write paths (shift-line payment
     adjustments) — audit every read-only claim.
  3. A "read-only" costing engine may have narrow write paths (return
     valuation choice).
  4. Consolidation is not automatically sequence-neutral (FIFO ordering can
     be lost) — verify explicitly.
  5. A documented "soft" variance threshold can be a mandatory posting
     component in practice — read the posting mechanics.
  6. GL treatment of a transaction type can be per-deployment configuration,
     not a fixed system property.

---

## 16. Implementation Plan — Ordered Design Review Sequence

1. Fix the posting-dimension set (unit + cost center, or extensible) before
   any GL work.
2. Nail down the closing model (D8): snapshot document + per-module gate;
   define variance/statistical account behavior at close.
3. Dedicated domain passes for workforce/payroll, forecasting,
   pricing/loyalty at full rigor.
4. Scope multi-currency now or explicitly defer with a ledger marker.
5. Define the repost/recompute idempotency contract before building any
   "recalculate from date X" mechanism.
6. Decide how far shift-acceptance goes: blocking vs. flagging
   discrepancies, line-level override authority.
7. Decide the recipe-costing recompute trigger: continuous vs.
   scheduled/manual.
8. Write down no-retroactive-price-edit as an enforced invariant.
9. Re-check any target external API's ceiling against the promised feature
   list.
10. Turn the six refuted assumptions into explicit test cases per domain.
11. Design the franchise extension boundary as field-level distribution from
    the start.
12. Decide the audit/integration contract shape early: revision-incremental
    export, three-way access-denial taxonomy, entity-deletion feed.
