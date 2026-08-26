// Package domain holds the core types for the ledger (GL) context: the
// chart of accounts and append-only journal documents. A JournalDocument
// is the aggregate root — its lifecycle (draft → posted → cancelled) is
// the posting gate (D4); a posted document is immutable and can only be
// corrected by a reversal (D1). Every document carries two dates (D7).
// Money is integer cents, single currency (company base) — multicurrency
// deferred (reference §16.4).
package domain

import (
	"errors"
	"time"

	"aivo/internal/sharedkernel"
)

// AccountType is a chart-of-accounts entry's type.
type AccountType string

// Account types.
const (
	TypeAsset       AccountType = "asset"
	TypeLiability   AccountType = "liability"
	TypeRevenue     AccountType = "revenue"
	TypeExpense     AccountType = "expense"
	TypeEquity      AccountType = "equity"
	TypeStatistical AccountType = "statistical"
)

// Default is the seed account type.
func (AccountType) Default() AccountType { return TypeAsset }

// Valid reports whether t is one of the six known account types.
func (t AccountType) Valid() bool {
	switch t {
	case TypeAsset, TypeLiability, TypeRevenue, TypeExpense, TypeEquity, TypeStatistical:
		return true
	}
	return false
}

// Side is a journal line's (or account's normal) side.
type Side string

// Normal sides.
const (
	SideDebit  Side = "debit"
	SideCredit Side = "credit"
)

// Default is the zero-value side.
func (Side) Default() Side { return SideDebit }

// Valid reports whether s is debit or credit.
func (s Side) Valid() bool { return s == SideDebit || s == SideCredit }

// DocumentKind is a journal document's own kind. Inventory-originated
// documents (posted via PostInventoryJournal) pass their source kind
// (e.g. "cogs") straight through this field too — a pre-existing quirk
// this refactor preserves, not a closed set in practice.
type DocumentKind string

// Document kinds.
const (
	KindShiftAcceptance DocumentKind = "shift_acceptance"
	KindManual          DocumentKind = "manual"
	KindReversal        DocumentKind = "reversal"
)

// Default is the manual-journal kind.
func (DocumentKind) Default() DocumentKind { return KindManual }

// Valid reports whether k is one of the three named document kinds
// (inventory source kinds passed through this field are not covered).
func (k DocumentKind) Valid() bool {
	return k == KindShiftAcceptance || k == KindManual || k == KindReversal
}

// DocumentState is a journal document's lifecycle state.
type DocumentState string

// Document states.
const (
	StateDraft     DocumentState = "draft"
	StatePosted    DocumentState = "posted"
	StateCancelled DocumentState = "cancelled"
)

// Default is the state of a freshly created document.
func (DocumentState) Default() DocumentState { return StateDraft }

// Valid reports whether s is one of the three known document states.
func (s DocumentState) Valid() bool {
	return s == StateDraft || s == StatePosted || s == StateCancelled
}

// SourceShift marks a document produced by shift acceptance. SourceKind
// stays a plain string (not a typed enum, see the JournalDocument field
// doc): inventory contributes further values (SourceReceipt, SourceCOGS,
// ...) it owns itself, so ledger cannot close the set without coupling
// to another context's constants (D6).
const SourceShift = "shift"

var (
	ErrNotDraft         = errors.New("ledger: document is not draft")
	ErrNotPosted        = errors.New("ledger: document is not posted")
	ErrAlreadyCancelled = errors.New("ledger: document already cancelled")
	ErrUnbalanced       = errors.New("ledger: debits do not equal credits")
	ErrInvalidSide      = errors.New("ledger: line side must be debit or credit")
	ErrInvalidAmount    = errors.New("ledger: line amount must be > 0")
	ErrPeriodClosed     = errors.New("ledger: accounting period is closed")
)

// Account is a chart-of-accounts entry (a light aggregate / reference).
// Code is unique within a restaurant; only a postable leaf account
// accepts lines. Type and NormalSide freeze after the first posting to
// the account (enforced in the app/store, not the UI).
type Account struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Code         string
	Name         string
	Type         AccountType
	NormalSide   Side
	Postable     bool
	CreatedAt    time.Time
}

// CostCenter is a flat per-restaurant dimension (seed: one "main"). No
// tree, no allocation engine — shallow until a named requirement.
type CostCenter struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Code         string
	Name         string
}

// JournalLine is one side of a document. Strictly one-sided: exactly a
// debit or a credit, amount > 0. Append-only once its document is posted.
type JournalLine struct {
	ID           sharedkernel.ID
	DocumentID   sharedkernel.ID
	AccountID    sharedkernel.ID
	Side         Side
	AmountCents  int64 // > 0, single currency (§16.4)
	CostCenterID sharedkernel.ID
	Memo         string
	Seq          int
}

// JournalDocument is the aggregate root: a balanced set of one-sided
// lines with a posting gate and two dates.
type JournalDocument struct {
	sharedkernel.AggregateRoot

	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Kind         DocumentKind
	State        DocumentState

	AccountingDate time.Time // business date of the fact (D7)
	RecordedAt     time.Time // wall-clock of the record (D7)
	PostedAt       *time.Time
	CancelledAt    *time.Time

	SourceKind string           // "shift" | "manual" | ""
	SourceID   *sharedkernel.ID // e.g. the shift id
	ReversalOf *sharedkernel.ID // set on a reversal document
	CreatedBy  sharedkernel.ID

	Lines []JournalLine
}

// NewDocument creates a draft document. accountingDate is the business
// date; recordedAt is the wall clock of the record (both required, D7).
func NewDocument(id, restaurantID, createdBy sharedkernel.ID, kind DocumentKind, accountingDate, recordedAt time.Time) *JournalDocument {
	return &JournalDocument{
		ID:             id,
		RestaurantID:   restaurantID,
		Kind:           kind,
		State:          StateDraft,
		AccountingDate: accountingDate,
		RecordedAt:     recordedAt,
		CreatedBy:      createdBy,
	}
}

// AddLine appends a one-sided line. Returns ErrInvalidSide/ErrInvalidAmount
// on a bad line. Only allowed while draft.
func (d *JournalDocument) AddLine(id, accountID, costCenterID sharedkernel.ID, side Side, amountCents int64, memo string) error {
	if d.State != StateDraft {
		return ErrNotDraft
	}
	if side != SideDebit && side != SideCredit {
		return ErrInvalidSide
	}
	if amountCents <= 0 {
		return ErrInvalidAmount
	}
	d.Lines = append(d.Lines, JournalLine{
		ID:           id,
		DocumentID:   d.ID,
		AccountID:    accountID,
		Side:         side,
		AmountCents:  amountCents,
		CostCenterID: costCenterID,
		Memo:         memo,
		Seq:          len(d.Lines) + 1,
	})
	return nil
}

// AddSigned adds a line whose side follows the sign of amount: positive →
// debit, negative → credit (the absolute value is the line amount). A
// zero amount adds nothing. Convenience for building auto-derived lines
// (variance, movements) whose direction is data-driven.
func (d *JournalDocument) AddSigned(id, accountID, costCenterID sharedkernel.ID, amount int64, memo string, newID func() sharedkernel.ID) error {
	if amount == 0 {
		return nil
	}
	side := SideDebit
	if amount < 0 {
		side, amount = SideCredit, -amount
	}
	return d.AddLine(id, accountID, costCenterID, side, amount, memo)
}

// Balance returns the debit and credit totals of the current lines.
func (d *JournalDocument) Balance() (debit, credit int64) {
	for _, l := range d.Lines {
		if l.Side == SideDebit {
			debit += l.AmountCents
		} else {
			credit += l.AmountCents
		}
	}
	return debit, credit
}

// Balanced reports whether debits equal credits.
func (d *JournalDocument) Balanced() bool {
	debit, credit := d.Balance()
	return debit == credit
}

// AutoBalance appends a single balancing line on the given account (the
// rounding/unassigned safety net, reference §2) when debits and credits
// differ. A no-op when already balanced. Draft only.
func (d *JournalDocument) AutoBalance(lineID, accountID, costCenterID sharedkernel.ID) error {
	debit, credit := d.Balance()
	diff := debit - credit // >0: need a credit; <0: need a debit
	if diff == 0 {
		return nil
	}
	side := SideCredit
	amount := diff
	if diff < 0 {
		side, amount = SideDebit, -diff
	}
	return d.AddLine(lineID, accountID, costCenterID, side, amount, "auto-balance (rounding/unassigned)")
}

// Post is the single transition into posted (D4). It requires a balanced,
// draft document and an open period. periodOpen may be nil (always open).
func (d *JournalDocument) Post(at time.Time, periodOpen func(accountingDate time.Time) bool) error {
	if d.State != StateDraft {
		return ErrNotDraft
	}
	// A zero-line document is trivially balanced (0=0) — allowed for a
	// no-activity shift acceptance. The manual-journal app path rejects
	// an empty document itself (422).
	if !d.Balanced() {
		return ErrUnbalanced
	}
	if periodOpen != nil && !periodOpen(d.AccountingDate) {
		return ErrPeriodClosed
	}
	d.State = StatePosted
	d.PostedAt = &at
	return nil
}

// Reverse produces the storno of a posted document (D1): a new reversal
// document with mirrored lines (debit↔credit), revalidated at the current
// date (accountingDate = at, refuted §15.1 — a closed period never blocks
// its own reversal), and marks the original cancelled. The original is
// never mutated beyond its state/cancelled_at bookkeeping.
func (d *JournalDocument) Reverse(newID sharedkernel.ID, at time.Time, newLineID func() sharedkernel.ID) (*JournalDocument, error) {
	switch d.State {
	case StateCancelled:
		return nil, ErrAlreadyCancelled
	case StatePosted:
	default:
		return nil, ErrNotPosted
	}

	rev := NewDocument(newID, d.RestaurantID, d.CreatedBy, KindReversal, at, at)
	rev.ReversalOf = &d.ID
	rev.SourceKind = d.SourceKind
	rev.SourceID = d.SourceID
	for _, l := range d.Lines {
		side := SideCredit
		if l.Side == SideCredit {
			side = SideDebit
		}
		if err := rev.AddLine(newLineID(), l.AccountID, l.CostCenterID, side, l.AmountCents, "reversal of "+l.Memo); err != nil {
			return nil, err
		}
	}
	// Reversal is posted immediately (a correction of a posted fact).
	if err := rev.Post(at, nil); err != nil {
		return nil, err
	}
	d.State = StateCancelled
	d.CancelledAt = &at
	return rev, nil
}
