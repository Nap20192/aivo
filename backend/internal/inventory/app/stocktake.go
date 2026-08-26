package app

import (
	"context"
	"database/sql"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// StartStocktake opens a draft count (≤1 open per restaurant).
func (a *App) StartStocktake(ctx context.Context, restaurantID uuid.UUID, businessDate time.Time, note string) (inv.Stocktake, error) {
	st := inv.Stocktake{ID: a.newID(), RestaurantID: restaurantID, Status: inv.DocDraft, BusinessDate: businessDate, Note: note}
	if err := a.store.InsertStocktake(ctx, st); err != nil {
		if err == ports.ErrConflict {
			return inv.Stocktake{}, ErrStocktakeOpen
		}
		return inv.Stocktake{}, err
	}
	return st, nil
}

func (a *App) Stocktake(ctx context.Context, restaurantID, id uuid.UUID) (inv.Stocktake, error) {
	return a.store.StocktakeByID(ctx, restaurantID, id)
}

func (a *App) Stocktakes(ctx context.Context, restaurantID uuid.UUID, status string) ([]inv.Stocktake, error) {
	return a.store.Stocktakes(ctx, restaurantID, status)
}

// StocktakeCountInput is one counted line.
type StocktakeCountInput struct {
	ProductID uuid.UUID
	QtyInput  float64
	Unit      string
}

// EnterCounts replaces the draft's counted lines (draft only).
func (a *App) EnterCounts(ctx context.Context, restaurantID, stocktakeID uuid.UUID, counts []StocktakeCountInput) (inv.Stocktake, error) {
	st, err := a.store.StocktakeByID(ctx, restaurantID, stocktakeID)
	if err != nil {
		return inv.Stocktake{}, err
	}
	if st.Status != inv.DocDraft {
		return inv.Stocktake{}, ErrNotDraft
	}
	lines := make([]inv.StocktakeLine, len(counts))
	for i, c := range counts {
		p, err := a.store.ProductByID(ctx, restaurantID, c.ProductID)
		if err != nil {
			return inv.Stocktake{}, err
		}
		counted, err := toBaseAllowZero(c.QtyInput, c.Unit, p.StockUnit)
		if err != nil {
			return inv.Stocktake{}, err
		}
		lines[i] = inv.StocktakeLine{ID: a.newID(), StocktakeID: stocktakeID, ProductID: c.ProductID, CountedQtyMilli: counted, Seq: i + 1}
	}
	if err := a.store.ReplaceStocktakeLines(ctx, stocktakeID, lines); err != nil {
		return inv.Stocktake{}, err
	}
	return a.store.StocktakeByID(ctx, restaurantID, stocktakeID)
}

// DryRunRow is a computed variance line (no state change).
type DryRunRow struct {
	ProductID         uuid.UUID
	CountedQtyMilli   int64
	ExpectedQtyMilli  int64
	VarianceQtyMilli  int64
	VarianceCostCents int64 // + surplus, − shortage
}

// DryRun computes expected/variance against current on-hand, saving
// nothing (read-only, idempotent — D2 §4 / refuted §15.2).
func (a *App) DryRun(ctx context.Context, restaurantID, stocktakeID uuid.UUID) ([]DryRunRow, error) {
	st, err := a.store.StocktakeByID(ctx, restaurantID, stocktakeID)
	if err != nil {
		return nil, err
	}
	out := make([]DryRunRow, len(st.Lines))
	for i, l := range st.Lines {
		oh, err := a.store.OnHand(ctx, restaurantID, l.ProductID)
		if err != nil {
			return nil, err
		}
		variance := l.CountedQtyMilli - oh.QtyMilli
		out[i] = DryRunRow{
			ProductID: l.ProductID, CountedQtyMilli: l.CountedQtyMilli, ExpectedQtyMilli: oh.QtyMilli,
			VarianceQtyMilli: variance, VarianceCostCents: varianceCost(oh, variance),
		}
	}
	return out, nil
}

// PostStocktake fixes expected at now, posts surplus/shortage moves and the
// aggregate GL document, in one transaction (§6.4).
func (a *App) PostStocktake(ctx context.Context, restaurantID, stocktakeID, postedBy uuid.UUID) (inv.Stocktake, error) {
	err := a.store.InTx(ctx, func(tx *sql.Tx, store ports.Store) error {
		status, err := store.LockDocument(ctx, inv.DocKindStocktake, restaurantID, stocktakeID)
		if err != nil {
			return err
		}
		if err := postGuard(status); err != nil {
			return err
		}
		st, err := store.StocktakeByID(ctx, restaurantID, stocktakeID)
		if err != nil {
			return err
		}
		var shortageTotal, surplusTotal int64
		lines := make([]inv.StocktakeLine, len(st.Lines))
		for i, l := range st.Lines {
			oh, err := store.LockOnHand(ctx, restaurantID, l.ProductID)
			if err != nil {
				return err
			}
			expected := oh.QtyMilli
			variance := l.CountedQtyMilli - expected
			var vcost int64
			switch {
			case variance > 0:
				cost, err := a.applySurplusMove(ctx, store, restaurantID, l.ProductID, variance, st.BusinessDate, stocktakeID)
				if err != nil {
					return err
				}
				surplusTotal += cost
				vcost = cost
			case variance < 0:
				cost, _, err := a.applyIssueMove(ctx, store, restaurantID, l.ProductID, inv.MoveStocktakeShortage, -variance, st.BusinessDate, inv.DocKindStocktake, stocktakeID, nil)
				if err != nil {
					return err
				}
				shortageTotal += cost
				vcost = -cost
			}
			exp := expected
			lines[i] = inv.StocktakeLine{
				ID: l.ID, StocktakeID: stocktakeID, ProductID: l.ProductID, CountedQtyMilli: l.CountedQtyMilli,
				ExpectedQtyMilli: &exp, VarianceQtyMilli: variance, VarianceCostCents: vcost, Seq: l.Seq,
			}
		}
		if err := store.ReplaceStocktakeLines(ctx, stocktakeID, lines); err != nil {
			return err
		}
		glLines := []ports.JournalLine{
			{Purpose: "inventory_shrinkage", Side: "debit", AmountCents: shortageTotal},
			{Purpose: "inventory", Side: "credit", AmountCents: shortageTotal},
			{Purpose: "inventory", Side: "debit", AmountCents: surplusTotal},
			{Purpose: "inventory_surplus", Side: "credit", AmountCents: surplusTotal},
		}
		if err := a.publishPostJournal(ctx, tx, EventStocktakePosted, inv.DocKindStocktake, restaurantID, postedBy, stocktakeID, st.BusinessDate, glLines); err != nil {
			return err
		}
		return store.MarkStocktakeStatus(ctx, restaurantID, stocktakeID, inv.DocDraft, inv.DocPosted, &postedBy)
	})
	if err != nil {
		return inv.Stocktake{}, err
	}
	return a.store.StocktakeByID(ctx, restaurantID, stocktakeID)
}

func (a *App) CancelStocktake(ctx context.Context, restaurantID, stocktakeID uuid.UUID) (inv.Stocktake, error) {
	err := a.cancelDocument(ctx, restaurantID, stocktakeID, inv.DocKindStocktake, EventStocktakeCancelled,
		func(ctx context.Context, store ports.Store, reversalID uuid.UUID, businessDate time.Time) error {
			return store.InsertStocktake(ctx, inv.Stocktake{
				ID: reversalID, RestaurantID: restaurantID, Status: inv.DocPosted,
				BusinessDate: businessDate, Note: "reversal", ReversalOf: &stocktakeID,
			})
		},
		func(ctx context.Context, store ports.Store, from, to string) error {
			return store.MarkStocktakeStatus(ctx, restaurantID, stocktakeID, from, to, nil)
		})
	if err != nil {
		return inv.Stocktake{}, err
	}
	return a.store.StocktakeByID(ctx, restaurantID, stocktakeID)
}

// varianceCost values a signed variance at the position's average: positive
// for a surplus, negative for a shortage.
func varianceCost(oh inv.OnHand, variance int64) int64 {
	if variance == 0 {
		return 0
	}
	mag := variance
	if mag < 0 {
		mag = -mag
	}
	cost, _ := oh.CostOfMilli(mag)
	if variance < 0 {
		return -cost
	}
	return cost
}

// toBaseAllowZero converts a display quantity that may be zero (a counted
// line can be 0) into milli-units of the base unit.
func toBaseAllowZero(qtyInput float64, inputUnit, stockUnit string) (int64, error) {
	if qtyInput == 0 {
		if !inv.ValidUnit(inputUnit) || !inv.ValidUnit(stockUnit) {
			return 0, inv.ErrInvalidUnit
		}
		return 0, nil
	}
	return inv.ToBaseMilli(qtyInput, inputUnit, stockUnit)
}
