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

func TestProductType_Valid(t *testing.T) {
	cases := []struct {
		t    ProductType
		want bool
	}{
		{TypeGoods, true}, {TypeDish, true}, {TypePrepared, true}, {TypeModifier, true},
		{"", false}, {"unknown", false},
	}
	for _, c := range cases {
		if got := c.t.Valid(); got != c.want {
			t.Errorf("ProductType(%q).Valid() = %v, want %v", c.t, got, c.want)
		}
	}
}

func TestDefaultProductType(t *testing.T) {
	if got := DefaultProductType(); got != TypeGoods {
		t.Errorf("DefaultProductType() = %v, want %v", got, TypeGoods)
	}
}

func TestTechCardFormat_Valid(t *testing.T) {
	cases := []struct {
		f    TechCardFormat
		want bool
	}{
		{FormatSimple, true}, {FormatTTK, true}, {"", false}, {"unknown", false},
	}
	for _, c := range cases {
		if got := c.f.Valid(); got != c.want {
			t.Errorf("TechCardFormat(%q).Valid() = %v, want %v", c.f, got, c.want)
		}
	}
}

func TestDefaultTechCardFormat(t *testing.T) {
	if got := DefaultTechCardFormat(); got != FormatSimple {
		t.Errorf("DefaultTechCardFormat() = %v, want %v", got, FormatSimple)
	}
}

func TestConsumeStrategy_Valid(t *testing.T) {
	cases := []struct {
		c    ConsumeStrategy
		want bool
	}{
		{ConsumeAssemble, true}, {ConsumeDepleteFinished, true}, {"", false}, {"unknown", false},
	}
	for _, c := range cases {
		if got := c.c.Valid(); got != c.want {
			t.Errorf("ConsumeStrategy(%q).Valid() = %v, want %v", c.c, got, c.want)
		}
	}
}

func TestDefaultConsumeStrategy(t *testing.T) {
	if got := DefaultConsumeStrategy(); got != ConsumeAssemble {
		t.Errorf("DefaultConsumeStrategy() = %v, want %v", got, ConsumeAssemble)
	}
}

func TestCostMethod_Valid(t *testing.T) {
	cases := []struct {
		m    CostMethod
		want bool
	}{
		{CostMethodWeightedAvg, true}, {"", false}, {"unknown", false},
	}
	for _, c := range cases {
		if got := c.m.Valid(); got != c.want {
			t.Errorf("CostMethod(%q).Valid() = %v, want %v", c.m, got, c.want)
		}
	}
}

func TestDefaultCostMethod(t *testing.T) {
	if got := DefaultCostMethod(); got != CostMethodWeightedAvg {
		t.Errorf("DefaultCostMethod() = %v, want %v", got, CostMethodWeightedAvg)
	}
}

func TestValidUnit(t *testing.T) {
	for _, u := range []string{UnitG, UnitKg, UnitMl, UnitL, UnitPcs} {
		if !ValidUnit(u) {
			t.Errorf("ValidUnit(%q) = false, want true", u)
		}
	}
	if ValidUnit("oz") {
		t.Error("ValidUnit(oz) = true, want false")
	}
}

func TestToBaseMilli_InvalidStockUnit(t *testing.T) {
	if _, err := ToBaseMilli(1, UnitG, "oz"); !errors.Is(err, ErrInvalidUnit) {
		t.Errorf("bad stock unit: got %v want ErrInvalidUnit", err)
	}
}

func TestToBaseMilli_NonPositiveQty(t *testing.T) {
	if _, err := ToBaseMilli(0, UnitG, UnitG); !errors.Is(err, ErrInvalidQty) {
		t.Errorf("zero qty: got %v want ErrInvalidQty", err)
	}
	if _, err := ToBaseMilli(-1, UnitG, UnitG); !errors.Is(err, ErrInvalidQty) {
		t.Errorf("negative qty: got %v want ErrInvalidQty", err)
	}
}

func TestFromBaseMilli(t *testing.T) {
	if got := FromBaseMilli(2_500_000); got != 2500 {
		t.Errorf("FromBaseMilli(2500000) = %v, want 2500", got)
	}
}

func TestNewProductValidation_InvalidType(t *testing.T) {
	if _, err := NewProduct(id(), id(), "SKU", "X", "bogus", UnitG, nil, nil); !errors.Is(err, ErrInvalidType) {
		t.Errorf("invalid type: got %v want ErrInvalidType", err)
	}
}

func TestValidReason(t *testing.T) {
	for _, r := range []string{ReasonSpoilage, ReasonExpiry, ReasonStaffMeal, ReasonLoss, ReasonOther} {
		if !ValidReason(r) {
			t.Errorf("ValidReason(%q) = false, want true", r)
		}
	}
	if ValidReason("bogus") {
		t.Error("ValidReason(bogus) = true, want false")
	}
}

func TestGoodsReceiptTotalCents(t *testing.T) {
	r := GoodsReceipt{Lines: []GoodsReceiptLine{{LineCostCents: 100}, {LineCostCents: 250}}}
	if got := r.TotalCents(); got != 350 {
		t.Errorf("TotalCents() = %d, want 350", got)
	}
	if got := (GoodsReceipt{}).TotalCents(); got != 0 {
		t.Errorf("TotalCents() empty = %d, want 0", got)
	}
}

// CostOfMilli's shortage branch when on-hand is already negative (avail
// clamped to 0, whole deficit priced at LastAvgCents).
func TestCostOfMilli_NegativeOnHand(t *testing.T) {
	oh := OnHand{QtyMilli: -1000, LastAvgCents: 5}
	cost, estimated := oh.CostOfMilli(2000)
	if !estimated {
		t.Error("expected estimated = true")
	}
	// deficit = 2000 - 0 = 2000 milli * 5c / 1000 = 10c.
	if cost != 10 {
		t.Errorf("cost = %d, want 10", cost)
	}
}

func TestAvgCentsPerBase_NonPositiveQty(t *testing.T) {
	oh := OnHand{QtyMilli: 0, LastAvgCents: 42}
	if got := oh.AvgCentsPerBase(); got != 42 {
		t.Errorf("AvgCentsPerBase() at zero qty = %d, want LastAvgCents 42", got)
	}
	oh = OnHand{QtyMilli: -5, LastAvgCents: 9}
	if got := oh.AvgCentsPerBase(); got != 9 {
		t.Errorf("AvgCentsPerBase() at negative qty = %d, want LastAvgCents 9", got)
	}
}

func TestBankRound_ZeroDenominator(t *testing.T) {
	if got := bankRound(5, 0); got != 0 {
		t.Errorf("bankRound(5,0) = %d, want 0", got)
	}
}

func TestBankRound_NegativeDenominator(t *testing.T) {
	// Both negative: signs cancel, same as bankRound(5,2) = 2.
	if got := bankRound(-5, -2); got != 2 {
		t.Errorf("bankRound(-5,-2) = %d, want 2", got)
	}
	// Positive numerator, negative denominator: negative result.
	if got := bankRound(5, -2); got != -2 {
		t.Errorf("bankRound(5,-2) = %d, want -2", got)
	}
}

func TestTechCardLine_NetQty(t *testing.T) {
	if got := (TechCardLine{Qty: 1000, YieldPermille: 900}).NetQty(); got != 900 {
		t.Errorf("NetQty() = %d, want 900", got)
	}
	// YieldPermille <= 0 defaults to 100% (no loss).
	if got := (TechCardLine{Qty: 1000, YieldPermille: 0}).NetQty(); got != 1000 {
		t.Errorf("NetQty() default yield = %d, want 1000", got)
	}
}

func TestValidateLines_YieldOutOfRange(t *testing.T) {
	a := id()
	bad := []TechCardLine{{IngredientProductID: a, Qty: 1, YieldPermille: 1001}}
	if err := ValidateLines(bad); !errors.Is(err, ErrInvalidYieldPct) {
		t.Errorf("yield too high: %v", err)
	}
	negative := []TechCardLine{{IngredientProductID: a, Qty: 1, YieldPermille: -1}}
	if err := ValidateLines(negative); !errors.Is(err, ErrInvalidYieldPct) {
		t.Errorf("negative yield: %v", err)
	}
}

func TestValidateLines_Valid(t *testing.T) {
	a, b := id(), id()
	lines := []TechCardLine{
		{IngredientProductID: a, Qty: 1, YieldPermille: 500},
		{IngredientProductID: b, Qty: 2, YieldPermille: YieldPermilleDefault},
	}
	if err := ValidateLines(lines); err != nil {
		t.Errorf("ValidateLines() = %v, want nil", err)
	}
}

func TestUnitCostFromRecipe_ZeroYield(t *testing.T) {
	if got := UnitCostFromRecipe(3000, 0); got != 0 {
		t.Errorf("UnitCostFromRecipe with zero yield = %d, want 0", got)
	}
	if got := UnitCostFromRecipe(3000, -1); got != 0 {
		t.Errorf("UnitCostFromRecipe with negative yield = %d, want 0", got)
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
