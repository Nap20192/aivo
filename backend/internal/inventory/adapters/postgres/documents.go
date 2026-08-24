package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// tableFor maps a document kind to its table.
func tableFor(docKind string) (string, error) {
	switch docKind {
	case inv.DocKindReceipt:
		return "goods_receipts", nil
	case inv.DocKindWriteoff:
		return "write_offs", nil
	case inv.DocKindStocktake:
		return "stocktakes", nil
	}
	return "", fmt.Errorf("inventory store: unknown doc kind %q", docKind)
}

func (s *Store) LockDocument(ctx context.Context, docKind string, restaurantID, id uuid.UUID) (string, error) {
	table, err := tableFor(docKind)
	if err != nil {
		return "", err
	}
	var status string
	err = s.q.QueryRowContext(ctx,
		`SELECT status FROM `+table+` WHERE restaurant_id = $1 AND id = $2 FOR UPDATE`, restaurantID, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ports.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("inventory store: lock %s: %w", table, err)
	}
	return status, nil
}

// --- goods receipts ----------------------------------------------------

const receiptCols = `id, restaurant_id, supplier_id, status, business_date, note, posted_at, posted_by, reversal_of, created_at`

func scanReceipt(row interface{ Scan(...any) error }) (inv.GoodsReceipt, error) {
	var r inv.GoodsReceipt
	err := row.Scan(&r.ID, &r.RestaurantID, &r.SupplierID, &r.Status, &r.BusinessDate, &r.Note,
		&r.PostedAt, &r.PostedBy, &r.ReversalOf, &r.CreatedAt)
	return r, err
}

func (s *Store) InsertReceipt(ctx context.Context, r inv.GoodsReceipt) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO goods_receipts (id, restaurant_id, supplier_id, status, business_date, note, posted_at, posted_by, reversal_of)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.RestaurantID, r.SupplierID, r.Status, dateStr(r.BusinessDate), r.Note, r.PostedAt, r.PostedBy, r.ReversalOf)
	if err != nil {
		return fmt.Errorf("inventory store: insert receipt: %w", err)
	}
	for _, l := range r.Lines {
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO goods_receipt_lines (id, receipt_id, product_id, qty_base_milli, input_unit, unit_price_cents, line_cost_cents, seq)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			l.ID, r.ID, l.ProductID, l.QtyBaseMilli, l.InputUnit, l.UnitPriceCents, l.LineCostCents, l.Seq); err != nil {
			return fmt.Errorf("inventory store: insert receipt line: %w", err)
		}
	}
	return nil
}

func (s *Store) receiptLines(ctx context.Context, receiptID uuid.UUID) ([]inv.GoodsReceiptLine, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, receipt_id, product_id, qty_base_milli, input_unit, unit_price_cents, line_cost_cents, seq FROM goods_receipt_lines WHERE receipt_id = $1 ORDER BY seq`, receiptID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: receipt lines: %w", err)
	}
	defer rows.Close()
	out := []inv.GoodsReceiptLine{}
	for rows.Next() {
		var l inv.GoodsReceiptLine
		if err := rows.Scan(&l.ID, &l.ReceiptID, &l.ProductID, &l.QtyBaseMilli, &l.InputUnit, &l.UnitPriceCents, &l.LineCostCents, &l.Seq); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) ReceiptByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.GoodsReceipt, error) {
	r, err := scanReceipt(s.q.QueryRowContext(ctx, `SELECT `+receiptCols+` FROM goods_receipts WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.GoodsReceipt{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.GoodsReceipt{}, fmt.Errorf("inventory store: receipt by id: %w", err)
	}
	if r.Lines, err = s.receiptLines(ctx, id); err != nil {
		return inv.GoodsReceipt{}, err
	}
	return r, nil
}

func (s *Store) Receipts(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.GoodsReceipt, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+receiptCols+` FROM goods_receipts
		 WHERE restaurant_id = $1 AND ($2 = '' OR business_date >= $2::date) AND ($3 = '' OR status = $3)
		 ORDER BY business_date DESC, created_at DESC`, restaurantID, from, status)
	if err != nil {
		return nil, fmt.Errorf("inventory store: receipts: %w", err)
	}
	defer rows.Close()
	out := []inv.GoodsReceipt{}
	for rows.Next() {
		r, err := scanReceipt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MarkReceiptStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error {
	return s.markDocStatus(ctx, "goods_receipts", restaurantID, id, from, to, postedBy)
}

// --- write-offs --------------------------------------------------------

const writeOffCols = `id, restaurant_id, reason, status, business_date, note, posted_at, posted_by, reversal_of, created_at`

func scanWriteOff(row interface{ Scan(...any) error }) (inv.WriteOff, error) {
	var w inv.WriteOff
	err := row.Scan(&w.ID, &w.RestaurantID, &w.Reason, &w.Status, &w.BusinessDate, &w.Note,
		&w.PostedAt, &w.PostedBy, &w.ReversalOf, &w.CreatedAt)
	return w, err
}

func (s *Store) InsertWriteOff(ctx context.Context, w inv.WriteOff) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO write_offs (id, restaurant_id, reason, status, business_date, note, posted_at, posted_by, reversal_of)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		w.ID, w.RestaurantID, w.Reason, w.Status, dateStr(w.BusinessDate), w.Note, w.PostedAt, w.PostedBy, w.ReversalOf)
	if err != nil {
		return fmt.Errorf("inventory store: insert write-off: %w", err)
	}
	for _, l := range w.Lines {
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO write_off_lines (id, write_off_id, product_id, qty_base_milli, input_unit, seq) VALUES ($1,$2,$3,$4,$5,$6)`,
			l.ID, w.ID, l.ProductID, l.QtyBaseMilli, l.InputUnit, l.Seq); err != nil {
			return fmt.Errorf("inventory store: insert write-off line: %w", err)
		}
	}
	return nil
}

func (s *Store) writeOffLines(ctx context.Context, writeOffID uuid.UUID) ([]inv.WriteOffLine, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, write_off_id, product_id, qty_base_milli, input_unit, seq FROM write_off_lines WHERE write_off_id = $1 ORDER BY seq`, writeOffID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: write-off lines: %w", err)
	}
	defer rows.Close()
	out := []inv.WriteOffLine{}
	for rows.Next() {
		var l inv.WriteOffLine
		if err := rows.Scan(&l.ID, &l.WriteOffID, &l.ProductID, &l.QtyBaseMilli, &l.InputUnit, &l.Seq); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) WriteOffByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.WriteOff, error) {
	w, err := scanWriteOff(s.q.QueryRowContext(ctx, `SELECT `+writeOffCols+` FROM write_offs WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.WriteOff{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.WriteOff{}, fmt.Errorf("inventory store: write-off by id: %w", err)
	}
	if w.Lines, err = s.writeOffLines(ctx, id); err != nil {
		return inv.WriteOff{}, err
	}
	return w, nil
}

func (s *Store) WriteOffs(ctx context.Context, restaurantID uuid.UUID, from, status string) ([]inv.WriteOff, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+writeOffCols+` FROM write_offs
		 WHERE restaurant_id = $1 AND ($2 = '' OR business_date >= $2::date) AND ($3 = '' OR status = $3)
		 ORDER BY business_date DESC, created_at DESC`, restaurantID, from, status)
	if err != nil {
		return nil, fmt.Errorf("inventory store: write-offs: %w", err)
	}
	defer rows.Close()
	out := []inv.WriteOff{}
	for rows.Next() {
		w, err := scanWriteOff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) MarkWriteOffStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error {
	return s.markDocStatus(ctx, "write_offs", restaurantID, id, from, to, postedBy)
}

// --- stocktakes --------------------------------------------------------

const stocktakeCols = `id, restaurant_id, status, business_date, note, posted_at, posted_by, reversal_of, created_at`

func scanStocktake(row interface{ Scan(...any) error }) (inv.Stocktake, error) {
	var st inv.Stocktake
	err := row.Scan(&st.ID, &st.RestaurantID, &st.Status, &st.BusinessDate, &st.Note,
		&st.PostedAt, &st.PostedBy, &st.ReversalOf, &st.CreatedAt)
	return st, err
}

func (s *Store) InsertStocktake(ctx context.Context, st inv.Stocktake) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO stocktakes (id, restaurant_id, status, business_date, note, posted_at, posted_by, reversal_of)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		st.ID, st.RestaurantID, st.Status, dateStr(st.BusinessDate), st.Note, st.PostedAt, st.PostedBy, st.ReversalOf)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: insert stocktake: %w", err)
	}
	if len(st.Lines) > 0 {
		return s.ReplaceStocktakeLines(ctx, st.ID, st.Lines)
	}
	return nil
}

func (s *Store) stocktakeLines(ctx context.Context, stocktakeID uuid.UUID) ([]inv.StocktakeLine, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, stocktake_id, product_id, counted_qty_milli, expected_qty_milli, variance_qty_milli, variance_cost_cents, seq FROM stocktake_lines WHERE stocktake_id = $1 ORDER BY seq`, stocktakeID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: stocktake lines: %w", err)
	}
	defer rows.Close()
	out := []inv.StocktakeLine{}
	for rows.Next() {
		var l inv.StocktakeLine
		if err := rows.Scan(&l.ID, &l.StocktakeID, &l.ProductID, &l.CountedQtyMilli, &l.ExpectedQtyMilli, &l.VarianceQtyMilli, &l.VarianceCostCents, &l.Seq); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) StocktakeByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Stocktake, error) {
	st, err := scanStocktake(s.q.QueryRowContext(ctx, `SELECT `+stocktakeCols+` FROM stocktakes WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.Stocktake{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.Stocktake{}, fmt.Errorf("inventory store: stocktake by id: %w", err)
	}
	if st.Lines, err = s.stocktakeLines(ctx, id); err != nil {
		return inv.Stocktake{}, err
	}
	return st, nil
}

func (s *Store) Stocktakes(ctx context.Context, restaurantID uuid.UUID, status string) ([]inv.Stocktake, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+stocktakeCols+` FROM stocktakes WHERE restaurant_id = $1 AND ($2 = '' OR status = $2) ORDER BY created_at DESC`,
		restaurantID, status)
	if err != nil {
		return nil, fmt.Errorf("inventory store: stocktakes: %w", err)
	}
	defer rows.Close()
	out := []inv.Stocktake{}
	for rows.Next() {
		st, err := scanStocktake(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceStocktakeLines(ctx context.Context, stocktakeID uuid.UUID, lines []inv.StocktakeLine) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM stocktake_lines WHERE stocktake_id = $1`, stocktakeID); err != nil {
		return fmt.Errorf("inventory store: clear stocktake lines: %w", err)
	}
	for _, l := range lines {
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO stocktake_lines (id, stocktake_id, product_id, counted_qty_milli, expected_qty_milli, variance_qty_milli, variance_cost_cents, seq)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			l.ID, stocktakeID, l.ProductID, l.CountedQtyMilli, l.ExpectedQtyMilli, l.VarianceQtyMilli, l.VarianceCostCents, l.Seq); err != nil {
			return fmt.Errorf("inventory store: insert stocktake line: %w", err)
		}
	}
	return nil
}

func (s *Store) MarkStocktakeStatus(ctx context.Context, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error {
	return s.markDocStatus(ctx, "stocktakes", restaurantID, id, from, to, postedBy)
}

// markDocStatus flips a stock document's status, guarded by its current
// status (RowsAffected == 0 → ErrConflict). Posting stamps posted_at/by.
func (s *Store) markDocStatus(ctx context.Context, table string, restaurantID, id uuid.UUID, from, to string, postedBy *uuid.UUID) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE `+table+` SET status = $4,
		   posted_at = CASE WHEN $4 = 'posted' THEN now() ELSE posted_at END,
		   posted_by = COALESCE($5, posted_by)
		 WHERE restaurant_id = $1 AND id = $2 AND status = $3`,
		restaurantID, id, from, to, postedBy)
	if err != nil {
		return fmt.Errorf("inventory store: mark %s status: %w", table, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrConflict
	}
	return nil
}
