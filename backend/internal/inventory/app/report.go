package app

import (
	"context"
	"errors"

	"aivo/internal/inventory/ports"

	"uuid"
)

// FoodCostRow is a per-menu-item food-cost line.
type FoodCostRow struct {
	MenuItemID           uuid.UUID
	Name                 string
	RevenueCents         int64
	TheoreticalCogsCents int64
	FoodCostPct          float64 // theoretical / revenue × 100
}

// FoodCostReport is theoretical vs actual COGS over a period (§8). Actual
// COGS is reported at the total level (a sale move is on a shared ingredient
// and does not attribute cleanly to a single menu item — per-item actual is
// a documented deferral); theoretical is per item from the active recipe.
type FoodCostReport struct {
	Items                     []FoodCostRow
	TotalRevenueCents         int64
	TotalActualCogsCents      int64
	TotalTheoreticalCogsCents int64
	EstimatedShare            float64 // fraction of sale moves with estimated cost
}

func (a *App) FoodCostReport(ctx context.Context, restaurantID uuid.UUID, from, to string) (FoodCostReport, error) {
	toDate, err := ParseDate(to)
	if err != nil {
		return FoodCostReport{}, err
	}
	sold, err := a.sales.SoldDishes(ctx, restaurantID, from, to)
	if err != nil {
		return FoodCostReport{}, err
	}
	var rep FoodCostReport
	for _, s := range sold {
		row := FoodCostRow{MenuItemID: s.MenuItemID, Name: s.Name, RevenueCents: s.RevenueCents}
		// theoretical = recipe cost of the active card × units sold.
		if p, err := a.store.ProductByMenuItem(ctx, restaurantID, s.MenuItemID); err == nil {
			if card, err := a.store.ActiveTechCard(ctx, restaurantID, p.ID, toDate); err == nil {
				if cost, ok, err := a.store.LatestCostCents(ctx, card.ID); err != nil {
					return FoodCostReport{}, err
				} else if ok {
					row.TheoreticalCogsCents = cost * s.QtyMilli / 1000
				}
			} else if !errors.Is(err, ports.ErrNotFound) {
				return FoodCostReport{}, err
			}
		} else if !errors.Is(err, ports.ErrNotFound) {
			return FoodCostReport{}, err
		}
		if row.RevenueCents > 0 {
			row.FoodCostPct = float64(row.TheoreticalCogsCents) / float64(row.RevenueCents) * 100
		}
		rep.Items = append(rep.Items, row)
		rep.TotalRevenueCents += row.RevenueCents
		rep.TotalTheoreticalCogsCents += row.TheoreticalCogsCents
	}

	actual, err := a.store.SaleCostByProduct(ctx, restaurantID, from, to)
	if err != nil {
		return FoodCostReport{}, err
	}
	for _, c := range actual {
		rep.TotalActualCogsCents += c
	}
	total, estimated, err := a.store.SaleEstimatedShare(ctx, restaurantID, from, to)
	if err != nil {
		return FoodCostReport{}, err
	}
	if total > 0 {
		rep.EstimatedShare = float64(estimated) / float64(total)
	}
	return rep, nil
}
