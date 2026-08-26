package postgres

import (
	"errors"
	"testing"

	inv "aivo/internal/domain/inventory"
	inventoryapp "aivo/internal/inventory/app"

	"uuid"
)

// Read/list/patch paths not already exercised by the flow-focused tests
// above: products, suppliers, tech-card reads, document lists/cancels,
// on-hand/stock-move reads, and the food-cost report.

func TestProductsListGetPatch(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, &menuItem)

	list, err := f.app.Products(f.ctx, f.restID)
	if err != nil || len(list) != 1 {
		t.Fatalf("Products() = %v, %v, want 1 product", list, err)
	}

	p, oh, err := f.app.Product(f.ctx, f.restID, dish.ID)
	if err != nil || p.ID != dish.ID || oh.QtyMilli != 0 {
		t.Fatalf("Product() = %+v, %+v, %v", p, oh, err)
	}

	newName := "Borscht"
	minStock := int64(5000) // milli-units
	updated, err := f.app.UpdateProduct(f.ctx, f.restID, dish.ID, inventoryapp.ProductPatch{Name: &newName, MinStock: &minStock})
	if err != nil || updated.Name != "Borscht" || updated.MinStock == nil || *updated.MinStock != 5000 {
		t.Fatalf("UpdateProduct() = %+v, %v", updated, err)
	}
	archived := true
	updated, err = f.app.UpdateProduct(f.ctx, f.restID, dish.ID, inventoryapp.ProductPatch{Archived: &archived, ClearMin: true})
	if err != nil || !updated.Archived || updated.MinStock != nil {
		t.Fatalf("UpdateProduct(archive+clear min) = %+v, %v", updated, err)
	}
	updated, err = f.app.UpdateProduct(f.ctx, f.restID, dish.ID, inventoryapp.ProductPatch{ClearMenu: true})
	if err != nil || updated.MenuItemID != nil {
		t.Fatalf("UpdateProduct(clear menu) = %+v, %v", updated, err)
	}
	// Re-attach a menu item, then attempt to steal an already-taken one.
	other := f.product(t, "SALAD", "Salad", inv.TypeDish, inv.UnitPcs, nil)
	otherMenuItem := uuid.New()
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, other.ID, inventoryapp.ProductPatch{MenuItemID: &otherMenuItem}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, dish.ID, inventoryapp.ProductPatch{MenuItemID: &otherMenuItem}); !errors.Is(err, inventoryapp.ErrMenuItemTaken) {
		t.Errorf("steal menu item: got %v, want ErrMenuItemTaken", err)
	}
	// A goods product can never take a menu item.
	goods := f.product(t, "SALT", "Salt", inv.TypeGoods, inv.UnitG, nil)
	yetAnother := uuid.New()
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, goods.ID, inventoryapp.ProductPatch{MenuItemID: &yetAnother}); !errors.Is(err, inv.ErrMenuItemOnNonDish) {
		t.Errorf("menu item on goods: got %v, want ErrMenuItemOnNonDish", err)
	}
}

func TestCreateProduct_MenuItemTaken(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, &menuItem)
	_, err := f.app.CreateProduct(f.ctx, f.restID, inventoryapp.ProductInput{SKU: "SOUP2", Name: "Soup 2", Type: string(inv.TypeDish), StockUnit: inv.UnitPcs, MenuItemID: &menuItem})
	if !errors.Is(err, inventoryapp.ErrMenuItemTaken) {
		t.Errorf("duplicate menu item: got %v, want ErrMenuItemTaken", err)
	}
}

func TestCreateProduct_SKUTaken(t *testing.T) {
	f := setup(t)
	f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	_, err := f.app.CreateProduct(f.ctx, f.restID, inventoryapp.ProductInput{SKU: "FLOUR", Name: "Flour 2", Type: string(inv.TypeGoods), StockUnit: inv.UnitG})
	if !errors.Is(err, inventoryapp.ErrSKUTaken) {
		t.Errorf("dup sku: got %v, want ErrSKUTaken", err)
	}
}

func TestSuppliersCRUD(t *testing.T) {
	f := setup(t)
	s, err := f.app.CreateSupplier(f.ctx, f.restID, "Acme", map[string]string{"phone": "123"}, "note")
	if err != nil {
		t.Fatal(err)
	}
	list, err := f.app.Suppliers(f.ctx, f.restID)
	if err != nil || len(list) != 1 {
		t.Fatalf("Suppliers() = %v, %v", list, err)
	}
	newName := "Acme Foods"
	updated, err := f.app.UpdateSupplier(f.ctx, f.restID, s.ID, inventoryapp.SupplierPatch{Name: &newName})
	if err != nil || updated.Name != "Acme Foods" {
		t.Fatalf("UpdateSupplier() = %+v, %v", updated, err)
	}
	if _, err := f.app.CreateSupplier(f.ctx, f.restID, "", nil, ""); !errors.Is(err, inventoryapp.ErrInvalid) {
		t.Errorf("empty name: got %v, want ErrInvalid", err)
	}
	if _, err := f.app.CreateSupplier(f.ctx, f.restID, "Acme Foods", nil, ""); !errors.Is(err, inventoryapp.ErrSupplierNameTaken) {
		t.Errorf("dup name: got %v, want ErrSupplierNameTaken", err)
	}
	if _, err := f.app.UpdateSupplier(f.ctx, f.restID, s.ID, inventoryapp.SupplierPatch{Contacts: map[string]string{"x": "y"}, Archived: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
}

func TestTechCardReadsAndRecost(t *testing.T) {
	f := setup(t)
	dish, _, _ := f.borschtWithFlourCard(t)

	versions, err := f.app.TechCardVersions(f.ctx, f.restID, dish.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("TechCardVersions() = %v, %v", versions, err)
	}
	active, err := f.app.ActiveTechCard(f.ctx, f.restID, dish.ID, day(2026, 1, 2))
	if err != nil || active.Card.ID != versions[0].Card.ID {
		t.Fatalf("ActiveTechCard() = %+v, %v", active, err)
	}
	byID, err := f.app.TechCard(f.ctx, f.restID, versions[0].Card.ID)
	if err != nil || byID.Card.ID != versions[0].Card.ID {
		t.Fatalf("TechCard() = %+v, %v", byID, err)
	}
	cost, err := f.app.Recost(f.ctx, f.restID, versions[0].Card.ID, f.userID)
	if err != nil || cost != 1200 { // 200g @ 6c
		t.Errorf("Recost() = %d, %v, want 1200, nil", cost, err)
	}
}

func TestDocumentListsAndCancels(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 1000, inv.UnitG, 5)

	receipts, err := f.app.Receipts(f.ctx, f.restID, "", "")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("Receipts() = %v, %v", receipts, err)
	}
	got, err := f.app.Receipt(f.ctx, f.restID, receipts[0].ID)
	if err != nil || got.ID != receipts[0].ID {
		t.Fatalf("Receipt() = %+v, %v", got, err)
	}

	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonSpoilage, day(2026, 1, 2), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 100, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	woList, err := f.app.WriteOffs(f.ctx, f.restID, "", "")
	if err != nil || len(woList) != 1 {
		t.Fatalf("WriteOffs() = %v, %v", woList, err)
	}
	gotWO, err := f.app.WriteOff(f.ctx, f.restID, w.ID)
	if err != nil || gotWO.ID != w.ID {
		t.Fatalf("WriteOff() = %+v, %v", gotWO, err)
	}
	if _, err := f.app.CancelWriteOff(f.ctx, f.restID, w.ID); err != nil {
		t.Fatalf("CancelWriteOff() = %v", err)
	}
	assertEventExists(t, f, inventoryapp.EventWriteOffCancelled, w.ID)

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 3), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.EnterCounts(f.ctx, f.restID, st.ID, []inventoryapp.StocktakeCountInput{{ProductID: flour.ID, QtyInput: 500, Unit: inv.UnitG}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	stList, err := f.app.Stocktakes(f.ctx, f.restID, "")
	if err != nil || len(stList) != 1 {
		t.Fatalf("Stocktakes() = %v, %v", stList, err)
	}
	gotST, err := f.app.Stocktake(f.ctx, f.restID, st.ID)
	if err != nil || gotST.ID != st.ID {
		t.Fatalf("Stocktake() = %+v, %v", gotST, err)
	}
	if _, err := f.app.CancelStocktake(f.ctx, f.restID, st.ID); err != nil {
		t.Fatalf("CancelStocktake() = %v", err)
	}
	assertEventExists(t, f, inventoryapp.EventStocktakeCancelled, st.ID)

	// Second open stocktake conflicts while the first is still draft.
	if _, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 4), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 5), ""); !errors.Is(err, inventoryapp.ErrStocktakeOpen) {
		t.Errorf("second open stocktake: got %v, want ErrStocktakeOpen", err)
	}
}

func TestOnHandAndStockMovesReads(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	minStock := int64(2_000_000) // 2000g
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, flour.ID, inventoryapp.ProductPatch{MinStock: &minStock}); err != nil {
		t.Fatal(err)
	}
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 1000, inv.UnitG, 5) // below the 2000g min

	// An archived product is excluded from OnHand entirely.
	sugar := f.product(t, "SUGAR", "Sugar", inv.TypeGoods, inv.UnitG, nil)
	archived := true
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, sugar.ID, inventoryapp.ProductPatch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}

	rows, err := f.app.OnHand(f.ctx, f.restID, false)
	if err != nil || len(rows) != 1 || !rows[0].BelowMin {
		t.Fatalf("OnHand() = %+v, %v, want 1 row below min (archived sugar excluded)", rows, err)
	}
	lowOnly, err := f.app.OnHand(f.ctx, f.restID, true)
	if err != nil || len(lowOnly) != 1 {
		t.Fatalf("OnHand(low_stock) = %+v, %v", lowOnly, err)
	}

	moves, err := f.app.StockMoves(f.ctx, f.restID, "", &flour.ID)
	if err != nil || len(moves) != 1 {
		t.Fatalf("StockMoves() = %v, %v", moves, err)
	}
}

func TestFoodCostReport(t *testing.T) {
	f := setup(t)
	_, _, _ = f.borschtWithFlourCard(t)
	rep, err := f.app.FoodCostReport(f.ctx, f.restID, "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatalf("FoodCostReport() error = %v", err)
	}
	// noSales{} returns nothing sold, so the report is empty but errorless.
	if len(rep.Items) != 0 {
		t.Errorf("FoodCostReport().Items = %v, want empty (noSales fixture)", rep.Items)
	}
	if _, err := f.app.FoodCostReport(f.ctx, f.restID, "2026-01-01", "not-a-date"); err == nil {
		t.Error("FoodCostReport(bad to date) = nil error, want error")
	}
}

func TestApp_Now(t *testing.T) {
	f := setup(t)
	if f.app.Now().IsZero() {
		t.Error("Now() returned zero time")
	}
}

func boolPtr(b bool) *bool { return &b }
