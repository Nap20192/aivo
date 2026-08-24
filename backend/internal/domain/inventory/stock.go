package domain

import (
	"time"

	"aivo/internal/sharedkernel"
)

// Stock move kinds (D2 append-only book). qty is signed: receipts and
// surplus are positive, issues (sale/write-off/shortage) negative.
const (
	MoveReceipt           = "receipt"
	MoveSale              = "sale"
	MoveWriteoff          = "writeoff"
	MoveStocktakeSurplus  = "stocktake_surplus"
	MoveStocktakeShortage = "stocktake_shortage"
	MoveReversal          = "reversal"
)

// StockMove is one append-only entry in the perpetual stock ledger. It is
// never edited or deleted (D1/D2) — a correction is a mirror reversal move.
// cost_cents is stamped at the moment of the move, so food cost needs no
// retroactive recompute (self-contained book). Two dates (D7).
type StockMove struct {
	ID            sharedkernel.ID
	RestaurantID  sharedkernel.ID
	ProductID     sharedkernel.ID
	Kind          string
	QtyMilli      int64 // signed
	CostCents     int64 // signed
	Estimated     bool  // cost estimated (issue against non-positive stock)
	BusinessDate  time.Time
	RecordedAt    time.Time
	DocKind       string
	DocID         sharedkernel.ID
	SourceEventID *sharedkernel.ID // nullable, UNIQUE — sale idempotency
	CreatedAt     time.Time
}

// OnHand is the materialized weighted-average position for one product (a
// rewritable cache; source of truth is the fold of StockMove). qty and
// value must equal Σ moves.qty and Σ moves.cost (invariant test).
type OnHand struct {
	RestaurantID sharedkernel.ID
	ProductID    sharedkernel.ID
	QtyMilli     int64 // signed
	ValueCents   int64 // value of stock on hand
	LastAvgCents int64 // avg per base unit at last positive qty (estimation)
	UpdatedAt    time.Time
}

// CostOfMilli values qtyMilli (>0) base-milli-units at the current
// valuation, WITHOUT mutating. When on hand fully covers it, cost is the
// exact proportional share (value × qty / onHand); otherwise the covered
// part takes all remaining value and the deficit is estimated at
// LastAvgCents — and estimated is true (a sale is never blocked by stock,
// Domain 5).
func (o OnHand) CostOfMilli(qtyMilli int64) (costCents int64, estimated bool) {
	if o.QtyMilli >= qtyMilli && o.QtyMilli > 0 {
		return bankRound(o.ValueCents*qtyMilli, o.QtyMilli), false
	}
	avail := o.QtyMilli
	if avail < 0 {
		avail = 0
	}
	var costAvail int64
	if avail > 0 {
		costAvail = o.ValueCents // depleting everything on hand
	}
	deficit := qtyMilli - avail
	costDeficit := bankRound(o.LastAvgCents*deficit, MilliPerUnit)
	return costAvail + costDeficit, true
}

// ApplyMove folds a signed (qty, cost) delta into the position and
// refreshes LastAvgCents while qty stays positive. Receipts pass (+q, +c);
// issues pass (−q, −cost) with cost from CostOfMilli; a reversal passes the
// negated original signed (qty, cost) — an exact undo that never distorts
// the average.
func (o *OnHand) ApplyMove(qtyMilli, costCents int64) {
	o.QtyMilli += qtyMilli
	o.ValueCents += costCents
	if o.QtyMilli > 0 {
		o.LastAvgCents = bankRound(o.ValueCents*MilliPerUnit, o.QtyMilli)
	}
}

// AvgCentsPerBase is the current weighted-average cost per base unit
// (value / (qty/1000)); 0 when nothing is on hand.
func (o OnHand) AvgCentsPerBase() int64 {
	if o.QtyMilli <= 0 {
		return o.LastAvgCents
	}
	return bankRound(o.ValueCents*MilliPerUnit, o.QtyMilli)
}

// bankRound divides num by den (den ≠ 0) with round-half-to-even.
func bankRound(num, den int64) int64 {
	if den == 0 {
		return 0
	}
	neg := (num < 0) != (den < 0)
	a, d := num, den
	if a < 0 {
		a = -a
	}
	if d < 0 {
		d = -d
	}
	q, r := a/d, a%d
	twice := 2 * r
	if twice > d || (twice == d && q%2 == 1) {
		q++
	}
	if neg {
		q = -q
	}
	return q
}
