package domain

import (
	"errors"
	"testing"
	"time"

	"aivo/internal/sharedkernel"
)

func id() sharedkernel.ID { return sharedkernel.NewID() }

func TestBankRound(t *testing.T) {
	cases := []struct{ num, den, want int64 }{
		{5, 2, 2},                   // 2.5 → 2 (even)
		{7, 2, 4},                   // 3.5 → 4 (even)
		{1, 2, 0},                   // 0.5 → 0 (even)
		{3, 2, 2},                   // 1.5 → 2 (even)
		{5, 3, 2},                   // 1.66 → 2
		{-5, 2, -2},                 // -2.5 → -2
		{70000 * 1000, 10000000, 7}, // avg example
	}
	for _, c := range cases {
		if got := bankRound(c.num, c.den); got != c.want {
			t.Errorf("bankRound(%d,%d)=%d, want %d", c.num, c.den, got, c.want)
		}
	}
}

func TestToBaseMilli(t *testing.T) {
	// 2 kg into a g-based product = 2000 g = 2,000,000 milli.
	if got, err := ToBaseMilli(2, UnitKg, UnitG); err != nil || got != 2_000_000 {
		t.Errorf("2kg→g = %d,%v want 2000000", got, err)
	}
	// 200 g into a g product = 200,000 milli.
	if got, _ := ToBaseMilli(200, UnitG, UnitG); got != 200_000 {
		t.Errorf("200g→g = %d want 200000", got)
	}
	// 1.5 l into ml = 1500 ml = 1,500,000 milli.
	if got, _ := ToBaseMilli(1.5, UnitL, UnitMl); got != 1_500_000 {
		t.Errorf("1.5l→ml = %d want 1500000", got)
	}
	// kg for a pcs product → incompatible.
	if _, err := ToBaseMilli(1, UnitKg, UnitPcs); !errors.Is(err, ErrUnitIncompatible) {
		t.Errorf("kg for pcs: got %v want ErrUnitIncompatible", err)
	}
	if _, err := ToBaseMilli(1, "oz", UnitG); !errors.Is(err, ErrInvalidUnit) {
		t.Errorf("unknown unit: got %v", err)
	}
}

func TestNewProductValidation(t *testing.T) {
	mid := id()
	if _, err := NewProduct(id(), id(), "SKU", "Flour", TypeGoods, UnitKg, nil, nil); !errors.Is(err, ErrNonBaseStockUnit) {
		t.Errorf("kg stock unit: got %v want ErrNonBaseStockUnit", err)
	}
	if _, err := NewProduct(id(), id(), "SKU", "X", TypeGoods, UnitG, &mid, nil); !errors.Is(err, ErrMenuItemOnNonDish) {
		t.Errorf("menu item on goods: got %v", err)
	}
	if _, err := NewProduct(id(), id(), "SKU", "Borscht", TypeDish, UnitPcs, &mid, nil); err != nil {
		t.Errorf("valid dish: %v", err)
	}
}

// D2: weighted-average moving cost through receipts, an issue, a shortage,
// and an exact reversal by original cost.
func TestMovingAverage(t *testing.T) {
	oh := OnHand{}
	// receipt 5000 base units @ 30000c.
	oh.ApplyMove(5_000_000, 30000)
	if oh.AvgCentsPerBase() != 6 {
		t.Errorf("avg after r1 = %d, want 6", oh.AvgCentsPerBase())
	}
	// receipt 5000 @ 40000 → avg 7.
	oh.ApplyMove(5_000_000, 40000)
	if oh.QtyMilli != 10_000_000 || oh.ValueCents != 70000 || oh.AvgCentsPerBase() != 7 {
		t.Fatalf("after r2: qty=%d value=%d avg=%d, want 10000000/70000/7", oh.QtyMilli, oh.ValueCents, oh.AvgCentsPerBase())
	}
	// issue 5000 units at avg 7 → cost 35000, not estimated.
	cost, est := oh.CostOfMilli(5_000_000)
	if cost != 35000 || est {
		t.Errorf("issue cost = %d est=%v, want 35000/false", cost, est)
	}
	oh.ApplyMove(-5_000_000, -cost)
	if oh.QtyMilli != 5_000_000 || oh.ValueCents != 35000 {
		t.Errorf("after issue: qty=%d value=%d", oh.QtyMilli, oh.ValueCents)
	}
	// shortage: issue 10000 units, only 5000 on hand → estimated, qty goes negative.
	cost, est = oh.CostOfMilli(10_000_000)
	if !est {
		t.Error("over-issue should be estimated")
	}
	oh.ApplyMove(-10_000_000, -cost)
	if oh.QtyMilli != -5_000_000 {
		t.Errorf("after shortage qty = %d, want -5000000", oh.QtyMilli)
	}
}

// Reversal uses the ORIGINAL move's cost, not the current average.
func TestReversalUsesOriginalCost(t *testing.T) {
	oh := OnHand{}
	oh.ApplyMove(5_000_000, 30000) // r1: original cost 30000
	oh.ApplyMove(5_000_000, 40000) // r2 → avg 35000/5000units... value 70000
	// reverse r1 by its original (qty, cost), not current avg.
	oh.ApplyMove(-5_000_000, -30000)
	if oh.QtyMilli != 5_000_000 || oh.ValueCents != 40000 {
		t.Errorf("after reversing r1: qty=%d value=%d, want 5000000/40000", oh.QtyMilli, oh.ValueCents)
	}
}

// Fold invariant: on-hand qty/value equals the sum of applied deltas.
func TestOnHandFoldInvariant(t *testing.T) {
	deltas := []struct{ q, c int64 }{{5_000_000, 30000}, {3_000_000, 24000}, {-2_000_000, -13000}}
	oh := OnHand{}
	var sq, sc int64
	for _, d := range deltas {
		oh.ApplyMove(d.q, d.c)
		sq += d.q
		sc += d.c
	}
	if oh.QtyMilli != sq || oh.ValueCents != sc {
		t.Errorf("fold: on_hand (%d/%d) != Σ (%d/%d)", oh.QtyMilli, oh.ValueCents, sq, sc)
	}
}

// D5: interval resolution across two versions.
func TestTechCardActiveOn(t *testing.T) {
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jan5 := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	jan10 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	v1 := TechCard{ValidFrom: jan1, ValidTo: &jan5}
	v2 := TechCard{ValidFrom: jan5, ValidTo: nil}

	if !v1.ActiveOn(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Error("v1 should be active on Jan 3")
	}
	if v1.ActiveOn(jan5) {
		t.Error("v1 must NOT be active on its valid_to (Jan 5)")
	}
	if !v2.ActiveOn(jan5) || !v2.ActiveOn(jan10) {
		t.Error("v2 (open) should be active on Jan 5 and Jan 10")
	}
	if v2.ActiveOn(time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)) {
		t.Error("v2 must not be active before its valid_from")
	}
}

func TestRecipeCycleDetection(t *testing.T) {
	a, b, c := id(), id(), id()
	// A→B→A is a cycle from A.
	cyc := map[sharedkernel.ID][]sharedkernel.ID{a: {b}, b: {a}}
	if !ReachesSelf(a, cyc) {
		t.Error("A→B→A should reach self")
	}
	// A→B→C is acyclic.
	acyc := map[sharedkernel.ID][]sharedkernel.ID{a: {b}, b: {c}}
	if ReachesSelf(a, acyc) {
		t.Error("A→B→C should not reach self")
	}
}

func TestValidateLines(t *testing.T) {
	a := id()
	if err := ValidateLines(nil); !errors.Is(err, ErrEmptyRecipe) {
		t.Errorf("empty: %v", err)
	}
	dup := []TechCardLine{{IngredientProductID: a, Qty: 1}, {IngredientProductID: a, Qty: 2}}
	if err := ValidateLines(dup); !errors.Is(err, ErrDuplicateIngredient) {
		t.Errorf("dup: %v", err)
	}
	bad := []TechCardLine{{IngredientProductID: a, Qty: 0}}
	if err := ValidateLines(bad); !errors.Is(err, ErrInvalidQty) {
		t.Errorf("zero qty: %v", err)
	}
}

func TestRecipeCost(t *testing.T) {
	flour, water := id(), id()
	// 200 g flour @ 6c/base + 100 g water @ 0.
	lines := []TechCardLine{
		{IngredientProductID: flour, Qty: 200_000},
		{IngredientProductID: water, Qty: 100_000},
	}
	cost := RecipeCost(lines, map[sharedkernel.ID]int64{flour: 6, water: 0})
	if cost != 1200 {
		t.Errorf("recipe cost = %d, want 1200", cost)
	}
	// prepared unit cost from a 1000 g yield costing 3000c = 3 c/base.
	if got := UnitCostFromRecipe(3000, 1_000_000); got != 3 {
		t.Errorf("unit cost from recipe = %d, want 3", got)
	}
}
