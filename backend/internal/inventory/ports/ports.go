// Package ports defines the inventory context's boundaries: persistence
// (Store), the synchronous port into the ledger context (Ledger, via a
// bridge adapter), and a read-only view of pos sales (SalesReader) for the
// food-cost report. Go interfaces, not gRPC (ADR 0001).
package ports

import (
	"context"
	"database/sql"
	"errors"
	"time"

	inv "aivo/internal/domain/inventory"

	"uuid"
)

var (
	ErrNotFound = errors.New("inventory: not found")
	ErrConflict = errors.New("inventory: conflict")
)

// Store is inventory persistence, scoped by restaurantID in the query.
type Store interface {
	// WithTx returns a Store bound to tx; InTx runs fn in one transaction.
	WithTx(tx *sql.Tx) Store
	InTx(ctx context.Context, fn func(tx *sql.Tx, s Store) error) error

	// --- products ---
	InsertProduct(ctx context.Context, p inv.Product) error
	Products(ctx context.Context, restaurantID uuid.UUID) ([]inv.Product, error)
	ProductByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Product, error)
	ProductByMenuItem(ctx context.Context, restaurantID, menuItemID uuid.UUID) (inv.Product, error)
	ProductHasMoves(ctx context.Context, productID uuid.UUID) (bool, error)
	UpdateProduct(ctx context.Context, p inv.Product) error

	// --- tech cards ---
	InsertTechCard(ctx context.Context, tc inv.TechCard) error
	CloseTechCard(ctx context.Context, id uuid.UUID, validTo time.Time) error
	TechCardsByProduct(ctx context.Context, restaurantID, productID uuid.UUID) ([]inv.TechCard, error)
	TechCardByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.TechCard, error)
	// ActiveTechCard returns the version active on the date, ErrNotFound if none.
	ActiveTechCard(ctx context.Context, restaurantID, productID uuid.UUID, on time.Time) (inv.TechCard, error)
	// ActiveRecipeGraph returns product → its active-card ingredient products,
	// for every product with an active card on the date (cycle check §4).
	ActiveRecipeGraph(ctx context.Context, restaurantID uuid.UUID, on time.Time) (map[uuid.UUID][]uuid.UUID, error)
	InsertCosting(ctx context.Context, c inv.RecipeCosting) error
	Costings(ctx context.Context, techCardID uuid.UUID) ([]inv.RecipeCosting, error)
	LatestCostCents(ctx context.Context, techCardID uuid.UUID) (int64, bool, error)

	// --- stock ---
	InsertStockMove(ctx context.Context, m inv.StockMove) error
	// LockOnHand ensures the row exists then locks it FOR UPDATE, returning
	// the current position (zero for a new product).
	LockOnHand(ctx context.Context, restaurantID, productID uuid.UUID) (inv.OnHand, error)
	SaveOnHand(ctx context.Context, o inv.OnHand) error
	OnHand(ctx context.Context, restaurantID, productID uuid.UUID) (inv.OnHand, error)
	OnHandAll(ctx context.Context, restaurantID uuid.UUID) ([]inv.OnHand, error)
	// MaxMoveDate is the latest business_date of any move for the product
	// (backdate guard §5.4); zero time if none.
	MaxMoveDate(ctx context.Context, restaurantID, productID uuid.UUID) (time.Time, bool, error)
	StockMoves(ctx context.Context, restaurantID uuid.UUID, from string, product *uuid.UUID) ([]inv.StockMove, error)
	// MovesByDoc returns the (non-reversal) moves a document produced, to
	// mirror them on cancel.
	MovesByDoc(ctx context.Context, restaurantID uuid.UUID, docKind string, docID uuid.UUID) ([]inv.StockMove, error)
	// MoveExistsBySourceEvent reports whether a sale move already exists for
	// the event id (COGS idempotency second belt, §7.3).
	MoveExistsBySourceEvent(ctx context.Context, sourceEventID uuid.UUID) (bool, error)
	// SaleCostByProduct sums |cost| of sale moves in [from,to] per product
	// (food-cost actual).
	SaleCostByProduct(ctx context.Context, restaurantID uuid.UUID, from, to string) (map[uuid.UUID]int64, error)
	SaleEstimatedShare(ctx context.Context, restaurantID uuid.UUID, from, to string) (total, estimated int, err error)

	// --- suppliers ---
	InsertSupplier(ctx context.Context, s inv.Supplier) error
	Suppliers(ctx context.Context, restaurantID uuid.UUID) ([]inv.Supplier, error)
	SupplierByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Supplier, error)
	UpdateSupplier(ctx context.Context, s inv.Supplier) error

	// --- documents ---
	// LockDocument locks a stock document row FOR UPDATE and returns its
	// status, serializing concurrent post/cancel. docKind ∈ goods_receipt|
	// write_off|stocktake.
	LockDocument(ctx context.Context, docKind string, restaurantID, id uuid.UUID) (string, error)

	// --- goods receipts ---
	InsertReceipt(ctx context.Context, r inv.GoodsReceipt) error
	ReceiptByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.GoodsReceipt, error)
	Receipts(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.GoodsReceipt, error)
	// MarkReceiptStatus flips draft→posted (posted_at/by) or →cancelled
	// (reversal_of), guarded by the current status; ErrConflict on mismatch.
	MarkReceiptStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error

	// --- write-offs ---
	InsertWriteOff(ctx context.Context, w inv.WriteOff) error
	WriteOffByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.WriteOff, error)
	WriteOffs(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.WriteOff, error)
	MarkWriteOffStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error

	// --- stocktakes ---
	InsertStocktake(ctx context.Context, s inv.Stocktake) error
	StocktakeByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Stocktake, error)
	Stocktakes(ctx context.Context, restaurantID uuid.UUID, status string) ([]inv.Stocktake, error)
	ReplaceStocktakeLines(ctx context.Context, stocktakeID uuid.UUID, lines []inv.StocktakeLine) error
	MarkStocktakeStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error
}

// JournalLine is one line of an inventory GL document, by purpose.
type JournalLine struct {
	Purpose     string
	Side        string
	AmountCents int64
}

// Ledger is the synchronous port into the ledger context. Posting happens
// inside the inventory transaction (shared *sql.Tx); correction is a
// reversal, never an edit (append-only).
type Ledger interface {
	PostInventoryJournal(ctx context.Context, tx *sql.Tx, restaurantID, createdBy uuid.UUID, sourceKind string, sourceID uuid.UUID, accountingDate time.Time, lines []JournalLine) (docID uuid.UUID, err error)
	CancelJournalForSource(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID, sourceKind string, sourceID uuid.UUID) (reversalID uuid.UUID, err error)
}

// SaleQty is a sold dish and its quantity over a period (food-cost report).
type SaleQty struct {
	MenuItemID   uuid.UUID
	Name         string
	QtyMilli     int64 // count of items × 1000
	RevenueCents int64
}

// SalesReader reads pos sales for the food-cost report (a port over posdb —
// no raw SQL against pos tables from inventory, ddd §4).
type SalesReader interface {
	SoldDishes(ctx context.Context, restaurantID uuid.UUID, from, to string) ([]SaleQty, error)
}
