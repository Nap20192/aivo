package domain

import (
	"errors"
	"time"

	"aivo/internal/sharedkernel"
)

// Stock-document lifecycle (D4): draft → posted → cancelled(=reversal).
const (
	DocDraft     = "draft"
	DocPosted    = "posted"
	DocCancelled = "cancelled"
)

// Document kinds (stock_moves.doc_kind + document tables).
const (
	DocKindReceipt   = "goods_receipt"
	DocKindWriteoff  = "write_off"
	DocKindStocktake = "stocktake"
)

// GL source_kind per document kind (ledger journal source_kind, §2).
const (
	SourceReceipt   = "inventory_receipt"
	SourceWriteoff  = "inventory_writeoff"
	SourceStocktake = "inventory_stocktake"
	SourceCOGS      = "cogs"
)

// Write-off reasons.
const (
	ReasonSpoilage  = "spoilage"
	ReasonExpiry    = "expiry"
	ReasonStaffMeal = "staff_meal"
	ReasonLoss      = "loss"
	ReasonOther     = "other"
)

var (
	ErrEmptyDocument    = errors.New("inventory: document needs at least one line")
	ErrAlreadyPosted    = errors.New("inventory: document already posted")
	ErrNotPosted        = errors.New("inventory: document is not posted")
	ErrAlreadyCancelled = errors.New("inventory: document already cancelled")
	ErrInvalidReason    = errors.New("inventory: invalid write-off reason")
)

// ValidReason reports whether r is a known write-off reason.
func ValidReason(r string) bool {
	switch r {
	case ReasonSpoilage, ReasonExpiry, ReasonStaffMeal, ReasonLoss, ReasonOther:
		return true
	}
	return false
}

// Supplier is a minimal per-restaurant supplier reference (no price lists /
// orders — anti-scope §6.1).
type Supplier struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Name         string
	Contacts     map[string]string
	Note         string
	Archived     bool
	CreatedAt    time.Time
}

// GoodsReceipt is an inbound-stock document (aggregate root + lines).
type GoodsReceipt struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	SupplierID   *sharedkernel.ID
	Status       string
	BusinessDate time.Time
	Note         string
	PostedAt     *time.Time
	PostedBy     *sharedkernel.ID
	ReversalOf   *sharedkernel.ID
	CreatedAt    time.Time
	Lines        []GoodsReceiptLine
}

// GoodsReceiptLine carries the input quantity/unit and price, plus the
// derived base quantity and line cost (both stamped at draft time).
type GoodsReceiptLine struct {
	ID             sharedkernel.ID
	ReceiptID      sharedkernel.ID
	ProductID      sharedkernel.ID
	QtyBaseMilli   int64  // converted to the product's base unit
	InputUnit      string // the unit the user entered (for display)
	UnitPriceCents int64  // price per input unit
	LineCostCents  int64  // round(unit_price × qty_input)
	Seq            int
}

// TotalCents sums the receipt's line costs.
func (r GoodsReceipt) TotalCents() int64 {
	var t int64
	for _, l := range r.Lines {
		t += l.LineCostCents
	}
	return t
}

// WriteOff removes stock at current average cost (spoilage/loss/etc).
type WriteOff struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Reason       string
	Status       string
	BusinessDate time.Time
	Note         string
	PostedAt     *time.Time
	PostedBy     *sharedkernel.ID
	ReversalOf   *sharedkernel.ID
	CreatedAt    time.Time
	Lines        []WriteOffLine
}

type WriteOffLine struct {
	ID           sharedkernel.ID
	WriteOffID   sharedkernel.ID
	ProductID    sharedkernel.ID
	QtyBaseMilli int64
	InputUnit    string
	Seq          int
}

// Stocktake is a server-computed physical count (D2 §4). expected_qty is
// fixed at post, not at creation, so sales during the count don't create a
// phantom shortage.
type Stocktake struct {
	ID           sharedkernel.ID
	RestaurantID sharedkernel.ID
	Status       string
	BusinessDate time.Time
	Note         string
	PostedAt     *time.Time
	PostedBy     *sharedkernel.ID
	ReversalOf   *sharedkernel.ID
	CreatedAt    time.Time
	Lines        []StocktakeLine
}

type StocktakeLine struct {
	ID                sharedkernel.ID
	StocktakeID       sharedkernel.ID
	ProductID         sharedkernel.ID
	CountedQtyMilli   int64
	ExpectedQtyMilli  *int64 // fixed at post
	VarianceQtyMilli  int64  // counted − expected
	VarianceCostCents int64
	Seq               int
}
