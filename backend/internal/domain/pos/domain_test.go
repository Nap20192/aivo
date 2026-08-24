package domain

import (
	"errors"
	"testing"
	"time"

	"aivo/internal/sharedkernel"
)

func TestShiftClose(t *testing.T) {
	s := Shift{OpeningFloatCents: 10000} // $100 float
	now := time.Now()

	// Cash tenders $250, pay-in $20, pay-out $5, drop $10 →
	// expected = 10000 + 25000 + 2000 − 500 − 1000 = 35500.
	// Declared $354.50 → variance −$0.50.
	if err := s.Close(35450, 25000, 2000, 500, 1000, now); err != nil {
		t.Fatalf("close: %v", err)
	}
	if *s.ExpectedCents != 35500 {
		t.Errorf("expected = %d, want 35500", *s.ExpectedCents)
	}
	if *s.VarianceCents != -50 {
		t.Errorf("variance = %d, want -50", *s.VarianceCents)
	}
	if s.Open() || s.State() != ShiftClosed {
		t.Errorf("state = %s, want closed", s.State())
	}

	// Posted close is immutable.
	if err := s.Close(1, 1, 0, 0, 0, now); !errors.Is(err, ErrShiftClosed) {
		t.Errorf("second close: got %v, want ErrShiftClosed", err)
	}
}

func TestShiftCloseCardNotInDrawer(t *testing.T) {
	// Card tenders never reach the drawer: expected = float only.
	s := Shift{OpeningFloatCents: 5000}
	if err := s.Close(5000, 0, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if *s.ExpectedCents != 5000 || *s.VarianceCents != 0 {
		t.Errorf("expected=%d variance=%d, want 5000/0", *s.ExpectedCents, *s.VarianceCents)
	}
}

func TestShiftCloseRejectsNegative(t *testing.T) {
	s := Shift{}
	if err := s.Close(-1, 0, 0, 0, 0, time.Now()); !errors.Is(err, ErrNegativeAmount) {
		t.Errorf("negative declared: got %v, want ErrNegativeAmount", err)
	}
}

func TestShiftAccept(t *testing.T) {
	s := Shift{OpeningFloatCents: 0}
	by, doc := newID(), newID()
	// Cannot accept an open shift.
	if err := s.Accept(time.Now(), by, doc); !errors.Is(err, ErrShiftNotClosed) {
		t.Errorf("accept open: got %v, want ErrShiftNotClosed", err)
	}
	if err := s.Close(0, 0, 0, 0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Accept(time.Now(), by, doc); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if s.State() != ShiftAccepted || *s.JournalDocumentID != doc {
		t.Errorf("state=%s doc=%v", s.State(), s.JournalDocumentID)
	}
	// Idempotency guard: no second accept.
	if err := s.Accept(time.Now(), by, doc); !errors.Is(err, ErrAlreadyAccepted) {
		t.Errorf("second accept: got %v, want ErrAlreadyAccepted", err)
	}
}

func TestTicketClose(t *testing.T) {
	mk := func() Ticket {
		return Ticket{Status: TicketOpen, Lines: []TicketLine{{UnitPriceCents: 1000, Qty: 2}}} // total 2000
	}
	// Exact tender total closes.
	tk := mk()
	if err := tk.Close([]Tender{{PaymentGroup: PaymentGroupCash, AmountCents: 2000}}, time.Now()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if tk.Status != TicketClosed || tk.ClosedAt == nil {
		t.Error("not closed")
	}
	// Mismatch rejected.
	tk = mk()
	if err := tk.Close([]Tender{{PaymentGroup: PaymentGroupCash, AmountCents: 1500}}, time.Now()); !errors.Is(err, ErrTendersMismatch) {
		t.Errorf("mismatch: got %v, want ErrTendersMismatch", err)
	}
	// void closes without matching payment.
	tk = mk()
	if err := tk.Close([]Tender{{PaymentGroup: PaymentGroupVoid, AmountCents: 0}}, time.Now()); err != nil {
		t.Errorf("void close: %v", err)
	}
	// Second close rejected.
	if err := tk.Close(nil, time.Now()); !errors.Is(err, ErrTicketClosed) {
		t.Errorf("second close: got %v, want ErrTicketClosed", err)
	}
}

func newID() sharedkernel.ID { return sharedkernel.NewID() }

func TestTicketTotal(t *testing.T) {
	tk := Ticket{Lines: []TicketLine{
		{UnitPriceCents: 4600, Qty: 1, Options: []LineOption{{PriceDeltaCents: 1200}, {PriceDeltaCents: 300}}}, // 6100
		{UnitPriceCents: 900, Qty: 2}, // 1800
	}}
	if got := tk.TotalCents(); got != 7900 {
		t.Errorf("total = %d, want 7900", got)
	}
}
