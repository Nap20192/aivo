package http

import (
	"net/http"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/adapters/jwtauth"
	inventoryapp "aivo/internal/inventory/app"

	"uuid"
)

// --- goods receipts ----------------------------------------------------

func receiptView(r inv.GoodsReceipt) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"product_id": l.ProductID, "qty": qtyNum(l.QtyBaseMilli), "unit": l.InputUnit,
			"unit_price_cents": l.UnitPriceCents, "line_cost_cents": l.LineCostCents, "seq": l.Seq,
		}
	}
	return map[string]any{
		"id": r.ID, "supplier_id": r.SupplierID, "status": r.Status,
		"business_date": r.BusinessDate.Format("2006-01-02"), "note": r.Note,
		"posted_at": r.PostedAt, "reversal_of": r.ReversalOf, "total_cents": r.TotalCents(), "lines": lines,
	}
}

func (h *handler) invReceipts(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	rs, err := h.Inventory.Receipts(r.Context(), restaurantID, r.URL.Query().Get("from"), r.URL.Query().Get("status"))
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(rs))
	for i, rec := range rs {
		out[i] = receiptView(rec)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invCreateReceipt(w http.ResponseWriter, r *http.Request, claims jwtauth.Claims, restaurantID uuid.UUID) {
	var req struct {
		SupplierID   *uuid.UUID `json:"supplier_id"`
		BusinessDate string     `json:"business_date"`
		Note         string     `json:"note"`
		Lines        []struct {
			ProductID      uuid.UUID `json:"product_id"`
			Qty            float64   `json:"qty"`
			Unit           string    `json:"unit"`
			UnitPriceCents int64     `json:"unit_price_cents"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	bd, err := inventoryapp.ParseDate(req.BusinessDate)
	if writeAppErr(w, err) {
		return
	}
	lines := make([]inventoryapp.ReceiptLineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = inventoryapp.ReceiptLineInput{ProductID: l.ProductID, QtyInput: l.Qty, Unit: l.Unit, UnitPriceCents: l.UnitPriceCents}
	}
	rec, err := h.Inventory.CreateReceipt(r.Context(), restaurantID, req.SupplierID, bd, req.Note, lines, claims.UserID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, receiptView(rec))
}

func (h *handler) invGetReceipt(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "rid")
	if !ok {
		return
	}
	rec, err := h.Inventory.Receipt(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, receiptView(rec))
}

func (h *handler) invPostReceipt(w http.ResponseWriter, r *http.Request, claims jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "rid")
	if !ok {
		return
	}
	rec, err := h.Inventory.PostReceipt(r.Context(), restaurantID, id, claims.UserID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, receiptView(rec))
}

func (h *handler) invCancelReceipt(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "rid")
	if !ok {
		return
	}
	rec, err := h.Inventory.CancelReceipt(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, receiptView(rec))
}

// --- write-offs --------------------------------------------------------

func writeOffView(w inv.WriteOff) map[string]any {
	lines := make([]map[string]any, len(w.Lines))
	for i, l := range w.Lines {
		lines[i] = map[string]any{"product_id": l.ProductID, "qty": qtyNum(l.QtyBaseMilli), "unit": l.InputUnit, "seq": l.Seq}
	}
	return map[string]any{
		"id": w.ID, "reason": w.Reason, "status": w.Status,
		"business_date": w.BusinessDate.Format("2006-01-02"), "note": w.Note,
		"posted_at": w.PostedAt, "reversal_of": w.ReversalOf, "lines": lines,
	}
}

func (h *handler) invWriteOffs(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	ws, err := h.Inventory.WriteOffs(r.Context(), restaurantID, r.URL.Query().Get("from"), r.URL.Query().Get("status"))
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(ws))
	for i, wo := range ws {
		out[i] = writeOffView(wo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invCreateWriteOff(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	var req struct {
		Reason       string `json:"reason"`
		Note         string `json:"note"`
		BusinessDate string `json:"business_date"`
		Lines        []struct {
			ProductID uuid.UUID `json:"product_id"`
			Qty       float64   `json:"qty"`
			Unit      string    `json:"unit"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	bd, err := inventoryapp.ParseDate(req.BusinessDate)
	if writeAppErr(w, err) {
		return
	}
	lines := make([]inventoryapp.WriteOffLineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = inventoryapp.WriteOffLineInput{ProductID: l.ProductID, QtyInput: l.Qty, Unit: l.Unit}
	}
	wo, err := h.Inventory.CreateWriteOff(r.Context(), restaurantID, req.Reason, bd, req.Note, lines)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, writeOffView(wo))
}

func (h *handler) invGetWriteOff(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "wid")
	if !ok {
		return
	}
	wo, err := h.Inventory.WriteOff(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, writeOffView(wo))
}

func (h *handler) invPostWriteOff(w http.ResponseWriter, r *http.Request, claims jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "wid")
	if !ok {
		return
	}
	wo, err := h.Inventory.PostWriteOff(r.Context(), restaurantID, id, claims.UserID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, writeOffView(wo))
}

func (h *handler) invCancelWriteOff(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "wid")
	if !ok {
		return
	}
	wo, err := h.Inventory.CancelWriteOff(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, writeOffView(wo))
}

// --- stocktakes --------------------------------------------------------

func stocktakeView(s inv.Stocktake) map[string]any {
	lines := make([]map[string]any, len(s.Lines))
	for i, l := range s.Lines {
		var expected any
		if l.ExpectedQtyMilli != nil {
			expected = qtyNum(*l.ExpectedQtyMilli)
		}
		lines[i] = map[string]any{
			"product_id": l.ProductID, "counted_qty": qtyNum(l.CountedQtyMilli), "expected_qty": expected,
			"variance_qty": qtyNum(l.VarianceQtyMilli), "variance_cost_cents": l.VarianceCostCents, "seq": l.Seq,
		}
	}
	return map[string]any{
		"id": s.ID, "status": s.Status, "business_date": s.BusinessDate.Format("2006-01-02"),
		"note": s.Note, "posted_at": s.PostedAt, "reversal_of": s.ReversalOf, "lines": lines,
	}
}

func (h *handler) invStocktakes(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	ss, err := h.Inventory.Stocktakes(r.Context(), restaurantID, r.URL.Query().Get("status"))
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(ss))
	for i, s := range ss {
		out[i] = stocktakeView(s)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invCreateStocktake(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	var req struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.Inventory.StartStocktake(r.Context(), restaurantID, dateOnly(time.Now()), req.Note)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, stocktakeView(s))
}

func (h *handler) invGetStocktake(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	s, err := h.Inventory.Stocktake(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, stocktakeView(s))
}

func (h *handler) invEnterCounts(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	var req struct {
		Lines []struct {
			ProductID  uuid.UUID `json:"product_id"`
			CountedQty float64   `json:"counted_qty"`
			Unit       string    `json:"unit"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	counts := make([]inventoryapp.StocktakeCountInput, len(req.Lines))
	for i, l := range req.Lines {
		counts[i] = inventoryapp.StocktakeCountInput{ProductID: l.ProductID, QtyInput: l.CountedQty, Unit: l.Unit}
	}
	s, err := h.Inventory.EnterCounts(r.Context(), restaurantID, id, counts)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, stocktakeView(s))
}

func (h *handler) invDryRunStocktake(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	rows, err := h.Inventory.DryRun(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = map[string]any{
			"product_id": row.ProductID, "counted_qty": qtyNum(row.CountedQtyMilli), "expected_qty": qtyNum(row.ExpectedQtyMilli),
			"variance_qty": qtyNum(row.VarianceQtyMilli), "variance_cost_cents": row.VarianceCostCents,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out})
}

func (h *handler) invPostStocktake(w http.ResponseWriter, r *http.Request, claims jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	s, err := h.Inventory.PostStocktake(r.Context(), restaurantID, id, claims.UserID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, stocktakeView(s))
}

func (h *handler) invCancelStocktake(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	id, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	s, err := h.Inventory.CancelStocktake(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, stocktakeView(s))
}

// --- on-hand / moves / food-cost ---------------------------------------

func (h *handler) invOnHand(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	rows, err := h.Inventory.OnHand(r.Context(), restaurantID, r.URL.Query().Get("low_stock") != "")
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = map[string]any{
			"product_id": row.Product.ID, "sku": row.Product.SKU, "name": row.Product.Name,
			"qty": qtyNum(row.OnHand.QtyMilli), "unit": row.Product.StockUnit,
			"value_cents": row.OnHand.ValueCents, "avg_cents": row.OnHand.AvgCentsPerBase(), "below_min": row.BelowMin,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invStockMoves(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	var product *uuid.UUID
	if q := r.URL.Query().Get("product"); q != "" {
		id, err := uuid.Parse(q)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", "bad product id")
			return
		}
		product = &id
	}
	moves, err := h.Inventory.StockMoves(r.Context(), restaurantID, r.URL.Query().Get("from"), product)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(moves))
	for i, m := range moves {
		out[i] = map[string]any{
			"id": m.ID, "product_id": m.ProductID, "kind": m.Kind, "qty": qtyNum(m.QtyMilli),
			"cost_cents": m.CostCents, "estimated": m.Estimated,
			"business_date": m.BusinessDate.Format("2006-01-02"), "recorded_at": m.RecordedAt,
			"doc_kind": m.DocKind, "doc_id": m.DocID,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invFoodCost(w http.ResponseWriter, r *http.Request, _ jwtauth.Claims, restaurantID uuid.UUID) {
	q := r.URL.Query()
	rep, err := h.Inventory.FoodCostReport(r.Context(), restaurantID, q.Get("from"), q.Get("to"))
	if writeAppErr(w, err) {
		return
	}
	items := make([]map[string]any, len(rep.Items))
	for i, it := range rep.Items {
		items[i] = map[string]any{
			"menu_item_id": it.MenuItemID, "name": it.Name, "revenue_cents": it.RevenueCents,
			"theoretical_cogs_cents": it.TheoreticalCogsCents, "food_cost_pct": it.FoodCostPct,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"totals": map[string]any{
			"revenue_cents": rep.TotalRevenueCents, "actual_cogs_cents": rep.TotalActualCogsCents,
			"theoretical_cogs_cents": rep.TotalTheoreticalCogsCents,
		},
		"estimated_share": rep.EstimatedShare,
	})
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
