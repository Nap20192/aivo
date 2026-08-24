package http

import (
	"net/http"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/domain/platform"
	inventoryapp "aivo/internal/inventory/app"

	"uuid"
)

// Inventory endpoints (§10). All gated manager+ (storekeeper role deferred —
// see impl-contract-2 deviations). Quantities are numbers in a display unit
// + a unit code; the server converts. Money is integer cents.

func qtyNum(milli int64) float64 { return float64(milli) / 1000 }

func milliOf(num float64) int64 { return int64(num*1000 + 0.5) }

// --- products ----------------------------------------------------------

func productView(p inv.Product) map[string]any {
	var minStock any
	if p.MinStock != nil {
		minStock = qtyNum(*p.MinStock)
	}
	return map[string]any{
		"id": p.ID, "sku": p.SKU, "name": p.Name, "type": p.Type, "stock_unit": p.StockUnit,
		"menu_item_id": p.MenuItemID, "min_stock": minStock, "archived": p.Archived,
	}
}

func onHandView(unit string, oh inv.OnHand) map[string]any {
	return map[string]any{
		"qty": qtyNum(oh.QtyMilli), "unit": unit, "value_cents": oh.ValueCents, "avg_cents": oh.AvgCentsPerBase(),
	}
}

func (h *handler) invListProducts(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	ps, err := h.Inventory.Products(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(ps))
	for i, p := range ps {
		out[i] = productView(p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invCreateProduct(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req struct {
		SKU        string     `json:"sku"`
		Name       string     `json:"name"`
		Type       string     `json:"type"`
		StockUnit  string     `json:"stock_unit"`
		MenuItemID *uuid.UUID `json:"menu_item_id"`
		MinStock   *float64   `json:"min_stock"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	in := inventoryapp.ProductInput{SKU: req.SKU, Name: req.Name, Type: req.Type, StockUnit: req.StockUnit, MenuItemID: req.MenuItemID}
	if req.MinStock != nil {
		m := milliOf(*req.MinStock)
		in.MinStock = &m
	}
	p, err := h.Inventory.CreateProduct(r.Context(), rest.ID, in)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, productView(p))
}

func (h *handler) invGetProduct(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	pid, ok := pathUUID(w, r, "pid")
	if !ok {
		return
	}
	p, oh, err := h.Inventory.Product(r.Context(), rest.ID, pid)
	if writeAppErr(w, err) {
		return
	}
	v := productView(p)
	v["on_hand"] = onHandView(p.StockUnit, oh)
	writeJSON(w, http.StatusOK, v)
}

func (h *handler) invPatchProduct(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	pid, ok := pathUUID(w, r, "pid")
	if !ok {
		return
	}
	var req struct {
		Name       *string    `json:"name"`
		MinStock   *float64   `json:"min_stock"`
		MenuItemID *uuid.UUID `json:"menu_item_id"`
		Archived   *bool      `json:"archived"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	patch := inventoryapp.ProductPatch{Name: req.Name, MenuItemID: req.MenuItemID, Archived: req.Archived}
	if req.MinStock != nil {
		m := milliOf(*req.MinStock)
		patch.MinStock = &m
	}
	p, err := h.Inventory.UpdateProduct(r.Context(), rest.ID, pid, patch)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, productView(p))
}

// --- tech cards --------------------------------------------------------

func techCardView(v inventoryapp.TechCardView) map[string]any {
	c := v.Card
	var validTo any
	if c.ValidTo != nil {
		validTo = c.ValidTo.Format("2006-01-02")
	}
	lines := make([]map[string]any, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = map[string]any{"ingredient_product_id": l.IngredientProductID, "qty": qtyNum(l.Qty), "seq": l.Seq}
	}
	m := map[string]any{
		"id": c.ID, "product_id": c.ProductID, "valid_from": c.ValidFrom.Format("2006-01-02"),
		"valid_to": validTo, "consumption": c.Consumption, "yield_qty": qtyNum(c.YieldMilli),
		"cost_cents": v.CostCents, "lines": lines,
	}
	if v.Costings != nil {
		costings := make([]map[string]any, len(v.Costings))
		for i, rc := range v.Costings {
			costings[i] = map[string]any{"cost_cents": rc.CostCents, "method": rc.Method, "computed_at": rc.ComputedAt}
		}
		m["cost_history"] = costings
	}
	return m
}

func (h *handler) invTechCards(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	pid, ok := pathUUID(w, r, "pid")
	if !ok {
		return
	}
	views, err := h.Inventory.TechCardVersions(r.Context(), rest.ID, pid)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(views))
	for i, v := range views {
		out[i] = techCardView(v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invActiveTechCard(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	pid, ok := pathUUID(w, r, "pid")
	if !ok {
		return
	}
	on := h.Inventory.Now()
	if q := r.URL.Query().Get("on"); q != "" {
		d, err := inventoryapp.ParseDate(q)
		if writeAppErr(w, err) {
			return
		}
		on = d
	}
	v, err := h.Inventory.ActiveTechCard(r.Context(), rest.ID, pid, on)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, techCardView(v))
}

func (h *handler) invCreateTechCard(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	pid, ok := pathUUID(w, r, "pid")
	if !ok {
		return
	}
	var req struct {
		ValidFrom   string   `json:"valid_from"`
		Consumption string   `json:"consumption"`
		YieldQty    *float64 `json:"yield_qty"`
		Lines       []struct {
			IngredientProductID uuid.UUID `json:"ingredient_product_id"`
			Qty                 float64   `json:"qty"`
			Unit                string    `json:"unit"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	validFrom, err := inventoryapp.ParseDate(req.ValidFrom)
	if writeAppErr(w, err) {
		return
	}
	var yield int64
	if req.YieldQty != nil {
		yield = milliOf(*req.YieldQty)
	}
	lines := make([]inventoryapp.TechCardLineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = inventoryapp.TechCardLineInput{IngredientProductID: l.IngredientProductID, QtyInput: l.Qty, Unit: l.Unit}
	}
	tc, err := h.Inventory.CreateTechCardVersion(r.Context(), rest.ID, pid, validFrom, req.Consumption, yield, lines, u.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, techCardView(inventoryapp.TechCardView{Card: tc}))
}

func (h *handler) invGetTechCard(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	tcid, ok := pathUUID(w, r, "tcid")
	if !ok {
		return
	}
	v, err := h.Inventory.TechCard(r.Context(), rest.ID, tcid)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, techCardView(v))
}

func (h *handler) invRecost(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	tcid, ok := pathUUID(w, r, "tcid")
	if !ok {
		return
	}
	cost, err := h.Inventory.Recost(r.Context(), rest.ID, tcid, u.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cost_cents": cost})
}

// --- suppliers ---------------------------------------------------------

func supplierView(s inv.Supplier) map[string]any {
	return map[string]any{"id": s.ID, "name": s.Name, "contacts": s.Contacts, "note": s.Note, "archived": s.Archived}
}

func (h *handler) invListSuppliers(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	ss, err := h.Inventory.Suppliers(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(ss))
	for i, s := range ss {
		out[i] = supplierView(s)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) invCreateSupplier(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req struct {
		Name     string            `json:"name"`
		Contacts map[string]string `json:"contacts"`
		Note     string            `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.Inventory.CreateSupplier(r.Context(), rest.ID, req.Name, req.Contacts, req.Note)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, supplierView(s))
}

func (h *handler) invPatchSupplier(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	sid, ok := pathUUID(w, r, "sid")
	if !ok {
		return
	}
	var req struct {
		Name     *string           `json:"name"`
		Contacts map[string]string `json:"contacts"`
		Archived *bool             `json:"archived"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.Inventory.UpdateSupplier(r.Context(), rest.ID, sid, inventoryapp.SupplierPatch{Name: req.Name, Contacts: req.Contacts, Archived: req.Archived})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, supplierView(s))
}
