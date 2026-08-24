package domain

import (
	"errors"
	"testing"
	"time"

	"aivo/internal/sharedkernel"
)

func newID() sharedkernel.ID { return sharedkernel.NewID() }

// buildBalanced makes a draft with debit 15000 = credit 15000.
func buildBalanced(t *testing.T) *JournalDocument {
	t.Helper()
	d := NewDocument(newID(), newID(), newID(), KindShiftAcceptance, time.Now(), time.Now())
	cc := newID()
	if err := d.AddLine(newID(), newID(), cc, SideDebit, 10000, "cash"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddLine(newID(), newID(), cc, SideDebit, 5000, "card"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddLine(newID(), newID(), cc, SideCredit, 15000, "sales"); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAddLineRejectsBadLines(t *testing.T) {
	d := NewDocument(newID(), newID(), newID(), KindManual, time.Now(), time.Now())
	cc := newID()
	if err := d.AddLine(newID(), newID(), cc, "sideways", 100, ""); !errors.Is(err, ErrInvalidSide) {
		t.Errorf("bad side: got %v, want ErrInvalidSide", err)
	}
	if err := d.AddLine(newID(), newID(), cc, SideDebit, 0, ""); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("zero amount: got %v, want ErrInvalidAmount", err)
	}
	if err := d.AddLine(newID(), newID(), cc, SideDebit, -5, ""); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("negative amount: got %v, want ErrInvalidAmount", err)
	}
}

func TestBalanceAndPost(t *testing.T) {
	d := buildBalanced(t)
	if !d.Balanced() {
		t.Fatal("want balanced")
	}
	if err := d.Post(time.Now(), nil); err != nil {
		t.Fatalf("post: %v", err)
	}
	if d.State != StatePosted || d.PostedAt == nil {
		t.Fatalf("state=%s postedAt=%v", d.State, d.PostedAt)
	}
	// Posted is immutable: no more lines, no second post.
	if err := d.AddLine(newID(), newID(), newID(), SideDebit, 1, ""); !errors.Is(err, ErrNotDraft) {
		t.Errorf("add after post: got %v, want ErrNotDraft", err)
	}
	if err := d.Post(time.Now(), nil); !errors.Is(err, ErrNotDraft) {
		t.Errorf("second post: got %v, want ErrNotDraft", err)
	}
}

func TestPostRejectsUnbalanced(t *testing.T) {
	d := NewDocument(newID(), newID(), newID(), KindManual, time.Now(), time.Now())
	_ = d.AddLine(newID(), newID(), newID(), SideDebit, 100, "")
	if err := d.Post(time.Now(), nil); !errors.Is(err, ErrUnbalanced) {
		t.Errorf("unbalanced post: got %v, want ErrUnbalanced", err)
	}
}

func TestPostRespectsPeriodGate(t *testing.T) {
	d := buildBalanced(t)
	if err := d.Post(time.Now(), func(time.Time) bool { return false }); !errors.Is(err, ErrPeriodClosed) {
		t.Errorf("closed period: got %v, want ErrPeriodClosed", err)
	}
}

func TestAutoBalance(t *testing.T) {
	d := NewDocument(newID(), newID(), newID(), KindShiftAcceptance, time.Now(), time.Now())
	cc, unassigned := newID(), newID()
	_ = d.AddLine(newID(), newID(), cc, SideDebit, 9800, "cash")
	_ = d.AddLine(newID(), newID(), cc, SideCredit, 10000, "sales")
	if err := d.AutoBalance(newID(), unassigned, cc); err != nil {
		t.Fatal(err)
	}
	if !d.Balanced() {
		t.Fatal("auto-balance left it unbalanced")
	}
	last := d.Lines[len(d.Lines)-1]
	if last.AccountID != unassigned || last.Side != SideDebit || last.AmountCents != 200 {
		t.Errorf("auto line = %+v, want debit 200 on unassigned", last)
	}
	// Already balanced → no line added.
	before := len(d.Lines)
	_ = d.AutoBalance(newID(), unassigned, cc)
	if len(d.Lines) != before {
		t.Error("auto-balance added a line to an already-balanced doc")
	}
}

func TestReverseMirrorsAndCancels(t *testing.T) {
	d := buildBalanced(t)
	past := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	d.AccountingDate = past
	if err := d.Post(time.Now(), nil); err != nil {
		t.Fatal(err)
	}

	today := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	rev, err := d.Reverse(newID(), today, newID)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	// §15.1: reversal revalidated at current date, original cancelled.
	if !rev.AccountingDate.Equal(today) {
		t.Errorf("reversal accounting_date = %v, want today %v", rev.AccountingDate, today)
	}
	if d.State != StateCancelled || d.CancelledAt == nil {
		t.Errorf("original state=%s cancelledAt=%v, want cancelled", d.State, d.CancelledAt)
	}
	if rev.Kind != KindReversal || rev.ReversalOf == nil || *rev.ReversalOf != d.ID {
		t.Errorf("reversal linkage wrong: kind=%s reversalOf=%v", rev.Kind, rev.ReversalOf)
	}
	if !rev.Balanced() || rev.State != StatePosted {
		t.Errorf("reversal not posted/balanced: state=%s", rev.State)
	}
	// Mirrored: original debit 15000 total ↔ reversal credit 15000 total.
	od, oc := d.Balance()
	rd, rc := rev.Balance()
	if od != rc || oc != rd {
		t.Errorf("not mirrored: orig(%d/%d) rev(%d/%d)", od, oc, rd, rc)
	}

	// Double cancel → ErrAlreadyCancelled.
	if _, err := d.Reverse(newID(), today, newID); !errors.Is(err, ErrAlreadyCancelled) {
		t.Errorf("double reverse: got %v, want ErrAlreadyCancelled", err)
	}
}

func TestReverseRejectsDraft(t *testing.T) {
	d := buildBalanced(t)
	if _, err := d.Reverse(newID(), time.Now(), newID); !errors.Is(err, ErrNotPosted) {
		t.Errorf("reverse draft: got %v, want ErrNotPosted", err)
	}
}
