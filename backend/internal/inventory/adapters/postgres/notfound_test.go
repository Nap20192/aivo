package postgres

import (
	"errors"
	"testing"

	inv "aivo/internal/domain/inventory"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"

	"uuid"
)

// Not-found / wrong-state reads and guards not already exercised — the
// last stretch of app-package branch coverage.

func TestNotFoundReads(t *testing.T) {
	f := setup(t)
	unknown := uuid.New()

	if _, _, err := f.app.Product(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Product(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.UpdateProduct(f.ctx, f.restID, unknown, inventoryapp.ProductPatch{}); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("UpdateProduct(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.Receipt(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Receipt(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.WriteOff(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("WriteOff(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.Stocktake(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Stocktake(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.TechCard(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("TechCard(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.ActiveTechCard(f.ctx, f.restID, unknown, day(2026, 1, 1)); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("ActiveTechCard(unknown product): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.Recost(f.ctx, f.restID, unknown, f.userID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Recost(unknown card): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.UpdateSupplier(f.ctx, f.restID, unknown, inventoryapp.SupplierPatch{}); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("UpdateSupplier(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.PostReceipt(f.ctx, f.restID, unknown, f.userID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("PostReceipt(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.CancelWriteOff(f.ctx, f.restID, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("CancelWriteOff(unknown): got %v, want ErrNotFound", err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, unknown, f.userID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("PostStocktake(unknown): got %v, want ErrNotFound", err)
	}
}

// EnterCounts on a posted (non-draft) stocktake is rejected.
func TestEnterCounts_NotDraft(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 100, inv.UnitG, 5)

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 2), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.EnterCounts(f.ctx, f.restID, st.ID, []inventoryapp.StocktakeCountInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}}); !errors.Is(err, inventoryapp.ErrNotDraft) {
		t.Errorf("EnterCounts on posted: got %v, want ErrNotDraft", err)
	}
}

// UpdateProduct's ClearMenu/MenuItemID no-op branches: re-assigning the
// SAME menu item id it already has skips the "is it taken" check.
func TestUpdateProduct_SameMenuItemIsNoop(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, &menuItem)
	updated, err := f.app.UpdateProduct(f.ctx, f.restID, dish.ID, inventoryapp.ProductPatch{MenuItemID: &menuItem})
	if err != nil || updated.MenuItemID == nil || *updated.MenuItemID != menuItem {
		t.Fatalf("UpdateProduct(same menu item) = %+v, %v", updated, err)
	}
}

// A cancelled write-off can't be cancelled again, and a draft can't be
// cancelled (only posted documents can).
func TestCancelWriteOff_Guards(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 100, inv.UnitG, 5)

	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonLoss, day(2026, 1, 2), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelWriteOff(f.ctx, f.restID, w.ID); !errors.Is(err, inventoryapp.ErrNotPosted) {
		t.Errorf("cancel a draft write-off: got %v, want ErrNotPosted", err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelWriteOff(f.ctx, f.restID, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelWriteOff(f.ctx, f.restID, w.ID); !errors.Is(err, inventoryapp.ErrAlreadyCancelled) {
		t.Errorf("re-cancel write-off: got %v, want ErrAlreadyCancelled", err)
	}
}

// PostWriteOff twice hits the already-posted guard.
func TestPostWriteOff_AlreadyPosted(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 100, inv.UnitG, 5)
	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonLoss, day(2026, 1, 2), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); !errors.Is(err, inventoryapp.ErrAlreadyPosted) {
		t.Errorf("re-post write-off: got %v, want ErrAlreadyPosted", err)
	}
}

// PostStocktake twice hits the already-posted guard, and cancelling a
// draft stocktake is rejected.
func TestStocktake_PostAndCancelGuards(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 100, inv.UnitG, 5)

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 2), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CancelStocktake(f.ctx, f.restID, st.ID); !errors.Is(err, inventoryapp.ErrNotPosted) {
		t.Errorf("cancel a draft stocktake: got %v, want ErrNotPosted", err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); !errors.Is(err, inventoryapp.ErrAlreadyPosted) {
		t.Errorf("re-post stocktake: got %v, want ErrAlreadyPosted", err)
	}
}
