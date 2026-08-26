package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// TechCardLineInput is one recipe line as entered (display unit).
// YieldPermille is optional (0 = default 100%, no cooking loss).
type TechCardLineInput struct {
	IngredientProductID uuid.UUID
	QtyInput            float64
	Unit                string
	YieldPermille       int
}

// TechCardMeta bundles the tech card's document format and its ГОСТ
// 31987-2012 ТТК text fields (§ scope/presentation/storage/organoleptic —
// meaningful when Format is FormatTTK, stored regardless). Format defaults
// to FormatSimple when empty.
type TechCardMeta struct {
	Format           inv.TechCardFormat
	ScopeNote        *string
	PresentationNote *string
	StorageNote      *string
	OrganolepticNote *string
}

// CreateTechCardVersion creates a calendar-versioned recipe: it closes the
// version active on valid_from (backdated create = ordinary interval, D5),
// inserts the new open version + lines, and records the first costing — all
// in one transaction (§11). Rejects cycles, duplicate/empty lines, and a
// second version starting on the same day.
func (a *App) CreateTechCardVersion(ctx context.Context, restaurantID, productID uuid.UUID, validFrom time.Time, consumption inv.ConsumeStrategy, yieldMilli int64, meta TechCardMeta, lineInputs []TechCardLineInput, createdBy uuid.UUID) (inv.TechCard, error) {
	product, err := a.store.ProductByID(ctx, restaurantID, productID)
	if err != nil {
		return inv.TechCard{}, err
	}
	if product.Type != inv.TypeDish && product.Type != inv.TypePrepared {
		return inv.TechCard{}, fmt.Errorf("%w: tech cards belong to a dish or prepared product", ErrInvalid)
	}
	if !consumption.Valid() {
		return inv.TechCard{}, inv.ErrInvalidConsumption
	}
	if yieldMilli <= 0 {
		yieldMilli = inv.MilliPerUnit
	}
	if meta.Format == "" {
		meta.Format = inv.DefaultTechCardFormat()
	}
	if !meta.Format.Valid() {
		return inv.TechCard{}, inv.ErrInvalidFormat
	}

	tcID := a.newID()
	lines := make([]inv.TechCardLine, len(lineInputs))
	ingredientIDs := make([]uuid.UUID, len(lineInputs))
	for i, li := range lineInputs {
		ing, err := a.store.ProductByID(ctx, restaurantID, li.IngredientProductID)
		if err != nil {
			return inv.TechCard{}, err
		}
		qty, err := inv.ToBaseMilli(li.QtyInput, li.Unit, ing.StockUnit)
		if err != nil {
			return inv.TechCard{}, err
		}
		yield := li.YieldPermille
		if yield <= 0 {
			yield = inv.YieldPermilleDefault
		}
		lines[i] = inv.TechCardLine{
			ID: a.newID(), TechCardID: tcID, IngredientProductID: li.IngredientProductID,
			Qty: qty, Seq: i + 1, YieldPermille: yield,
		}
		ingredientIDs[i] = li.IngredientProductID
	}
	if err := inv.ValidateLines(lines); err != nil {
		return inv.TechCard{}, err
	}

	// Cycle check: overlay the new recipe on the active graph.
	adj, err := a.store.ActiveRecipeGraph(ctx, restaurantID, validFrom)
	if err != nil {
		return inv.TechCard{}, err
	}
	adj[productID] = ingredientIDs
	if inv.ReachesSelf(productID, adj) {
		return inv.TechCard{}, inv.ErrRecipeCycle
	}

	tc := inv.TechCard{
		ID: tcID, RestaurantID: restaurantID, ProductID: productID,
		ValidFrom: validFrom, ValidTo: nil, Consumption: consumption,
		YieldMilli: yieldMilli, CreatedBy: createdBy, Lines: lines,
		Format: meta.Format, ScopeNote: meta.ScopeNote, PresentationNote: meta.PresentationNote,
		StorageNote: meta.StorageNote, OrganolepticNote: meta.OrganolepticNote,
	}
	err = a.store.InTx(ctx, func(_ *sql.Tx, st ports.Store) error {
		active, err := st.ActiveTechCard(ctx, restaurantID, productID, validFrom)
		switch {
		case err == nil:
			if sameDate(active.ValidFrom, validFrom) {
				return ErrVersionExists
			}
			if err := st.CloseTechCard(ctx, active.ID, validFrom); err != nil {
				return err
			}
		case errors.Is(err, ports.ErrNotFound):
			// no active version on that date
		default:
			return err
		}
		if err := st.InsertTechCard(ctx, tc); err != nil {
			if errors.Is(err, ports.ErrConflict) {
				return ErrVersionExists
			}
			return err
		}
		cost, err := a.recipeCostOn(ctx, st, restaurantID, tc, validFrom, map[uuid.UUID]bool{})
		if err != nil {
			return err
		}
		return st.InsertCosting(ctx, inv.RecipeCosting{
			ID: a.newID(), TechCardID: tc.ID, CostCents: cost,
			Method: inv.CostMethodWeightedAvg, ComputedBy: createdBy,
		})
	})
	if err != nil {
		return inv.TechCard{}, err
	}
	return tc, nil
}

// Recost recomputes and appends a new costing entry (append-only series) at
// the current date, returning the fresh cost.
func (a *App) Recost(ctx context.Context, restaurantID, techCardID, computedBy uuid.UUID) (int64, error) {
	tc, err := a.store.TechCardByID(ctx, restaurantID, techCardID)
	if err != nil {
		return 0, err
	}
	cost, err := a.recipeCostOn(ctx, a.store, restaurantID, tc, a.now(), map[uuid.UUID]bool{})
	if err != nil {
		return 0, err
	}
	if err := a.store.InsertCosting(ctx, inv.RecipeCosting{
		ID: a.newID(), TechCardID: techCardID, CostCents: cost,
		Method: inv.CostMethodWeightedAvg, ComputedBy: computedBy,
	}); err != nil {
		return 0, err
	}
	return cost, nil
}

// recipeCostOn computes a version's total recipe cost on date `on`: Σ over
// lines of qty × the ingredient's cost per base unit. Goods/modifiers cost
// their moving-average; dish/prepared ingredients recurse into their active
// card (one traversal; `seen` guards against a data cycle defensively).
func (a *App) recipeCostOn(ctx context.Context, st ports.Store, restaurantID uuid.UUID, tc inv.TechCard, on time.Time, seen map[uuid.UUID]bool) (int64, error) {
	if seen[tc.ProductID] {
		return 0, nil // defensive cycle break (creation already rejects cycles)
	}
	seen[tc.ProductID] = true
	defer delete(seen, tc.ProductID)

	var total int64
	for _, l := range tc.Lines {
		unit, err := a.unitCostOf(ctx, st, restaurantID, l.IngredientProductID, on, seen)
		if err != nil {
			return 0, err
		}
		total += inv.LineCost(l.Qty, unit)
	}
	return total, nil
}

// unitCostOf is a product's cost per base unit on date `on`: moving-average
// for goods/modifier, recursive recipe cost per yield for dish/prepared.
func (a *App) unitCostOf(ctx context.Context, st ports.Store, restaurantID, productID uuid.UUID, on time.Time, seen map[uuid.UUID]bool) (int64, error) {
	p, err := st.ProductByID(ctx, restaurantID, productID)
	if err != nil {
		return 0, err
	}
	if p.Type == inv.TypeGoods || p.Type == inv.TypeModifier {
		oh, err := st.OnHand(ctx, restaurantID, productID)
		if err != nil {
			return 0, err
		}
		return oh.AvgCentsPerBase(), nil
	}
	// dish / prepared: recurse into its active card.
	card, err := st.ActiveTechCard(ctx, restaurantID, productID, on)
	if errors.Is(err, ports.ErrNotFound) {
		return 0, nil // no recipe → no theoretical cost
	}
	if err != nil {
		return 0, err
	}
	recipe, err := a.recipeCostOn(ctx, st, restaurantID, card, on, seen)
	if err != nil {
		return 0, err
	}
	return inv.UnitCostFromRecipe(recipe, card.YieldMilli), nil
}

// TechCardView bundles a version with its current cost (list/detail).
type TechCardView struct {
	Card      inv.TechCard
	CostCents int64
	Costings  []inv.RecipeCosting // detail only
}

func (a *App) TechCardVersions(ctx context.Context, restaurantID, productID uuid.UUID) ([]TechCardView, error) {
	cards, err := a.store.TechCardsByProduct(ctx, restaurantID, productID)
	if err != nil {
		return nil, err
	}
	out := make([]TechCardView, len(cards))
	for i, c := range cards {
		cost, _, err := a.store.LatestCostCents(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		out[i] = TechCardView{Card: c, CostCents: cost}
	}
	return out, nil
}

func (a *App) ActiveTechCard(ctx context.Context, restaurantID, productID uuid.UUID, on time.Time) (TechCardView, error) {
	card, err := a.store.ActiveTechCard(ctx, restaurantID, productID, on)
	if err != nil {
		return TechCardView{}, err
	}
	cost, _, err := a.store.LatestCostCents(ctx, card.ID)
	if err != nil {
		return TechCardView{}, err
	}
	return TechCardView{Card: card, CostCents: cost}, nil
}

func (a *App) TechCard(ctx context.Context, restaurantID, id uuid.UUID) (TechCardView, error) {
	card, err := a.store.TechCardByID(ctx, restaurantID, id)
	if err != nil {
		return TechCardView{}, err
	}
	costings, err := a.store.Costings(ctx, id)
	if err != nil {
		return TechCardView{}, err
	}
	var cost int64
	if len(costings) > 0 {
		cost = costings[0].CostCents // ordered desc
	}
	return TechCardView{Card: card, CostCents: cost, Costings: costings}, nil
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
