package domain

import (
	"errors"
	"testing"
	"time"
)

func TestShiftClose(t *testing.T) {
	s := Shift{OpeningFloatCents: 10000} // $100 float
	now := time.Now()

	// Sales $250, declared $349.50 -> expected $350, variance -$0.50.
	if err := s.Close(34950, 25000, now); err != nil {
		t.Fatalf("close: %v", err)
	}
	if *s.ExpectedCents != 35000 {
		t.Errorf("expected = %d, want 35000", *s.ExpectedCents)
	}
	if *s.VarianceCents != -50 {
		t.Errorf("variance = %d, want -50", *s.VarianceCents)
	}
	if s.Open() {
		t.Error("shift still open after close")
	}

	// Posted close is immutable.
	if err := s.Close(1, 1, now); !errors.Is(err, ErrShiftClosed) {
		t.Errorf("second close: got %v, want ErrShiftClosed", err)
	}
}

func TestShiftCloseRejectsNegative(t *testing.T) {
	s := Shift{}
	if err := s.Close(-1, 0, time.Now()); !errors.Is(err, ErrNegativeAmount) {
		t.Errorf("negative declared: got %v, want ErrNegativeAmount", err)
	}
}

func TestTicketTotal(t *testing.T) {
	tk := Ticket{Lines: []TicketLine{
		{UnitPriceCents: 4600, Qty: 1, Options: []LineOption{{PriceDeltaCents: 1200}, {PriceDeltaCents: 300}}}, // 6100
		{UnitPriceCents: 900, Qty: 2}, // 1800
	}}
	if got := tk.TotalCents(); got != 7900 {
		t.Errorf("total = %d, want 7900", got)
	}
}
