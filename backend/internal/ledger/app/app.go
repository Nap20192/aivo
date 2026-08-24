// Package app is the ledger context's use-case layer: chart-of-accounts
// reads, the per-restaurant GL-semantics map, manual journals, shift
// journal build/post, and reversals. Posting goes through the domain
// gate; a posted document is corrected only by a reversal (append-only).
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	ledger "aivo/internal/domain/ledger"
	"aivo/internal/ledger/ports"
	"aivo/internal/sharedkernel"

	"uuid"
)

// ErrInvalid marks caller-fixable input (422). ErrUnknownPurpose /
// ErrAccountNotPostable are specific 422 causes mapped by the HTTP layer.
var (
	ErrInvalid            = errors.New("ledger: invalid input")
	ErrUnknownPurpose     = errors.New("ledger: unknown purpose")
	ErrAccountNotPostable = errors.New("ledger: account is not postable")
)

// Purposes is the fixed set of GL-semantics mapping keys (contract §2).
// tender:<group> maps a payment group to its account.
var Purposes = []string{
	"sales_revenue", "cash_drawer", "cash_over_short", "cash_movement", "rounding_unassigned",
	"tender:cash", "tender:card", "tender:gift_card", "tender:comp", "tender:house_account",
	// Inventory / COGS (increment-2, §9).
	"inventory", "accounts_payable", "received_not_billed", "cogs", "inventory_shrinkage", "inventory_surplus",
}

func knownPurpose(p string) bool {
	for _, k := range Purposes {
		if k == p {
			return true
		}
	}
	return false
}

// CostCenterMain is the seeded default cost center code.
const CostCenterMain = "main"

type App struct {
	store ports.Store
	newID func() sharedkernel.ID
	now   func() time.Time
}

func New(store ports.Store) *App {
	return &App{store: store, newID: sharedkernel.NewID, now: func() time.Time { return time.Now().UTC() }}
}

// periodOpen is the D8 close-gate stub: always open in increment-1. The
// extension point is fixed; the snapshot document lands later.
func (a *App) periodOpen(_ uuid.UUID, _ time.Time) bool { return true }

// Accounts returns the restaurant's chart of accounts.
func (a *App) Accounts(ctx context.Context, restaurantID uuid.UUID) ([]ledger.Account, error) {
	return a.store.Accounts(ctx, restaurantID)
}

// AccountMapGet returns the purpose→account config.
func (a *App) AccountMapGet(ctx context.Context, restaurantID uuid.UUID) ([]ports.AccountMapEntry, error) {
	return a.store.AccountMap(ctx, restaurantID)
}

// AccountForPurpose resolves a mapping (used by the pos ledger bridge).
func (a *App) AccountForPurpose(ctx context.Context, restaurantID uuid.UUID, purpose string) (uuid.UUID, error) {
	return a.store.AccountForPurpose(ctx, restaurantID, purpose)
}

// MapEntry is one PUT account-map row.
type MapEntry struct {
	Purpose   string
	AccountID uuid.UUID
}

// AccountMapPut replaces the given mappings (upsert). Rejects unknown
// purposes and non-postable/foreign accounts (§15.6 config write path).
func (a *App) AccountMapPut(ctx context.Context, restaurantID uuid.UUID, entries []MapEntry) ([]ports.AccountMapEntry, error) {
	for _, e := range entries {
		if !knownPurpose(e.Purpose) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownPurpose, e.Purpose)
		}
		acc, err := a.store.AccountByID(ctx, restaurantID, e.AccountID)
		if err != nil {
			return nil, err
		}
		if !acc.Postable {
			return nil, fmt.Errorf("%w: %s", ErrAccountNotPostable, acc.Code)
		}
	}
	err := a.store.InTx(ctx, func(st ports.Store) error {
		for _, e := range entries {
			if err := st.PutAccountMap(ctx, restaurantID, e.Purpose, e.AccountID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a.store.AccountMap(ctx, restaurantID)
}

// GetJournals lists posted documents (list view), optional filters.
func (a *App) GetJournals(ctx context.Context, restaurantID uuid.UUID, from string, account *uuid.UUID, source string) ([]ports.JournalSummary, error) {
	if from == "" {
		return nil, fmt.Errorf("%w: from is required", ErrInvalid)
	}
	return a.store.PostedJournals(ctx, restaurantID, from, account, source)
}

// GetJournal returns one document with its lines.
func (a *App) GetJournal(ctx context.Context, restaurantID, docID uuid.UUID) (ledger.JournalDocument, error) {
	return a.store.DocumentByID(ctx, restaurantID, docID)
}

// LiveDocumentForShift returns the shift's single non-cancelled journal
// (draft before acceptance, posted after) — the acceptance review target.
func (a *App) LiveDocumentForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) (ledger.JournalDocument, error) {
	return a.store.LiveDocumentBySource(ctx, restaurantID, ledger.SourceShift, shiftID)
}

// --- Manual journal ----------------------------------------------------

// ManualLine is one line of a manual journal.
type ManualLine struct {
	AccountID    uuid.UUID
	Side         string
	AmountCents  int64
	CostCenterID *uuid.UUID // nil → main
	Memo         string
}

// ManualJournalInput is the request for a manual journal.
type ManualJournalInput struct {
	RestaurantID   uuid.UUID
	CreatedBy      uuid.UUID
	AccountingDate time.Time
	Memo           string
	Lines          []ManualLine
}

// ManualJournal builds a manual document. For manual journals an
// imbalance is an error (no auto-balancing) — contract §4. When post is
// true it is posted immediately.
func (a *App) ManualJournal(ctx context.Context, in ManualJournalInput, post bool) (ledger.JournalDocument, error) {
	if len(in.Lines) == 0 {
		return ledger.JournalDocument{}, fmt.Errorf("%w: at least one line", ErrInvalid)
	}
	mainCC, err := a.store.CostCenterByCode(ctx, in.RestaurantID, CostCenterMain)
	if err != nil {
		return ledger.JournalDocument{}, err
	}
	doc := ledger.NewDocument(a.newID(), in.RestaurantID, in.CreatedBy, ledger.KindManual, in.AccountingDate, a.now())
	doc.SourceKind = ledger.KindManual
	for _, l := range in.Lines {
		acc, err := a.store.AccountByID(ctx, in.RestaurantID, l.AccountID)
		if err != nil {
			return ledger.JournalDocument{}, err
		}
		if !acc.Postable {
			return ledger.JournalDocument{}, fmt.Errorf("%w: %s", ErrAccountNotPostable, acc.Code)
		}
		cc := mainCC.ID
		if l.CostCenterID != nil {
			cc = *l.CostCenterID
		}
		if err := doc.AddLine(a.newID(), l.AccountID, cc, l.Side, l.AmountCents, l.Memo); err != nil {
			return ledger.JournalDocument{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	if !doc.Balanced() {
		return ledger.JournalDocument{}, ledger.ErrUnbalanced
	}
	if post {
		if err := doc.Post(a.now(), func(d time.Time) bool { return a.periodOpen(in.RestaurantID, d) }); err != nil {
			return ledger.JournalDocument{}, err
		}
	}
	if err := a.store.InsertDocument(ctx, doc); err != nil {
		return ledger.JournalDocument{}, err
	}
	return *doc, nil
}

// --- Reversal ----------------------------------------------------------

// CancelJournal storns a posted document: a reversal is posted at the
// current date and the original is marked cancelled, in one transaction.
func (a *App) CancelJournal(ctx context.Context, restaurantID, docID uuid.UUID) (uuid.UUID, error) {
	var reversalID uuid.UUID
	err := a.store.InTx(ctx, func(st ports.Store) error {
		id, err := a.cancelOn(ctx, st, restaurantID, docID)
		reversalID = id
		return err
	})
	return reversalID, err
}

// cancelOn reverses a posted document on the given (tx-bound) store.
func (a *App) cancelOn(ctx context.Context, st ports.Store, restaurantID, docID uuid.UUID) (uuid.UUID, error) {
	orig, err := st.DocumentByID(ctx, restaurantID, docID)
	if err != nil {
		return uuid.Nil(), err
	}
	rev, err := orig.Reverse(a.newID(), a.now(), a.newID)
	if err != nil {
		return uuid.Nil(), err
	}
	// Mark the original cancelled BEFORE inserting the reversal, so the "one
	// live document per shift" partial-unique never sees two live documents
	// at once (the reversal keeps the source linkage).
	if err := st.MarkCancelled(ctx, restaurantID, docID, *orig.CancelledAt); err != nil {
		return uuid.Nil(), err
	}
	if err := st.InsertDocument(ctx, rev); err != nil {
		return uuid.Nil(), err
	}
	return rev.ID, nil
}

// CancelJournalForSource reverses the live posted journal for a source
// (e.g. an inventory document) on the caller's transaction, so the reversal
// GL and the reversal stock moves commit atomically (§6 cancel).
func (a *App) CancelJournalForSource(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID, sourceKind string, sourceID uuid.UUID) (uuid.UUID, error) {
	st := a.store.WithTx(tx)
	doc, err := st.LiveDocumentBySource(ctx, restaurantID, sourceKind, sourceID)
	if err != nil {
		return uuid.Nil(), err
	}
	return a.cancelOn(ctx, st, restaurantID, doc.ID)
}

// --- Shift acceptance journal (pos ledger bridge) ----------------------

// TenderTotal is one payment group's total for a shift.
type TenderTotal struct {
	Group       string
	AmountCents int64
}

// ShiftJournalInput is what pos hands the ledger to build the acceptance
// draft. The GL treatment (which account each tender lands on) is resolved
// here from the per-restaurant map (§15.6).
type ShiftJournalInput struct {
	RestaurantID   uuid.UUID
	ShiftID        uuid.UUID
	CreatedBy      uuid.UUID
	AccountingDate time.Time
	Tenders        []TenderTotal
	VarianceCents  int64 // declared - expected (cash drawer)
}

// BuildDraftShiftJournal creates the draft acceptance document in tx and
// returns its id. Lines: one debit per tender group on its mapped account
// (cash adjusted by the variance), a cash_over_short line when variance is
// non-zero (refuted §15.5 — variance is a posting, not a soft field), a
// sales_revenue credit for the tender total, and an auto-balance safety
// line on rounding_unassigned. Cash movements affect the displayed
// expected/variance (pos) but are not posted as separate GL lines in
// increment-1 (the cash_movement mapping is seeded for later).
func (a *App) BuildDraftShiftJournal(ctx context.Context, tx *sql.Tx, in ShiftJournalInput) (uuid.UUID, error) {
	st := a.store.WithTx(tx)
	mainCC, err := st.CostCenterByCode(ctx, in.RestaurantID, CostCenterMain)
	if err != nil {
		return uuid.Nil(), err
	}
	cc := mainCC.ID

	acct := func(purpose string) (uuid.UUID, error) {
		return st.AccountForPurpose(ctx, in.RestaurantID, purpose)
	}

	doc := ledger.NewDocument(a.newID(), in.RestaurantID, in.CreatedBy, ledger.KindShiftAcceptance, in.AccountingDate, a.now())
	doc.SourceKind = ledger.SourceShift
	sid := in.ShiftID
	doc.SourceID = &sid

	var salesTotal, cashTotal int64
	for _, t := range in.Tenders {
		if t.AmountCents == 0 || t.Group == "void" {
			continue
		}
		salesTotal += t.AmountCents
		if t.Group == "cash" {
			cashTotal += t.AmountCents
			continue // cash handled below, adjusted by variance
		}
		id, err := acct("tender:" + t.Group)
		if err != nil {
			return uuid.Nil(), err
		}
		if err := doc.AddLine(a.newID(), id, cc, ledger.SideDebit, t.AmountCents, "tender:"+t.Group); err != nil {
			return uuid.Nil(), err
		}
	}

	// Cash drawer debit = cash tendered + variance (declared vs expected).
	if cashTotal != 0 || in.VarianceCents != 0 {
		cashAcct, err := acct("tender:cash")
		if err != nil {
			return uuid.Nil(), err
		}
		if err := doc.AddSigned(a.newID(), cashAcct, cc, cashTotal+in.VarianceCents, "cash tenders (declared)", a.newID); err != nil {
			return uuid.Nil(), err
		}
	}
	// Cash over/short absorbs the variance (mandatory line when != 0).
	if in.VarianceCents != 0 {
		osAcct, err := acct("cash_over_short")
		if err != nil {
			return uuid.Nil(), err
		}
		if err := doc.AddSigned(a.newID(), osAcct, cc, -in.VarianceCents, "cash over/short", a.newID); err != nil {
			return uuid.Nil(), err
		}
	}
	// Sales revenue credit for the full tender total.
	if salesTotal > 0 {
		salesAcct, err := acct("sales_revenue")
		if err != nil {
			return uuid.Nil(), err
		}
		if err := doc.AddLine(a.newID(), salesAcct, cc, ledger.SideCredit, salesTotal, "sales"); err != nil {
			return uuid.Nil(), err
		}
	}
	// Safety net (should be a no-op with integer cents).
	if roundAcct, err := acct("rounding_unassigned"); err == nil {
		if err := doc.AutoBalance(a.newID(), roundAcct, cc); err != nil {
			return uuid.Nil(), err
		}
	} else if !errors.Is(err, ports.ErrNotFound) {
		return uuid.Nil(), err
	}

	if err := st.InsertDocument(ctx, doc); err != nil {
		return uuid.Nil(), err
	}
	return doc.ID, nil
}

// CostCenters returns the restaurant's cost centers (override dropdown).
func (a *App) CostCenters(ctx context.Context, restaurantID uuid.UUID) ([]ledger.CostCenter, error) {
	return a.store.CostCenters(ctx, restaurantID)
}

// PostShiftJournal posts a draft acceptance document in tx (called at
// Accept). It locks the document row FOR UPDATE and re-reads its state in
// the same transaction, so a concurrent override cannot rewrite the lines
// of a document that has just been posted (append-only, B1/§15.2/§15.3).
func (a *App) PostShiftJournal(ctx context.Context, tx *sql.Tx, restaurantID, docID uuid.UUID) error {
	st := a.store.WithTx(tx)
	state, err := st.LockDocumentState(ctx, restaurantID, docID)
	if err != nil {
		return err
	}
	if state != ledger.StateDraft {
		return ledger.ErrNotDraft
	}
	doc, err := st.DocumentByID(ctx, restaurantID, docID)
	if err != nil {
		return err
	}
	at := a.now()
	if err := doc.Post(at, func(d time.Time) bool { return a.periodOpen(restaurantID, d) }); err != nil {
		return err
	}
	return st.MarkPosted(ctx, restaurantID, docID, at)
}

// InventoryJournalLine is one line of an inventory GL document, addressed
// by purpose (resolved through ledger_account_map — the same §15.6 config).
type InventoryJournalLine struct {
	Purpose     string // inventory|accounts_payable|cogs|inventory_shrinkage|inventory_surplus
	Side        string // debit|credit
	AmountCents int64
	Memo        string
}

// InventoryJournalInput is the request to post an inventory GL document.
type InventoryJournalInput struct {
	RestaurantID   uuid.UUID
	CreatedBy      uuid.UUID
	SourceKind     string // inventory_receipt|inventory_writeoff|inventory_stocktake|cogs
	SourceID       uuid.UUID
	AccountingDate time.Time
	Lines          []InventoryJournalLine
}

// PostInventoryJournal builds and posts an inventory GL document on the
// caller's transaction, immediately (inventory postings are mechanical, no
// human review — §2). Lines address accounts by purpose; the document is
// auto-balanced onto rounding_unassigned and posted through the domain
// gate. Correction is only via CancelJournal (reversal); append-only holds.
func (a *App) PostInventoryJournal(ctx context.Context, tx *sql.Tx, in InventoryJournalInput) (uuid.UUID, error) {
	st := a.store.WithTx(tx)
	mainCC, err := st.CostCenterByCode(ctx, in.RestaurantID, CostCenterMain)
	if err != nil {
		return uuid.Nil(), err
	}
	cc := mainCC.ID

	doc := ledger.NewDocument(a.newID(), in.RestaurantID, in.CreatedBy, in.SourceKind, in.AccountingDate, a.now())
	doc.SourceKind = in.SourceKind
	sid := in.SourceID
	doc.SourceID = &sid

	for _, l := range in.Lines {
		if l.AmountCents == 0 {
			continue
		}
		acctID, err := st.AccountForPurpose(ctx, in.RestaurantID, l.Purpose)
		if err != nil {
			return uuid.Nil(), err
		}
		memo := l.Memo
		if memo == "" {
			memo = l.Purpose
		}
		if err := doc.AddLine(a.newID(), acctID, cc, l.Side, l.AmountCents, memo); err != nil {
			return uuid.Nil(), err
		}
	}
	if roundAcct, err := st.AccountForPurpose(ctx, in.RestaurantID, "rounding_unassigned"); err == nil {
		if err := doc.AutoBalance(a.newID(), roundAcct, cc); err != nil {
			return uuid.Nil(), err
		}
	} else if !errors.Is(err, ports.ErrNotFound) {
		return uuid.Nil(), err
	}
	at := a.now()
	if err := doc.Post(at, func(d time.Time) bool { return a.periodOpen(in.RestaurantID, d) }); err != nil {
		return uuid.Nil(), err
	}
	if err := st.InsertDocument(ctx, doc); err != nil {
		return uuid.Nil(), err
	}
	return doc.ID, nil
}

// OverrideDraftLines rewrites a draft document's lines (acceptance review
// override, §15.2). The write runs in one transaction that locks the
// document row FOR UPDATE and re-reads its state, so a concurrent accept
// cannot flip the document to posted between the check and the rewrite
// (append-only guard, B1). Rejects non-postable/foreign accounts and cost
// centers that do not belong to the restaurant.
func (a *App) OverrideDraftLines(ctx context.Context, restaurantID, docID uuid.UUID, lines []ledger.JournalLine) error {
	for _, l := range lines {
		acc, err := a.store.AccountByID(ctx, restaurantID, l.AccountID)
		if err != nil {
			return err
		}
		if !acc.Postable {
			return fmt.Errorf("%w: %s", ErrAccountNotPostable, acc.Code)
		}
	}
	if err := a.validateCostCenters(ctx, restaurantID, lines); err != nil {
		return err
	}
	return a.store.InTx(ctx, func(st ports.Store) error {
		state, err := st.LockDocumentState(ctx, restaurantID, docID)
		if err != nil {
			return err
		}
		if state != ledger.StateDraft {
			return ledger.ErrNotDraft
		}
		return st.ReplaceDraftLines(ctx, docID, lines)
	})
}

// validateCostCenters rejects a line whose cost center does not belong to
// the restaurant (an FK would otherwise surface as a 500 instead of 422).
func (a *App) validateCostCenters(ctx context.Context, restaurantID uuid.UUID, lines []ledger.JournalLine) error {
	centers, err := a.store.CostCenters(ctx, restaurantID)
	if err != nil {
		return err
	}
	own := map[uuid.UUID]bool{}
	for _, c := range centers {
		own[c.ID] = true
	}
	for _, l := range lines {
		if !own[l.CostCenterID] {
			return fmt.Errorf("%w: cost center %s does not belong to the restaurant", ErrInvalid, l.CostCenterID)
		}
	}
	return nil
}
