package domain

import (
	"errors"
	"time"

	"aivo/internal/sharedkernel"
)

// ConsumeStrategy is how a sale of the product depletes stock (Domain 3).
type ConsumeStrategy string

const (
	ConsumeAssemble        ConsumeStrategy = "assemble"         // deplete the recipe's ingredients
	ConsumeDepleteFinished ConsumeStrategy = "deplete_finished" // deplete the finished product itself
)

// DefaultConsumeStrategy is the strategy assumed when none is given.
func DefaultConsumeStrategy() ConsumeStrategy { return ConsumeAssemble }

// Valid reports whether c is a known strategy.
func (c ConsumeStrategy) Valid() bool {
	return c == ConsumeAssemble || c == ConsumeDepleteFinished
}

// CostMethod is a recipe's costing method.
type CostMethod string

// CostMethodWeightedAvg is the only supported costing method today.
const CostMethodWeightedAvg CostMethod = "weighted_avg"

// DefaultCostMethod is the method assumed when none is given.
func DefaultCostMethod() CostMethod { return CostMethodWeightedAvg }

// Valid reports whether m is a known costing method.
func (m CostMethod) Valid() bool { return m == CostMethodWeightedAvg }

// TechCardFormat is a tech card's document format. Simple (default) is the
// lean costing-only card; TTK adds the ГОСТ 31987-2012
// технико-технологическая карта text sections (§
// scope/presentation/storage/organoleptic — required for a new, non-typical
// dish presented to a regulator) on top of the same recipe data. The format
// never changes costing math — it only gates which text fields a
// restaurant is expected to fill in and which print template renders.
type TechCardFormat string

const (
	FormatSimple TechCardFormat = "simple"
	FormatTTK    TechCardFormat = "ttk"
)

// DefaultTechCardFormat is the format assumed when none is given.
func DefaultTechCardFormat() TechCardFormat { return FormatSimple }

// Valid reports whether f is a known tech card format.
func (f TechCardFormat) Valid() bool { return f == FormatSimple || f == FormatTTK }

// YieldPermilleDefault is 100.0% (no cooking loss modeled) — a tech card
// line that doesn't specify a yield defaults to gross == net.
const YieldPermilleDefault = 1000

var (
	ErrInvalidConsumption  = errors.New("inventory: invalid consumption strategy")
	ErrEmptyRecipe         = errors.New("inventory: recipe needs at least one line")
	ErrDuplicateIngredient = errors.New("inventory: ingredient appears twice in a recipe")
	ErrRecipeCycle         = errors.New("inventory: recipe forms a cycle")
	ErrBadInterval         = errors.New("inventory: valid_to must be after valid_from")
	ErrInvalidFormat       = errors.New("inventory: invalid tech card format")
	ErrInvalidYieldPct     = errors.New("inventory: yield_permille must be in (0, 1000]")
)

// TechCard is a calendar-versioned recipe for a dish/prepared product (D5).
// The aggregate boundary is the version + its lines. A version's interval
// is [ValidFrom, ValidTo); the open (current) version has ValidTo == nil.
type TechCard struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	ProductID    sharedkernel.ID
	ValidFrom    time.Time  // date
	ValidTo      *time.Time // date, nil = open/current
	Consumption  ConsumeStrategy
	YieldMilli   int64 // yield quantity (informational / prepared unit cost)
	CreatedBy    sharedkernel.ID
	CreatedAt    time.Time
	Lines        []TechCardLine

	// GOST 31987-2012 ТТК fields — nullable, meaningful only when
	// Format == FormatTTK (a "новая, нетиповая" dish presented to a
	// regulator needs them); stored regardless of format so switching a
	// card to ttk later doesn't lose anything already typed.
	Format           TechCardFormat
	ScopeNote        *string // область применения
	PresentationNote *string // требования к оформлению, подаче, реализации
	StorageNote      *string // условия и сроки хранения
	OrganolepticNote *string // показатели качества и безопасности (органолептика)
}

// TechCardLine is one ingredient of a recipe version. Qty is the gross
// (as-purchased/AP) amount actually taken from stock — that's what costing
// and the stock ledger use. YieldPermille is the ГОСТ "выход"/Western
// AP→EP yield after cold/heat processing loss (‰, 1000 = 100%,
// informational for kitchen prep and ТТК printing — it never affects
// costing, which is always priced on the gross Qty actually consumed).
type TechCardLine struct {
	ID                  sharedkernel.ID
	TechCardID          sharedkernel.ID
	IngredientProductID sharedkernel.ID
	Qty                 int64 // milli-units in the ingredient's base unit (gross/AP)
	Seq                 int
	YieldPermille       int // 1..1000, default 1000 (no loss)
}

// NetQty is the edible/usable quantity after cooking loss: Qty ×
// YieldPermille / 1000 (informational — see TechCardLine doc comment).
func (l TechCardLine) NetQty() int64 {
	yp := int64(l.YieldPermille)
	if yp <= 0 {
		yp = YieldPermilleDefault
	}
	return l.Qty * yp / YieldPermilleDefault
}

// RecipeCosting is one entry in a tech card's append-only cost series
// (Domain 3 — cost is never an in-place mutated field). The current cost of
// a version is the latest entry by ComputedAt.
type RecipeCosting struct {
	ID         sharedkernel.ID
	TechCardID sharedkernel.ID
	CostCents  int64
	Method     CostMethod
	ComputedAt time.Time
	ComputedBy sharedkernel.ID
}

// ActiveOn reports whether the version is the active one on date d:
// ValidFrom ≤ d < ValidTo (or ValidTo open).
func (t TechCard) ActiveOn(d time.Time) bool {
	if d.Before(t.ValidFrom) {
		return false
	}
	return t.ValidTo == nil || d.Before(*t.ValidTo)
}

// ValidateLines checks a proposed recipe: non-empty, positive qty, no
// duplicate ingredient. (Cycle detection is separate — it needs the graph.)
func ValidateLines(lines []TechCardLine) error {
	if len(lines) == 0 {
		return ErrEmptyRecipe
	}
	seen := map[sharedkernel.ID]bool{}
	for _, l := range lines {
		if l.Qty <= 0 {
			return ErrInvalidQty
		}
		if l.YieldPermille < 0 || l.YieldPermille > YieldPermilleDefault {
			return ErrInvalidYieldPct
		}
		if seen[l.IngredientProductID] {
			return ErrDuplicateIngredient
		}
		seen[l.IngredientProductID] = true
	}
	return nil
}

// ReachesSelf reports whether start is reachable from itself in the recipe
// graph adj (product → ingredient products of its active card). Used to
// reject a recipe that would close a cycle (§4). DFS, O(V+E).
func ReachesSelf(start sharedkernel.ID, adj map[sharedkernel.ID][]sharedkernel.ID) bool {
	visited := map[sharedkernel.ID]bool{}
	var dfs func(n sharedkernel.ID) bool
	dfs = func(n sharedkernel.ID) bool {
		for _, next := range adj[n] {
			if next == start {
				return true
			}
			if !visited[next] {
				visited[next] = true
				if dfs(next) {
					return true
				}
			}
		}
		return false
	}
	return dfs(start)
}

// LineCost values one recipe line: qty (milli) × unitCostPerBase (cents per
// base unit), banker-rounded to cents.
func LineCost(qtyMilli, unitCostPerBase int64) int64 {
	return bankRound(qtyMilli*unitCostPerBase, MilliPerUnit)
}

// RecipeCost is Σ of the line costs given each ingredient's cost per base
// unit (missing ingredient costs count as 0).
func RecipeCost(lines []TechCardLine, unitCostPerBase map[sharedkernel.ID]int64) int64 {
	var total int64
	for _, l := range lines {
		total += LineCost(l.Qty, unitCostPerBase[l.IngredientProductID])
	}
	return total
}

// UnitCostFromRecipe turns a prepared/dish version's total recipe cost into
// a cost per base unit of its yield (for use as an ingredient elsewhere).
func UnitCostFromRecipe(recipeCostCents, yieldMilli int64) int64 {
	if yieldMilli <= 0 {
		return 0
	}
	return bankRound(recipeCostCents*MilliPerUnit, yieldMilli)
}
