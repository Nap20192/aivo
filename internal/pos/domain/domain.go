// Package domain holds the core types for the POS context: cash shifts
// and per-table tickets. Service requests belong to the menu context; POS
// only reads them through its bridge port.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Shift is one cash shift at a restaurant. Closing posts
// Expected/Declared/Variance immutably — a closed Shift is never updated
// again.
type Shift struct {
	ID                uuid.UUID
	RestaurantID      uuid.UUID
	OpenedBy          uuid.UUID
	OpenedAt          time.Time
	OpeningFloatCents int
	ClosedAt          *time.Time
	DeclaredCents     *int
	ExpectedCents     *int
	VarianceCents     *int
}

// Open reports whether the shift is still open.
func (s Shift) Open() bool { return s.ClosedAt == nil }

var (
	ErrShiftClosed    = errors.New("shift already closed")
	ErrNegativeAmount = errors.New("amount must be >= 0")
)

// Close posts the closing figures: expected cash = opening float + cash
// sales during the shift (salesCents, computed by the caller from the
// shift's ticket lines), variance = declared - expected. Returns
// ErrShiftClosed if already closed.
func (s *Shift) Close(declaredCents, salesCents int, at time.Time) error {
	if !s.Open() {
		return ErrShiftClosed
	}
	if declaredCents < 0 || salesCents < 0 {
		return ErrNegativeAmount
	}
	expected := s.OpeningFloatCents + salesCents
	variance := declaredCents - expected
	s.ClosedAt = &at
	s.DeclaredCents = &declaredCents
	s.ExpectedCents = &expected
	s.VarianceCents = &variance
	return nil
}

// Ticket lifecycle states.
const (
	TicketOpen   = "open"
	TicketClosed = "closed"
)

// Ticket is the running order for one table during a shift. At most one
// open Ticket per table at a time.
type Ticket struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	ShiftID      uuid.UUID
	TableID      uuid.UUID
	Status       string
	Lines        []TicketLine
	CreatedAt    time.Time
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
	ID             uuid.UUID
	TicketID       uuid.UUID
	MenuItemID     uuid.UUID
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
