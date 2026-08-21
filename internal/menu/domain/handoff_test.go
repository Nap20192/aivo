package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNewHandoffCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := NewHandoffCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 {
			t.Fatalf("len(%q) = %d, want 6", code, len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune(HandoffCodeAlphabet, r) {
				t.Fatalf("code %q contains %q outside the alphabet", code, r)
			}
		}
		// The ambiguous glyphs must never appear.
		if strings.ContainsAny(code, "0O1I") {
			t.Fatalf("code %q contains an ambiguous glyph", code)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Errorf("only %d unique codes in 200 draws — RNG suspicious", len(seen))
	}
}

func TestHandoffActive(t *testing.T) {
	now := time.Now()
	h := Handoff{ExpiresAt: now.Add(15 * time.Minute)}
	if !h.Active(now) {
		t.Error("fresh handoff should be active")
	}
	if h.Active(now.Add(16 * time.Minute)) {
		t.Error("expired handoff should be inactive")
	}
	used := now
	h.UsedAt = &used
	if h.Active(now) {
		t.Error("used handoff should be inactive (single-use)")
	}
}

func TestHandoffTotal(t *testing.T) {
	h := Handoff{Lines: []HandoffLine{
		{UnitPriceCents: 4600, Qty: 1, Options: []OrderLineOption{{PriceDeltaCents: 1200}}}, // 5800
		{UnitPriceCents: 900, Qty: 2}, // 1800
	}}
	if got := h.TotalCents(); got != 7600 {
		t.Errorf("total = %d, want 7600", got)
	}
}
