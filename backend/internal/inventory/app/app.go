// Package app is the inventory context's use-case layer: nomenclature,
// calendar-versioned tech cards + costing, the perpetual weighted-average
// stock ledger, stock documents (receipt / write-off / stocktake) that post
// to the GL through the ledger bridge, COGS on sale, and the food-cost
// report. Chosen contexts import inward only; the ledger is reached solely
// through ports.Ledger (a bridge), never a direct ledger/app import.
package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"
	"aivo/internal/sharedkernel"

	"uuid"
)

var (
	// ErrInvalid marks caller-fixable input (422).
	ErrInvalid          = errors.New("inventory: invalid input")
	ErrSKUTaken         = errors.New("inventory: sku already used")
	ErrMenuItemTaken    = errors.New("inventory: menu item already has a dish")
	ErrVersionExists    = errors.New("inventory: a tech card version already starts on this date")
	ErrBackdated        = errors.New("inventory: business_date is before the last posted move for a product")
	ErrStocktakeOpen    = errors.New("inventory: an open stocktake already exists")
	ErrAlreadyPosted    = errors.New("inventory: document already posted")
	ErrAlreadyCancelled = errors.New("inventory: document already cancelled")
	ErrNotDraft         = errors.New("inventory: document is not a draft")
	ErrNotPosted        = errors.New("inventory: document is not posted")
)

type App struct {
	store  ports.Store
	ledger ports.Ledger
	sales  ports.SalesReader
	newID  func() sharedkernel.ID
	now    func() time.Time
}

func New(store ports.Store, ledger ports.Ledger, sales ports.SalesReader) *App {
	return &App{store: store, ledger: ledger, sales: sales,
		newID: sharedkernel.NewID, now: func() time.Time { return time.Now().UTC() }}
}

// Now returns the app's current date/time (UTC) — the default "active on"
// date for tech-card reads.
func (a *App) Now() time.Time { return a.now() }

// ParseDate parses a YYYY-MM-DD business date into a UTC date.
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: business_date must be YYYY-MM-DD", ErrInvalid)
	}
	return t, nil
}

// deriveEventID is a deterministic id for a sale move, so a re-run of the
// same ticket line + ingredient cannot double-deplete (COGS idempotency,
// §7.3). sha256 of the two ids, first 16 bytes.
func deriveEventID(a, b sharedkernel.ID) sharedkernel.ID {
	h := sha256.Sum256(append(append([]byte{}, a[:]...), b[:]...))
	var id sharedkernel.ID
	copy(id[:], h[:16])
	return id
}

// --- products ----------------------------------------------------------

// ProductInput is a new-product request (units validated in the domain).
type ProductInput struct {
	SKU        string
	Name       string
	Type       string
	StockUnit  string
	MenuItemID *uuid.UUID
	MinStock   *int64
}

func (a *App) CreateProduct(ctx context.Context, restaurantID uuid.UUID, in ProductInput) (inv.Product, error) {
	p, err := inv.NewProduct(a.newID(), restaurantID, in.SKU, in.Name, in.Type, in.StockUnit, in.MenuItemID, in.MinStock)
	if err != nil {
		return inv.Product{}, err // domain validation error, mapped by HTTP
	}
	if in.MenuItemID != nil {
		if err := a.checkMenuItemFree(ctx, restaurantID, *in.MenuItemID); err != nil {
			return inv.Product{}, err
		}
	}
	if err := a.store.InsertProduct(ctx, p); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return inv.Product{}, ErrSKUTaken
		}
		return inv.Product{}, err
	}
	return p, nil
}

func (a *App) checkMenuItemFree(ctx context.Context, restaurantID, menuItemID uuid.UUID) error {
	_, err := a.store.ProductByMenuItem(ctx, restaurantID, menuItemID)
	if err == nil {
		return ErrMenuItemTaken
	}
	if !errors.Is(err, ports.ErrNotFound) {
		return err
	}
	return nil
}

func (a *App) Products(ctx context.Context, restaurantID uuid.UUID) ([]inv.Product, error) {
	return a.store.Products(ctx, restaurantID)
}

// Product returns a product with its current on-hand position.
func (a *App) Product(ctx context.Context, restaurantID, id uuid.UUID) (inv.Product, inv.OnHand, error) {
	p, err := a.store.ProductByID(ctx, restaurantID, id)
	if err != nil {
		return inv.Product{}, inv.OnHand{}, err
	}
	oh, err := a.store.OnHand(ctx, restaurantID, id)
	if err != nil {
		return inv.Product{}, inv.OnHand{}, err
	}
	return p, oh, nil
}

// ProductPatch is a partial product update (stock_unit is not editable —
// it is locked by the first move).
type ProductPatch struct {
	Name       *string
	MinStock   *int64
	ClearMin   bool // set MinStock to NULL
	MenuItemID *uuid.UUID
	ClearMenu  bool
	Archived   *bool
}

func (a *App) UpdateProduct(ctx context.Context, restaurantID, id uuid.UUID, patch ProductPatch) (inv.Product, error) {
	p, err := a.store.ProductByID(ctx, restaurantID, id)
	if err != nil {
		return inv.Product{}, err
	}
	if patch.Name != nil {
		p.Name = *patch.Name
	}
	if patch.ClearMin {
		p.MinStock = nil
	} else if patch.MinStock != nil {
		p.MinStock = patch.MinStock
	}
	if patch.ClearMenu {
		p.MenuItemID = nil
	} else if patch.MenuItemID != nil {
		if p.Type != inv.TypeDish {
			return inv.Product{}, inv.ErrMenuItemOnNonDish
		}
		if p.MenuItemID == nil || *p.MenuItemID != *patch.MenuItemID {
			if err := a.checkMenuItemFree(ctx, restaurantID, *patch.MenuItemID); err != nil {
				return inv.Product{}, err
			}
		}
		p.MenuItemID = patch.MenuItemID
	}
	if patch.Archived != nil {
		p.Archived = *patch.Archived
	}
	if err := a.store.UpdateProduct(ctx, p); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return inv.Product{}, ErrMenuItemTaken
		}
		return inv.Product{}, err
	}
	return p, nil
}

// --- stock reads -------------------------------------------------------

// OnHandRow pairs a product with its position (on-hand report).
type OnHandRow struct {
	Product  inv.Product
	OnHand   inv.OnHand
	BelowMin bool
}

func (a *App) OnHand(ctx context.Context, restaurantID uuid.UUID, lowStockOnly bool) ([]OnHandRow, error) {
	products, err := a.store.Products(ctx, restaurantID)
	if err != nil {
		return nil, err
	}
	positions, err := a.store.OnHandAll(ctx, restaurantID)
	if err != nil {
		return nil, err
	}
	byProduct := map[uuid.UUID]inv.OnHand{}
	for _, o := range positions {
		byProduct[o.ProductID] = o
	}
	out := []OnHandRow{}
	for _, p := range products {
		if p.Archived {
			continue
		}
		oh := byProduct[p.ID]
		oh.RestaurantID, oh.ProductID = restaurantID, p.ID
		below := p.MinStock != nil && oh.QtyMilli < *p.MinStock
		if lowStockOnly && !below {
			continue
		}
		out = append(out, OnHandRow{Product: p, OnHand: oh, BelowMin: below})
	}
	return out, nil
}

func (a *App) StockMoves(ctx context.Context, restaurantID uuid.UUID, from string, product *uuid.UUID) ([]inv.StockMove, error) {
	return a.store.StockMoves(ctx, restaurantID, from, product)
}

// roundToEven rounds a float to the nearest int64 (half-to-even).
func roundToEven(f float64) int64 { return int64(math.RoundToEven(f)) }
