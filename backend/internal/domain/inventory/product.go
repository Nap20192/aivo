// Package domain holds the core types for the inventory context:
// nomenclature (products + units), calendar-versioned tech cards with an
// append-only cost series, a perpetual weighted-average stock ledger, and
// the stock documents (receipt / write-off / stocktake). It imports only
// sharedkernel and the standard library. Money is integer cents;
// quantities are int64 milli-units of a product's base unit (the "cents
// analog", domain.md §0) — single currency, multicurrency deferred
// (reference §16.4).
package domain

import (
	"errors"
	"math"
	"time"

	"aivo/internal/sharedkernel"
)

// Product types (closed enum, Domain 3).
const (
	TypeGoods    = "goods"    // raw material
	TypeDish     = "dish"     // sold item, linked to a menu_item
	TypePrepared = "prepared" // in-house semi-product (has its own tech card + stock)
	TypeModifier = "modifier" // add-on (syrup, extra cheese)
)

// Units. Base units (in which stock is kept) are g, ml, pcs; kg and l are
// compatible display units.
const (
	UnitG   = "g"
	UnitKg  = "kg"
	UnitMl  = "ml"
	UnitL   = "l"
	UnitPcs = "pcs"
)

// MilliPerUnit is the milli-unit scale: 1 base unit = 1000 milli-units.
const MilliPerUnit = 1000

var (
	ErrInvalidType       = errors.New("inventory: invalid product type")
	ErrInvalidUnit       = errors.New("inventory: invalid unit")
	ErrNonBaseStockUnit  = errors.New("inventory: stock_unit must be a base unit (g, ml, pcs)")
	ErrUnitIncompatible  = errors.New("inventory: unit incompatible with product stock unit")
	ErrInvalidQty        = errors.New("inventory: quantity must be > 0")
	ErrMenuItemOnNonDish = errors.New("inventory: menu_item_id is only for a dish product")
)

type unitInfo struct {
	dimension string // mass | volume | count
	factor    int64  // multiples of the base unit
}

var units = map[string]unitInfo{
	UnitG:   {"mass", 1},
	UnitKg:  {"mass", 1000},
	UnitMl:  {"volume", 1},
	UnitL:   {"volume", 1000},
	UnitPcs: {"count", 1},
}

// ValidUnit reports whether u is a known unit.
func ValidUnit(u string) bool { _, ok := units[u]; return ok }

// ValidBaseUnit reports whether u is a base (factor-1) unit.
func ValidBaseUnit(u string) bool {
	i, ok := units[u]
	return ok && i.factor == 1
}

// ValidType reports whether t is a known product type.
func ValidType(t string) bool {
	return t == TypeGoods || t == TypeDish || t == TypePrepared || t == TypeModifier
}

// ToBaseMilli converts a display quantity (a decimal number in inputUnit)
// into int64 milli-units of stockUnit. inputUnit must share stockUnit's
// dimension (else ErrUnitIncompatible). Banker's rounding to the nearest
// milli-unit.
func ToBaseMilli(qtyInput float64, inputUnit, stockUnit string) (int64, error) {
	in, ok := units[inputUnit]
	if !ok {
		return 0, ErrInvalidUnit
	}
	base, ok := units[stockUnit]
	if !ok {
		return 0, ErrInvalidUnit
	}
	if in.dimension != base.dimension {
		return 0, ErrUnitIncompatible
	}
	if qtyInput <= 0 {
		return 0, ErrInvalidQty
	}
	// qty in base units × 1000 milli/base. factor already normalizes to the
	// base unit (kg→g = ×1000); stockUnit is a base unit so base.factor == 1.
	milli := qtyInput * float64(in.factor) * MilliPerUnit
	return int64(math.RoundToEven(milli)), nil
}

// FromBaseMilli renders milli-units back as a base-unit decimal number.
func FromBaseMilli(milli int64) float64 { return float64(milli) / MilliPerUnit }

// Product is a nomenclature entry (aggregate root).
type Product struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	SKU          string
	Name         string
	Type         string           // goods|dish|prepared|modifier
	StockUnit    string           // g|ml|pcs (base unit stock is kept in)
	MenuItemID   *sharedkernel.ID // only for dish; bare uuid, no FK
	MinStock     *int64           // milli-units, for low-stock alerts
	Archived     bool
	CreatedAt    time.Time
}

// NewProduct validates and constructs a product. menuItemID is allowed
// only for a dish.
func NewProduct(id, restaurantID sharedkernel.ID, sku, name, ptype, stockUnit string, menuItemID *sharedkernel.ID, minStock *int64) (Product, error) {
	if !ValidType(ptype) {
		return Product{}, ErrInvalidType
	}
	if !ValidBaseUnit(stockUnit) {
		return Product{}, ErrNonBaseStockUnit
	}
	if menuItemID != nil && ptype != TypeDish {
		return Product{}, ErrMenuItemOnNonDish
	}
	return Product{
		ID: id, RestaurantID: restaurantID, SKU: sku, Name: name,
		Type: ptype, StockUnit: stockUnit, MenuItemID: menuItemID, MinStock: minStock,
	}, nil
}
