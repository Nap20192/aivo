package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// --- shared move mechanics ---------------------------------------------

// applyReceiptMove records an inbound move (+qty, +cost) and folds it into
// on-hand, all on the tx-bound store.
func (a *App) applyReceiptMove(ctx context.Context, st ports.Store, restaurantID, productID uuid.UUID, qtyMilli, costCents int64, businessDate time.Time, docKind string, docID uuid.UUID) error {
	oh, err := st.LockOnHand(ctx, restaurantID, productID)
	if err != nil {
		return err
	}
	oh.ApplyMove(qtyMilli, costCents)
	move := inv.StockMove{
		ID: a.newID(), RestaurantID: restaurantID, ProductID: productID, Kind: inv.MoveReceipt,
		QtyMilli: qtyMilli, CostCents: costCents, BusinessDate: businessDate, DocKind: docKind, DocID: docID,
	}
	if err := st.InsertStockMove(ctx, move); err != nil {
		return err
	}
	return st.SaveOnHand(ctx, oh)
}

// applyIssueMove records an outbound move of magnitude qtyMilli at the
// current average and returns its cost; estimated when stock was short.
func (a *App) applyIssueMove(ctx context.Context, st ports.Store, restaurantID, productID uuid.UUID, kind string, qtyMilli int64, businessDate time.Time, docKind string, docID uuid.UUID, sourceEventID *uuid.UUID) (int64, bool, error) {
	oh, err := st.LockOnHand(ctx, restaurantID, productID)
	if err != nil {
		return 0, false, err
	}
	cost, estimated := oh.CostOfMilli(qtyMilli)
	oh.ApplyMove(-qtyMilli, -cost)
	move := inv.StockMove{
		ID: a.newID(), RestaurantID: restaurantID, ProductID: productID, Kind: kind,
		QtyMilli: -qtyMilli, CostCents: -cost, Estimated: estimated, BusinessDate: businessDate,
		DocKind: docKind, DocID: docID, SourceEventID: sourceEventID,
	}
	if err := st.InsertStockMove(ctx, move); err != nil {
		return 0, false, err
	}
	if err := st.SaveOnHand(ctx, oh); err != nil {
		return 0, false, err
	}
	return cost, estimated, nil
}

// applySurplusMove records an inbound stocktake surplus valued at the
// current average, returning its cost.
func (a *App) applySurplusMove(ctx context.Context, st ports.Store, restaurantID, productID uuid.UUID, qtyMilli int64, businessDate time.Time, docID uuid.UUID) (int64, error) {
	oh, err := st.LockOnHand(ctx, restaurantID, productID)
	if err != nil {
		return 0, err
	}
	cost, _ := oh.CostOfMilli(qtyMilli)
	oh.ApplyMove(qtyMilli, cost)
	move := inv.StockMove{
		ID: a.newID(), RestaurantID: restaurantID, ProductID: productID, Kind: inv.MoveStocktakeSurplus,
		QtyMilli: qtyMilli, CostCents: cost, BusinessDate: businessDate, DocKind: inv.DocKindStocktake, DocID: docID,
	}
	if err := st.InsertStockMove(ctx, move); err != nil {
		return 0, err
	}
	return cost, st.SaveOnHand(ctx, oh)
}

// reverseMoves mirrors a document's moves (by their original qty/cost, not
// current average) onto a reversal document and rolls on-hand back exactly.
func (a *App) reverseMoves(ctx context.Context, st ports.Store, restaurantID uuid.UUID, docKind string, origID, reversalID uuid.UUID, businessDate time.Time) error {
	moves, err := st.MovesByDoc(ctx, restaurantID, docKind, origID)
	if err != nil {
		return err
	}
	for _, m := range moves {
		oh, err := st.LockOnHand(ctx, restaurantID, m.ProductID)
		if err != nil {
			return err
		}
		oh.ApplyMove(-m.QtyMilli, -m.CostCents)
		rev := inv.StockMove{
			ID: a.newID(), RestaurantID: restaurantID, ProductID: m.ProductID, Kind: inv.MoveReversal,
			QtyMilli: -m.QtyMilli, CostCents: -m.CostCents, BusinessDate: businessDate, DocKind: docKind, DocID: reversalID,
		}
		if err := st.InsertStockMove(ctx, rev); err != nil {
			return err
		}
		if err := st.SaveOnHand(ctx, oh); err != nil {
			return err
		}
	}
	return nil
}

// checkBackdate rejects a document whose business_date precedes the last
// posted move of any of its products (§5.4).
func (a *App) checkBackdate(ctx context.Context, st ports.Store, restaurantID uuid.UUID, businessDate time.Time, productIDs []uuid.UUID) error {
	for _, pid := range productIDs {
		last, ok, err := st.MaxMoveDate(ctx, restaurantID, pid)
		if err != nil {
			return err
		}
		if ok && businessDate.Before(last) {
			return fmt.Errorf("%w: product %s (last move %s)", ErrBackdated, pid, last.Format("2006-01-02"))
		}
	}
	return nil
}

// docStatusErr maps a non-draft/non-posted status to the right app error.
func postGuard(status string) error {
	switch status {
	case inv.DocPosted:
		return ErrAlreadyPosted
	case inv.DocCancelled:
		return ErrAlreadyCancelled
	case inv.DocDraft:
		return nil
	}
	return ErrNotDraft
}

func cancelGuard(status string) error {
	switch status {
	case inv.DocPosted:
		return nil
	case inv.DocCancelled:
		return ErrAlreadyCancelled
	}
	return ErrNotPosted
}

// --- goods receipts ----------------------------------------------------

// ReceiptLineInput is one receipt line as entered.
type ReceiptLineInput struct {
	ProductID      uuid.UUID
	QtyInput       float64
	Unit           string
	UnitPriceCents int64
}

func (a *App) CreateReceipt(ctx context.Context, restaurantID uuid.UUID, supplierID *uuid.UUID, businessDate time.Time, note string, lineInputs []ReceiptLineInput, createdBy uuid.UUID) (inv.GoodsReceipt, error) {
	if len(lineInputs) == 0 {
		return inv.GoodsReceipt{}, inv.ErrEmptyDocument
	}
	r := inv.GoodsReceipt{
		ID: a.newID(), RestaurantID: restaurantID, SupplierID: supplierID,
		Status: inv.DocDraft, BusinessDate: businessDate, Note: note,
	}
	for i, li := range lineInputs {
		p, err := a.store.ProductByID(ctx, restaurantID, li.ProductID)
		if err != nil {
			return inv.GoodsReceipt{}, err
		}
		qtyBase, err := inv.ToBaseMilli(li.QtyInput, li.Unit, p.StockUnit)
		if err != nil {
			return inv.GoodsReceipt{}, err
		}
		if li.UnitPriceCents < 0 {
			return inv.GoodsReceipt{}, fmt.Errorf("%w: unit_price must be >= 0", ErrInvalid)
		}
		r.Lines = append(r.Lines, inv.GoodsReceiptLine{
			ID: a.newID(), ReceiptID: r.ID, ProductID: li.ProductID, QtyBaseMilli: qtyBase,
			InputUnit: li.Unit, UnitPriceCents: li.UnitPriceCents,
			LineCostCents: roundToEven(float64(li.UnitPriceCents) * li.QtyInput), Seq: i + 1,
		})
	}
	if err := a.store.InsertReceipt(ctx, r); err != nil {
		return inv.GoodsReceipt{}, err
	}
	return r, nil
}

func (a *App) Receipt(ctx context.Context, restaurantID, id uuid.UUID) (inv.GoodsReceipt, error) {
	return a.store.ReceiptByID(ctx, restaurantID, id)
}

func (a *App) Receipts(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.GoodsReceipt, error) {
	return a.store.Receipts(ctx, restaurantID, from, status)
}

func (a *App) PostReceipt(ctx context.Context, restaurantID, receiptID, postedBy uuid.UUID) (inv.GoodsReceipt, error) {
	err := a.store.InTx(ctx, func(tx *sql.Tx, st ports.Store) error {
		status, err := st.LockDocument(ctx, inv.DocKindReceipt, restaurantID, receiptID)
		if err != nil {
			return err
		}
		if err := postGuard(status); err != nil {
			return err
		}
		r, err := st.ReceiptByID(ctx, restaurantID, receiptID)
		if err != nil {
			return err
		}
		pids := make([]uuid.UUID, len(r.Lines))
		for i, l := range r.Lines {
			pids[i] = l.ProductID
		}
		if err := a.checkBackdate(ctx, st, restaurantID, r.BusinessDate, pids); err != nil {
			return err
		}
		for _, l := range r.Lines {
			if err := a.applyReceiptMove(ctx, st, restaurantID, l.ProductID, l.QtyBaseMilli, l.LineCostCents, r.BusinessDate, inv.DocKindReceipt, receiptID); err != nil {
				return err
			}
		}
		total := r.TotalCents()
		if _, err := a.ledger.PostInventoryJournal(ctx, tx, restaurantID, postedBy, inv.SourceReceipt, receiptID, r.BusinessDate, []ports.JournalLine{
			{Purpose: "inventory", Side: "debit", AmountCents: total},
			{Purpose: "accounts_payable", Side: "credit", AmountCents: total},
		}); err != nil {
			return err
		}
		return st.MarkReceiptStatus(ctx, restaurantID, receiptID, inv.DocDraft, inv.DocPosted, &postedBy)
	})
	if err != nil {
		return inv.GoodsReceipt{}, err
	}
	return a.store.ReceiptByID(ctx, restaurantID, receiptID)
}

func (a *App) CancelReceipt(ctx context.Context, restaurantID, receiptID uuid.UUID) (inv.GoodsReceipt, error) {
	err := a.cancelDocument(ctx, restaurantID, receiptID, inv.DocKindReceipt, inv.SourceReceipt,
		func(ctx context.Context, st ports.Store, reversalID uuid.UUID, businessDate time.Time) error {
			orig, err := st.ReceiptByID(ctx, restaurantID, receiptID)
			if err != nil {
				return err
			}
			return st.InsertReceipt(ctx, inv.GoodsReceipt{
				ID: reversalID, RestaurantID: restaurantID, SupplierID: orig.SupplierID,
				Status: inv.DocPosted, BusinessDate: businessDate, Note: "reversal", ReversalOf: &receiptID,
			})
		},
		func(ctx context.Context, st ports.Store, from, to string) error {
			return st.MarkReceiptStatus(ctx, restaurantID, receiptID, from, to, nil)
		})
	if err != nil {
		return inv.GoodsReceipt{}, err
	}
	return a.store.ReceiptByID(ctx, restaurantID, receiptID)
}

// cancelDocument is the shared cancel flow: guard posted, create a reversal
// document row, mirror its moves, cancel the GL journal, mark cancelled —
// all in one transaction (§6).
func (a *App) cancelDocument(ctx context.Context, restaurantID, docID uuid.UUID, docKind, sourceKind string, insertReversal func(context.Context, ports.Store, uuid.UUID, time.Time) error, mark func(context.Context, ports.Store, string, string) error) error {
	return a.store.InTx(ctx, func(tx *sql.Tx, st ports.Store) error {
		status, err := st.LockDocument(ctx, docKind, restaurantID, docID)
		if err != nil {
			return err
		}
		if err := cancelGuard(status); err != nil {
			return err
		}
		reversalID := a.newID()
		today := a.now()
		if err := insertReversal(ctx, st, reversalID, today); err != nil {
			return err
		}
		if err := a.reverseMoves(ctx, st, restaurantID, docKind, docID, reversalID, today); err != nil {
			return err
		}
		if _, err := a.ledger.CancelJournalForSource(ctx, tx, restaurantID, sourceKind, docID); err != nil {
			return err
		}
		return mark(ctx, st, inv.DocPosted, inv.DocCancelled)
	})
}

// --- write-offs --------------------------------------------------------

type WriteOffLineInput struct {
	ProductID uuid.UUID
	QtyInput  float64
	Unit      string
}

func (a *App) CreateWriteOff(ctx context.Context, restaurantID uuid.UUID, reason string, businessDate time.Time, note string, lineInputs []WriteOffLineInput) (inv.WriteOff, error) {
	if !inv.ValidReason(reason) {
		return inv.WriteOff{}, inv.ErrInvalidReason
	}
	if len(lineInputs) == 0 {
		return inv.WriteOff{}, inv.ErrEmptyDocument
	}
	w := inv.WriteOff{ID: a.newID(), RestaurantID: restaurantID, Reason: reason, Status: inv.DocDraft, BusinessDate: businessDate, Note: note}
	for i, li := range lineInputs {
		p, err := a.store.ProductByID(ctx, restaurantID, li.ProductID)
		if err != nil {
			return inv.WriteOff{}, err
		}
		qtyBase, err := inv.ToBaseMilli(li.QtyInput, li.Unit, p.StockUnit)
		if err != nil {
			return inv.WriteOff{}, err
		}
		w.Lines = append(w.Lines, inv.WriteOffLine{ID: a.newID(), WriteOffID: w.ID, ProductID: li.ProductID, QtyBaseMilli: qtyBase, InputUnit: li.Unit, Seq: i + 1})
	}
	if err := a.store.InsertWriteOff(ctx, w); err != nil {
		return inv.WriteOff{}, err
	}
	return w, nil
}

func (a *App) WriteOff(ctx context.Context, restaurantID, id uuid.UUID) (inv.WriteOff, error) {
	return a.store.WriteOffByID(ctx, restaurantID, id)
}

func (a *App) WriteOffs(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.WriteOff, error) {
	return a.store.WriteOffs(ctx, restaurantID, from, status)
}

func (a *App) PostWriteOff(ctx context.Context, restaurantID, writeOffID, postedBy uuid.UUID) (inv.WriteOff, error) {
	err := a.store.InTx(ctx, func(tx *sql.Tx, st ports.Store) error {
		status, err := st.LockDocument(ctx, inv.DocKindWriteoff, restaurantID, writeOffID)
		if err != nil {
			return err
		}
		if err := postGuard(status); err != nil {
			return err
		}
		w, err := st.WriteOffByID(ctx, restaurantID, writeOffID)
		if err != nil {
			return err
		}
		pids := make([]uuid.UUID, len(w.Lines))
		for i, l := range w.Lines {
			pids[i] = l.ProductID
		}
		if err := a.checkBackdate(ctx, st, restaurantID, w.BusinessDate, pids); err != nil {
			return err
		}
		var total int64
		for _, l := range w.Lines {
			cost, _, err := a.applyIssueMove(ctx, st, restaurantID, l.ProductID, inv.MoveWriteoff, l.QtyBaseMilli, w.BusinessDate, inv.DocKindWriteoff, writeOffID, nil)
			if err != nil {
				return err
			}
			total += cost
		}
		if _, err := a.ledger.PostInventoryJournal(ctx, tx, restaurantID, postedBy, inv.SourceWriteoff, writeOffID, w.BusinessDate, []ports.JournalLine{
			{Purpose: "inventory_shrinkage", Side: "debit", AmountCents: total},
			{Purpose: "inventory", Side: "credit", AmountCents: total},
		}); err != nil {
			return err
		}
		return st.MarkWriteOffStatus(ctx, restaurantID, writeOffID, inv.DocDraft, inv.DocPosted, &postedBy)
	})
	if err != nil {
		return inv.WriteOff{}, err
	}
	return a.store.WriteOffByID(ctx, restaurantID, writeOffID)
}

func (a *App) CancelWriteOff(ctx context.Context, restaurantID, writeOffID uuid.UUID) (inv.WriteOff, error) {
	err := a.cancelDocument(ctx, restaurantID, writeOffID, inv.DocKindWriteoff, inv.SourceWriteoff,
		func(ctx context.Context, st ports.Store, reversalID uuid.UUID, businessDate time.Time) error {
			orig, err := st.WriteOffByID(ctx, restaurantID, writeOffID)
			if err != nil {
				return err
			}
			return st.InsertWriteOff(ctx, inv.WriteOff{
				ID: reversalID, RestaurantID: restaurantID, Reason: orig.Reason, Status: inv.DocPosted,
				BusinessDate: businessDate, Note: "reversal", ReversalOf: &writeOffID,
			})
		},
		func(ctx context.Context, st ports.Store, from, to string) error {
			return st.MarkWriteOffStatus(ctx, restaurantID, writeOffID, from, to, nil)
		})
	if err != nil {
		return inv.WriteOff{}, err
	}
	return a.store.WriteOffByID(ctx, restaurantID, writeOffID)
}
