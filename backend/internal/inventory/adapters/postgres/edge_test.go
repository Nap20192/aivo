package postgres

import (
	"context"
	"errors"
	"testing"

	inv "aivo/internal/domain/inventory"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"

	"uuid"
)

// Stocktake surplus: counted > on-hand books a surplus move (the shortage
// branch is already covered by TestStocktakeDryRunAndPost).
func TestStocktakeSurplus(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 500, inv.UnitG, 5) // 500g, avg 5

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 2), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.EnterCounts(f.ctx, f.restID, st.ID, []inventoryapp.StocktakeCountInput{{ProductID: flour.ID, QtyInput: 800, Unit: inv.UnitG}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh.QtyMilli != 800_000 {
		t.Errorf("after surplus qty=%d, want 800000", oh.QtyMilli)
	}
	assertEventExists(t, f, inventoryapp.EventStocktakePosted, st.ID)
}

// unitCostOf's recursive branch: a prepared product used as an ingredient
// of another prepared product, priced through its own tech card (not the
// cycle-rejection path — a genuine two-level recipe).
func TestRecipeCost_RecursiveThroughPreparedIngredient(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 5000, inv.UnitG, 6) // avg 6c/g

	dough := f.product(t, "DOUGH", "Dough", inv.TypePrepared, inv.UnitG, nil)
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dough.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1_000_000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 1000, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	} // dough recipe cost = 1000g * 6c = 6000c, yield 1000g (1_000_000 milli) -> 6c/g

	menuItem := uuid.New()
	pie := f.product(t, "PIE", "Pie", inv.TypeDish, inv.UnitPcs, &menuItem)
	tc, err := f.app.CreateTechCardVersion(f.ctx, f.restID, pie.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: dough.ID, QtyInput: 500, Unit: inv.UnitG}}, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	// pie recost = 500g dough @ 6c/g (recursed through dough's own card) = 3000c.
	view, err := f.app.TechCard(f.ctx, f.restID, tc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.CostCents != 3000 {
		t.Errorf("recursive recipe cost = %d, want 3000", view.CostCents)
	}
}

// ConsumeForSale skips a ticket line whose menu item isn't a tracked
// product (retail item) and one whose dish has no active tech card on the
// sale date — a sale is never blocked by missing inventory data.
func TestConsumeForSale_SkipsUntrackedAndCardless(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, &menuItem) // no tech card ever created
	untrackedMenuItem := uuid.New()                                            // no product at all

	tx, err := f.db.BeginTx(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := f.app.ConsumeForSale(f.ctx, tx, f.restID, f.userID, uuid.New(), day(2026, 1, 1), []inventoryapp.SaleLine{
		{MenuItemID: untrackedMenuItem, Qty: 1, TicketLineID: uuid.New()},
		{MenuItemID: dish.ID, Qty: 1, TicketLineID: uuid.New()}, // wrong id on purpose: not a menu item match either
		{MenuItemID: menuItem, Qty: 1, TicketLineID: uuid.New()},
		{MenuItemID: menuItem, Qty: 0, TicketLineID: uuid.New()}, // qty <= 0, also skipped
	})
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if cost != 0 {
		t.Errorf("cost = %d, want 0 (nothing consumable)", cost)
	}
}

// ConsumeForSale with a ConsumeDepleteFinished strategy depletes the
// finished product itself rather than assembling from ingredients.
func TestConsumeForSale_DepleteFinished(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	prepped := f.product(t, "PREPPED", "Prepped Soup", inv.TypeDish, inv.UnitPcs, &menuItem)
	f.postReceipt(t, day(2026, 1, 1), prepped.ID, 10, inv.UnitPcs, 100) // 10 portions on hand @ 100c

	// ConsumeDepleteFinished ignores the tech card's lines at consume time
	// (cogs.go depletes product.ID directly) — a line is still required by
	// ValidateLines, so use an unrelated ingredient rather than a
	// self-reference (which the cycle check would reject).
	garnish := f.product(t, "GARNISH", "Garnish", inv.TypeGoods, inv.UnitG, nil)
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, prepped.ID, day(2026, 1, 1), inv.ConsumeDepleteFinished, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: garnish.ID, QtyInput: 1, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}

	tx, _ := f.db.BeginTx(f.ctx, nil)
	cost, err := f.app.ConsumeForSale(f.ctx, tx, f.restID, f.userID, uuid.New(), day(2026, 1, 2),
		[]inventoryapp.SaleLine{{MenuItemID: menuItem, Qty: 2, TicketLineID: uuid.New()}})
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if cost != 200 { // 2 portions @ 100c
		t.Errorf("deplete-finished cost = %d, want 200", cost)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, prepped.ID)
	if oh.QtyMilli != 8000 { // 10 - 2 portions, in pcs milli-units
		t.Errorf("on_hand after deplete-finished = %d, want 8000", oh.QtyMilli)
	}
}

// FoodCostReport with real sold-dish data exercises the
// theoretical/actual/estimated-share computation, not just the
// no-sales-in-period branch.
type fakeSales struct {
	rows []ports.SaleQty
}

func (s fakeSales) SoldDishes(context.Context, uuid.UUID, string, string) ([]ports.SaleQty, error) {
	return s.rows, nil
}

func TestFoodCostReport_WithSales(t *testing.T) {
	f := setup(t)
	dish, flour, menuItem := f.borschtWithFlourCard(t)

	tx, _ := f.db.BeginTx(f.ctx, nil)
	if _, err := f.app.ConsumeForSale(f.ctx, tx, f.restID, f.userID, uuid.New(), day(2026, 1, 2),
		[]inventoryapp.SaleLine{{MenuItemID: menuItem, Qty: 3, TicketLineID: uuid.New()}}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	appWithSales := inventoryapp.New(f.store, fakeSales{rows: []ports.SaleQty{
		{MenuItemID: menuItem, Name: "Borscht", QtyMilli: 3000, RevenueCents: 9000},
		// A sold dish with no inventory product mapping (retail item) —
		// ProductByMenuItem's ErrNotFound is swallowed, theoretical stays 0.
		{MenuItemID: uuid.New(), Name: "Retail Item", QtyMilli: 1000, RevenueCents: 0},
	}})
	rep, err := appWithSales.FoodCostReport(f.ctx, f.restID, "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 2 {
		t.Fatalf("FoodCostReport().Items = %+v, want 2 rows", rep.Items)
	}
	if rep.Items[0].MenuItemID != menuItem {
		t.Fatalf("FoodCostReport().Items[0] = %+v", rep.Items[0])
	}
	if rep.Items[1].TheoreticalCogsCents != 0 || rep.Items[1].FoodCostPct != 0 {
		t.Errorf("untracked item = %+v, want zero theoretical cogs and 0%% (RevenueCents=0 skips the pct branch)", rep.Items[1])
	}
	// theoretical = recipe cost (200g@6c=1200) * 3 sold = 3600.
	if rep.Items[0].TheoreticalCogsCents != 3600 {
		t.Errorf("theoretical cogs = %d, want 3600", rep.Items[0].TheoreticalCogsCents)
	}
	if rep.Items[0].FoodCostPct <= 0 {
		t.Errorf("food cost pct = %v, want > 0", rep.Items[0].FoodCostPct)
	}
	if rep.TotalActualCogsCents != 3600 { // the one sale move above, at the same 6c/g avg
		t.Errorf("actual cogs = %d, want 3600", rep.TotalActualCogsCents)
	}
	_ = dish
	_ = flour
}

// CreateReceipt/CreateWriteOff/CreateTechCardVersion reject an unknown
// product id (store lookup ErrNotFound bubbles as-is).
func TestCreateDocuments_UnknownProduct(t *testing.T) {
	f := setup(t)
	unknown := uuid.New()

	if _, err := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", []inventoryapp.ReceiptLineInput{{ProductID: unknown, QtyInput: 1, Unit: inv.UnitG, UnitPriceCents: 1}}, f.userID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("receipt unknown product: got %v, want ErrNotFound", err)
	}
	if _, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonLoss, day(2026, 1, 1), "", []inventoryapp.WriteOffLineInput{{ProductID: unknown, QtyInput: 1, Unit: inv.UnitG}}); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("write-off unknown product: got %v, want ErrNotFound", err)
	}
}

func TestCreateReceipt_NegativePrice(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	if _, err := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", []inventoryapp.ReceiptLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG, UnitPriceCents: -1}}, f.userID); !errors.Is(err, inventoryapp.ErrInvalid) {
		t.Errorf("negative price: got %v, want ErrInvalid", err)
	}
}

func TestCreateWriteOff_InvalidReason(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	if _, err := f.app.CreateWriteOff(f.ctx, f.restID, "bogus", day(2026, 1, 1), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}}); !errors.Is(err, inv.ErrInvalidReason) {
		t.Errorf("bad reason: got %v, want ErrInvalidReason", err)
	}
	if _, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonLoss, day(2026, 1, 1), "", nil); !errors.Is(err, inv.ErrEmptyDocument) {
		t.Errorf("empty lines: got %v, want ErrEmptyDocument", err)
	}
}

func TestCreateReceipt_EmptyDocument(t *testing.T) {
	f := setup(t)
	if _, err := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", nil, f.userID); !errors.Is(err, inv.ErrEmptyDocument) {
		t.Errorf("empty receipt: got %v, want ErrEmptyDocument", err)
	}
}

// Posting/cancelling a document twice hits the already-posted /
// already-cancelled guards.
func TestPostReceipt_AlreadyPosted(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	r, err := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", []inventoryapp.ReceiptLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG, UnitPriceCents: 1}}, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r.ID, f.userID); !errors.Is(err, inventoryapp.ErrAlreadyPosted) {
		t.Errorf("re-post: got %v, want ErrAlreadyPosted", err)
	}
	if _, err := f.app.CancelReceipt(f.ctx, f.restID, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelReceipt(f.ctx, f.restID, r.ID); !errors.Is(err, inventoryapp.ErrAlreadyCancelled) {
		t.Errorf("re-cancel: got %v, want ErrAlreadyCancelled", err)
	}
}

func TestCancelReceipt_NotPosted(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	r, err := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", []inventoryapp.ReceiptLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG, UnitPriceCents: 1}}, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelReceipt(f.ctx, f.restID, r.ID); !errors.Is(err, inventoryapp.ErrNotPosted) {
		t.Errorf("cancel a draft: got %v, want ErrNotPosted", err)
	}
}

// TechCardVersion rejects an empty/duplicate/invalid recipe and an
// unknown ingredient.
func TestCreateTechCardVersion_InvalidInputs(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, &menuItem)
	goods := f.product(t, "SALT", "Salt", inv.TypeGoods, inv.UnitG, nil)

	// tech cards belong only to dish/prepared products.
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, goods.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{}, f.userID); err == nil {
		t.Error("tech card on goods product: got nil error")
	}
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), "bogus", 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: goods.ID, QtyInput: 1, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inv.ErrInvalidConsumption) {
		t.Errorf("bad consumption: got %v, want ErrInvalidConsumption", err)
	}
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{Format: "bogus"},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: goods.ID, QtyInput: 1, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inv.ErrInvalidFormat) {
		t.Errorf("bad format: got %v, want ErrInvalidFormat", err)
	}
	unknown := uuid.New()
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: unknown, QtyInput: 1, Unit: inv.UnitG}}, f.userID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("unknown ingredient: got %v, want ErrNotFound", err)
	}
}

// PostReceipt on an already-posted-then-backdated product is rejected
// even on the first post if a later receipt for the same product already
// landed (checkBackdate's cross-document guard, not just same-document).
func TestPostReceipt_BackdateAgainstOtherDocument(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 10), flour.ID, 100, inv.UnitG, 5)

	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonLoss, day(2026, 1, 1), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 10, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); !errors.Is(err, inventoryapp.ErrBackdated) {
		t.Errorf("write-off backdated against a receipt: got %v, want ErrBackdated", err)
	}
}
