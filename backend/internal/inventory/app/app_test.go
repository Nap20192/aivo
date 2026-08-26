package app

import (
	"errors"
	"testing"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/sharedkernel"
)

func TestParseDate(t *testing.T) {
	got, err := ParseDate("2026-01-15")
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseDate() = %v, want %v", got, want)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	if _, err := ParseDate("not-a-date"); !errors.Is(err, ErrInvalid) {
		t.Errorf("ParseDate(bad) error = %v, want ErrInvalid", err)
	}
	if _, err := ParseDate(""); !errors.Is(err, ErrInvalid) {
		t.Errorf("ParseDate(empty) error = %v, want ErrInvalid", err)
	}
}

func TestPostGuard(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{inv.DocPosted, ErrAlreadyPosted},
		{inv.DocCancelled, ErrAlreadyCancelled},
		{inv.DocDraft, nil},
		{"bogus", ErrNotDraft},
	}
	for _, c := range cases {
		if got := postGuard(c.status); !errors.Is(got, c.want) && got != c.want {
			t.Errorf("postGuard(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestCancelGuard(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{inv.DocPosted, nil},
		{inv.DocCancelled, ErrAlreadyCancelled},
		{inv.DocDraft, ErrNotPosted},
	}
	for _, c := range cases {
		if got := cancelGuard(c.status); !errors.Is(got, c.want) && got != c.want {
			t.Errorf("cancelGuard(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestToBaseAllowZero(t *testing.T) {
	got, err := toBaseAllowZero(0, inv.UnitG, inv.UnitG)
	if err != nil || got != 0 {
		t.Errorf("toBaseAllowZero(0) = %d, %v, want 0, nil", got, err)
	}
	if _, err := toBaseAllowZero(0, "bogus", inv.UnitG); !errors.Is(err, inv.ErrInvalidUnit) {
		t.Errorf("toBaseAllowZero(0, bad unit) = %v, want ErrInvalidUnit", err)
	}
	got, err = toBaseAllowZero(2, inv.UnitKg, inv.UnitG)
	if err != nil || got != 2_000_000 {
		t.Errorf("toBaseAllowZero(2kg) = %d, %v, want 2000000, nil", got, err)
	}
}

func TestVarianceCost(t *testing.T) {
	oh := inv.OnHand{QtyMilli: 1000, ValueCents: 500, LastAvgCents: 500}
	if got := varianceCost(oh, 0); got != 0 {
		t.Errorf("varianceCost(0) = %d, want 0", got)
	}
	if got := varianceCost(oh, 1000); got <= 0 {
		t.Errorf("varianceCost(surplus) = %d, want > 0", got)
	}
	if got := varianceCost(oh, -1000); got >= 0 {
		t.Errorf("varianceCost(shortage) = %d, want < 0", got)
	}
}

func TestRoundToEven(t *testing.T) {
	if got := roundToEven(2.5); got != 2 {
		t.Errorf("roundToEven(2.5) = %d, want 2", got)
	}
	if got := roundToEven(3.5); got != 4 {
		t.Errorf("roundToEven(3.5) = %d, want 4", got)
	}
}

func TestDeriveEventID_Deterministic(t *testing.T) {
	a, b := sharedkernel.NewID(), sharedkernel.NewID()
	if deriveEventID(a, b) != deriveEventID(a, b) {
		t.Error("deriveEventID not deterministic for the same inputs")
	}
	if deriveEventID(a, b) == deriveEventID(b, a) {
		t.Error("deriveEventID should be order-sensitive")
	}
}
