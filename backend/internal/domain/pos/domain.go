// Package domain holds the core types for the POS context: cash shifts
// and per-table tickets. Service requests belong to the menu context; POS
// only reads them through its bridge port.
package domain

import (
	"errors"
	"time"

	"aivo/internal/sharedkernel"
)

// Shift lifecycle states (display cache; source of truth is the closed_at
// / accepted_at columns): open → closed → accepted (D6).
const (
	ShiftOpen     = "open"
	ShiftClosed   = "closed"
	ShiftAccepted = "accepted"
)

// Shift is one cash shift at a restaurant. Closing posts
// Expected/Declared/Variance immutably; acceptance (back office) posts the
// GL journal — a closed/accepted Shift is never re-closed.
type Shift struct {
	ID                sharedkernel.ID
	RestaurantID      sharedkernel.ID
	OpenedBy          sharedkernel.ID
	Cashier           string // display name, denormalized at open time
	OpenedAt          time.Time
	OpeningFloatCents int
	ClosedAt          *time.Time
	DeclaredCents     *int
	ExpectedCents     *int
	VarianceCents     *int
	AcceptedAt        *time.Time
	AcceptedBy        *sharedkernel.ID
	JournalDocumentID *sharedkernel.ID
}

// Open reports whether the shift is still open.
func (s Shift) Open() bool { return s.ClosedAt == nil }

// Accepted reports whether the shift's journal has been posted.
func (s Shift) Accepted() bool { return s.AcceptedAt != nil }

// State returns the display state (open|closed|accepted).
func (s Shift) State() string {
	switch {
	case s.Accepted():
		return ShiftAccepted
	case !s.Open():
		return ShiftClosed
	default:
		return ShiftOpen
	}
}

var (
	ErrShiftClosed     = errors.New("shift already closed")
	ErrShiftNotClosed  = errors.New("shift is not closed")
	ErrAlreadyAccepted = errors.New("shift already accepted")
	ErrNegativeAmount  = errors.New("amount must be >= 0")
	ErrTicketClosed    = errors.New("ticket already closed")
	ErrTendersMismatch = errors.New("tender total does not equal ticket total")
)

// Close posts the closing figures. Expected cash in the drawer =
// opening float + cash tenders taken during the shift + pay-ins − pay-outs
// − drops (card/other tenders never hit the drawer). Variance =
// declared − expected. The caller aggregates the cash figures from the
// shift's tenders and cash operations. Returns ErrShiftClosed if already
// closed.
func (s *Shift) Close(declaredCents, cashTendersCents, payInCents, payOutCents, dropCents int, at time.Time) error {
	if !s.Open() {
		return ErrShiftClosed
	}
	if declaredCents < 0 || cashTendersCents < 0 || payInCents < 0 || payOutCents < 0 || dropCents < 0 {
		return ErrNegativeAmount
	}
	expected := s.OpeningFloatCents + cashTendersCents + payInCents - payOutCents - dropCents
	variance := declaredCents - expected
	s.ClosedAt = &at
	s.DeclaredCents = &declaredCents
	s.ExpectedCents = &expected
	s.VarianceCents = &variance
	return nil
}

// Accept marks a closed shift accepted and records the posted journal.
// Guards: must be closed and not already accepted.
func (s *Shift) Accept(at time.Time, by, journalDocumentID sharedkernel.ID) error {
	if s.Open() {
		return ErrShiftNotClosed
	}
	if s.Accepted() {
		return ErrAlreadyAccepted
	}
	s.AcceptedAt = &at
	s.AcceptedBy = &by
	s.JournalDocumentID = &journalDocumentID
	return nil
}

// Ticket lifecycle states.
const (
	TicketOpen   = "open"
	TicketClosed = "closed"
)

// Payment groups drive GL semantics (contract §2/§3). void closes a
// ticket without payment (comped/cancelled) and creates no GL posting.
const (
	PaymentGroupCash         = "cash"
	PaymentGroupCard         = "card"
	PaymentGroupGiftCard     = "gift_card"
	PaymentGroupComp         = "comp"
	PaymentGroupVoid         = "void"
	PaymentGroupHouseAccount = "house_account"
)

// Cash operation kinds (in-shift movements, reference §7).
const (
	CashPayIn  = "pay_in"
	CashPayOut = "pay_out"
	CashDrop   = "drop"
)

// PaymentMethod is a configurable tender type; payment_group drives GL
// semantics. Seed: cash, card.
type PaymentMethod struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Code         string
	Name         string
	PaymentGroup string
	Active       bool
}

// Tender is one payment recorded against a ticket at close.
type Tender struct {
	MethodID     sharedkernel.ID
	PaymentGroup string
	AmountCents  int
	TipCents     int
}

// CashOperation is one in-shift cash movement.
type CashOperation struct {
	ID           sharedkernel.ID
	ShiftID      sharedkernel.ID
	RestaurantID sharedkernel.ID
	Kind         string
	AmountCents  int
	Reason       string
	RecordedBy   sharedkernel.ID
	RecordedAt   time.Time
}

// Ticket is the running order for one table during a shift. At most one
// open Ticket per table at a time.
type Ticket struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	ShiftID      sharedkernel.ID
	TableID      sharedkernel.ID
	CustomerID   *sharedkernel.ID // linked when a customer's handoff was accepted
	Status       string
	Note         string // diner note from a cart handoff, "" otherwise
	Lines        []TicketLine
	CreatedAt    time.Time
	ClosedAt     *time.Time
}

// Close validates tenders against the ticket total and closes it. Σ
// tender amounts must equal the ticket total, unless every tender is in
// the void group (a comped/cancelled ticket closes without payment). An
// empty tender list is allowed only for a zero-total ticket. Returns
// ErrTicketClosed if not open, ErrTendersMismatch on a wrong total.
func (t *Ticket) Close(tenders []Tender, at time.Time) error {
	if t.Status != TicketOpen {
		return ErrTicketClosed
	}
	sum := 0
	allVoid := len(tenders) > 0
	for _, td := range tenders {
		if td.AmountCents < 0 || td.TipCents < 0 {
			return ErrNegativeAmount
		}
		sum += td.AmountCents
		if td.PaymentGroup != PaymentGroupVoid {
			allVoid = false
		}
	}
	if !allVoid && sum != t.TotalCents() {
		return ErrTendersMismatch
	}
	t.Status = TicketClosed
	t.ClosedAt = &at
	return nil
}

// TotalCents is the sum of all line totals.
func (t Ticket) TotalCents() int {
	total := 0
	for _, l := range t.Lines {
		total += l.TotalCents()
	}
	return total
}

// LineOption is a snapshot of a chosen option at add time.
type LineOption struct {
	Label           string `json:"label"`
	PriceDeltaCents int    `json:"price_delta_cents"`
}

// TicketLine snapshots a menu item (name, unit price, options) at the
// moment a waiter adds it — later menu edits never alter it. FiredAt is
// set when the line is sent to the kitchen.
type TicketLine struct {
	ID             sharedkernel.ID
	TicketID       sharedkernel.ID
	MenuItemID     sharedkernel.ID
	Name           string
	UnitPriceCents int
	Qty            int
	Options        []LineOption
	FiredAt        *time.Time
	CreatedAt      time.Time
}

// TotalCents is unit price plus option deltas, times qty.
func (l TicketLine) TotalCents() int {
	unit := l.UnitPriceCents
	for _, o := range l.Options {
		unit += o.PriceDeltaCents
	}
	return unit * l.Qty
}
