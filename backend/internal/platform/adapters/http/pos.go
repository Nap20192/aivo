package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/platform"
	posdomain "aivo/internal/domain/pos"
	posapp "aivo/internal/pos/app"

	"uuid"
)

// Shapes here match the POS client (frontend/pos/src/types.ts): display
// times as "HH:MM" strings, options as labels, unit prices inclusive of
// option deltas. v1 models a single till per restaurant: till is always
// 1 and other_till_shift always null.

// posFunc runs with the session user and the restaurant the POS session
// operates on.
type posFunc func(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID)

// pos is auth + POS restaurant resolution: restaurant-scoped staff act
// on their own restaurant; org-wide users (owners) act on the org's
// first restaurant or pass ?restaurant_id= to pick one.
func (h *handler) pos(next posFunc) http.HandlerFunc {
	return h.auth(func(w http.ResponseWriter, r *http.Request, u domain.User) {
		if u.RestaurantID != nil {
			next(w, r, u, *u.RestaurantID)
			return
		}
		if q := r.URL.Query().Get("restaurant_id"); q != "" {
			id, err := uuid.Parse(q)
			if err != nil {
				writeErr(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			// Org scope check: must be one of the org's restaurants.
			if _, err := h.Platform.Restaurant(r.Context(), u.OrgID, id); writeAppErr(w, err) {
				return
			}
			next(w, r, u, id)
			return
		}
		rests, err := h.Platform.Restaurants(r.Context(), u.OrgID)
		if writeAppErr(w, err) {
			return
		}
		if len(rests) == 0 {
			writeErr(w, http.StatusNotFound, "not_found", "org has no restaurants")
			return
		}
		next(w, r, u, rests[0].ID)
	})
}

// --- Views -------------------------------------------------------------

// posLocation is the timezone POS display times ("HH:MM") render in,
// set from RESTAURANT_TZ via Deps.POSLocation (default: server-local).
// ponytail: single global TZ; move to a per-restaurant column when
// multi-region tenants need it.
var posLocation = time.Local

func hhmm(t time.Time) string { return t.In(posLocation).Format("15:04") }

type posShiftView struct {
	ID                uuid.UUID `json:"id"`
	Number            string    `json:"number"` // "shift-121"
	Till              int       `json:"till"`
	Cashier           string    `json:"cashier"`
	OpenedAt          string    `json:"opened_at"` // "16:04"
	OpeningFloatCents int       `json:"opening_float_cents"`
	ExpectedCents     int       `json:"expected_cents"`
	State             string    `json:"state"` // open|closed|accepted
}

func (h *handler) toPosShiftView(ctx context.Context, s posdomain.Shift, number, expectedCents int) posShiftView {
	cashier := s.Cashier // denormalized at open time
	if cashier == "" {
		// Shifts opened before the cashier column existed.
		if u, err := h.Platform.User(ctx, s.OpenedBy); err == nil {
			cashier = displayName(u.Email)
		}
	}
	return posShiftView{
		ID: s.ID, Number: fmt.Sprintf("shift-%d", number), Till: 1, Cashier: cashier,
		OpenedAt: hhmm(s.OpenedAt), OpeningFloatCents: s.OpeningFloatCents, ExpectedCents: expectedCents,
		State: string(s.State()),
	}
}

type posLineView struct {
	ID             uuid.UUID `json:"id"`
	MenuItemID     uuid.UUID `json:"menu_item_id"`
	Name           string    `json:"name"`
	Qty            int       `json:"qty"`
	Options        []string  `json:"options"`
	UnitPriceCents int       `json:"unit_price_cents"` // base + option deltas
}

type posTicketView struct {
	ID       uuid.UUID     `json:"id"`
	Lines    []posLineView `json:"lines"`
	Note     *string       `json:"note"`
	Source   string        `json:"source"`
	PlacedAt *string       `json:"placed_at"`
	FiredAt  *string       `json:"fired_at"`
	// Extras beyond the client type, harmless and useful for debugging.
	TableID    uuid.UUID `json:"table_id"`
	ShiftID    uuid.UUID `json:"shift_id"`
	Status     string    `json:"status"`
	TotalCents int       `json:"total_cents"`
}

func toPosTicketView(t posdomain.Ticket) posTicketView {
	lines := make([]posLineView, len(t.Lines))
	var lastFired *time.Time
	for i, l := range t.Lines {
		unit := l.UnitPriceCents
		labels := []string{}
		for _, o := range l.Options {
			unit += o.PriceDeltaCents
			labels = append(labels, o.Label)
		}
		lines[i] = posLineView{ID: l.ID, MenuItemID: l.MenuItemID, Name: l.Name, Qty: l.Qty, Options: labels, UnitPriceCents: unit}
		if l.FiredAt != nil && (lastFired == nil || l.FiredAt.After(*lastFired)) {
			lastFired = l.FiredAt
		}
	}
	placed := hhmm(t.CreatedAt)
	var fired *string
	if lastFired != nil {
		f := hhmm(*lastFired)
		fired = &f
	}
	var note *string
	if t.Note != "" {
		note = &t.Note
	}
	return posTicketView{
		ID: t.ID, Lines: lines, Note: note,
		Source: "at the till · " + placed, PlacedAt: &placed, FiredAt: fired,
		TableID: t.TableID, ShiftID: t.ShiftID, Status: string(t.Status), TotalCents: t.TotalCents(),
	}
}

type posTableView struct {
	ID     uuid.UUID      `json:"id"`
	Number string         `json:"number"`
	Covers *int           `json:"covers"` // not tracked in v1
	Ticket *posTicketView `json:"ticket"`
}

type posRequestView struct {
	ID             uuid.UUID `json:"id"`
	TableID        uuid.UUID `json:"table_id"`
	TableNumber    string    `json:"table_number"`
	Kind           string    `json:"kind"` // "waiter" | "bill"
	AskedAt        string    `json:"asked_at"`
	CreatedAt      int64     `json:"created_at"` // epoch ms
	OpenTotalCents *int      `json:"open_total_cents"`
}

type posMenuItemView struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	PriceCents int       `json:"price_cents"`
	Mods       []string  `json:"mods,omitempty"` // single-select option labels (e.g. doneness)
}

type posMenuCategoryView struct {
	ID    uuid.UUID         `json:"id"`
	Name  string            `json:"name"`
	Items []posMenuItemView `json:"items"`
}

// posMenu builds the add-line sheet: available items only, mods = the
// labels of the item's first single-select group (the prototype's
// doneness picker); multi-select groups still work by label via
// POST .../lines. POS takes orders from every menu — categories are the
// union of all menus, with the menu name prefixed on the category label
// when the restaurant has more than one menu ("Dinner · Starters").
func posMenu(menus []menudomain.Menu, cats []menudomain.Category, items []menudomain.MenuItem) []posMenuCategoryView {
	itemsByCat := map[uuid.UUID][]posMenuItemView{}
	for _, it := range items {
		if !it.Available {
			continue
		}
		var mods []string
		for _, g := range it.OptionGroups {
			if !g.Multi {
				for _, o := range g.Options {
					mods = append(mods, o.Label)
				}
				break // first single-select group only
			}
		}
		itemsByCat[it.CategoryID] = append(itemsByCat[it.CategoryID], posMenuItemView{
			ID: it.ID, Name: it.Name, PriceCents: it.PriceCents, Mods: mods,
		})
	}
	menuName := map[uuid.UUID]string{}
	for _, m := range menus {
		menuName[m.ID] = m.Name
	}
	out := []posMenuCategoryView{}
	// Iterate menus (default first) so categories group by menu order.
	for _, m := range menus {
		for _, c := range cats {
			if c.MenuID != m.ID {
				continue
			}
			its := itemsByCat[c.ID]
			if len(its) == 0 {
				continue
			}
			label := c.Name
			if len(menus) > 1 {
				label = m.Name + " · " + c.Name
			}
			out = append(out, posMenuCategoryView{ID: c.ID, Name: label, Items: its})
		}
	}
	return out
}

// --- Handlers ----------------------------------------------------------

func (h *handler) posState(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	st, err := h.Pos.State(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}
	rest, err := h.Platform.RestaurantPublic(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}
	cats, items, err := h.Menu.Menu(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}

	var shift *posShiftView
	if st.Shift != nil {
		v := h.toPosShiftView(r.Context(), *st.Shift, st.ShiftNumber, st.ShiftExpectedCents)
		shift = &v
	}

	tables := make([]posTableView, len(st.Tables))
	ticketTotalByTable := map[uuid.UUID]int{}
	for i, ts := range st.Tables {
		tv := posTableView{ID: ts.Table.ID, Number: ts.Table.Label}
		if ts.Ticket != nil {
			v := toPosTicketView(*ts.Ticket)
			tv.Ticket = &v
			ticketTotalByTable[ts.Table.ID] = v.TotalCents
		}
		tables[i] = tv
	}

	labelByTable := map[uuid.UUID]string{}
	for _, ts := range st.Tables {
		labelByTable[ts.Table.ID] = ts.Table.Label
	}
	requests := make([]posRequestView, len(st.Requests))
	for i, sr := range st.Requests {
		var openTotal *int
		if total, ok := ticketTotalByTable[sr.TableID]; ok {
			openTotal = &total
		}
		requests[i] = posRequestView{
			ID: sr.ID, TableID: sr.TableID, TableNumber: labelByTable[sr.TableID],
			Kind: requestType(sr.Kind), AskedAt: hhmm(sr.CreatedAt),
			CreatedAt: sr.CreatedAt.UnixMilli(), OpenTotalCents: openTotal,
		}
	}

	menus, err := h.MenuAdmin.Menus(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}
	menu := posMenu(menus, cats, items)

	// Tender methods for the ticket-close screen (§4 has no list-methods
	// endpoint; POS reads them off /pos/state).
	methods, err := h.Pos.PaymentMethods(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}
	paymentMethods := make([]map[string]any, 0, len(methods))
	for _, m := range methods {
		if !m.Active {
			continue
		}
		paymentMethods = append(paymentMethods, map[string]any{
			"id": m.ID, "code": m.Code, "name": m.Name, "payment_group": m.PaymentGroup,
		})
	}

	// ETag on the 5s poll: hash the body, If-None-Match hit → 304 and no
	// bytes. No caching layers — just a hash compare per request.
	body, err := json.Marshal(map[string]any{
		"restaurant":       map[string]any{"id": rest.ID, "slug": rest.Slug, "name": rest.Name},
		"till":             1,
		"cashier":          displayName(u.Email),
		"shift":            shift,
		"other_till_shift": nil,
		"tables":           tables,
		"requests":         requests,
		"menu":             menu,
		"payment_methods":  paymentMethods,
	})
	if writeAppErr(w, err) {
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func (h *handler) posOpenShift(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	var req struct {
		OpeningFloatCents int `json:"opening_float_cents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sh, err := h.Pos.OpenShift(r.Context(), restaurantID, u.ID, displayName(u.Email), req.OpeningFloatCents)
	if writeAppErr(w, err) {
		return
	}
	number, err := h.Pos.ShiftNumber(r.Context(), restaurantID, sh.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, h.toPosShiftView(r.Context(), sh, number, sh.OpeningFloatCents))
}

// posCloseShift atomically closes the shift and builds the draft
// acceptance journal (contract §4).
func (h *handler) posCloseShift(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		DeclaredCents int `json:"declared_cents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sh, docID, err := h.Pos.CloseShift(r.Context(), restaurantID, id, u.ID, req.DeclaredCents)
	if writeAppErr(w, err) {
		return
	}
	number, err := h.Pos.ShiftNumber(r.Context(), restaurantID, sh.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shift": map[string]any{
		"id":                  sh.ID,
		"number":              fmt.Sprintf("shift-%d", number),
		"state":               sh.State(),
		"expected_cents":      *sh.ExpectedCents,
		"declared_cents":      *sh.DeclaredCents,
		"variance_cents":      *sh.VarianceCents,
		"closed_at":           hhmm(*sh.ClosedAt),
		"journal_document_id": docID,
	}})
}

// posCloseTicket records tenders and closes a ticket (contract §4).
func (h *handler) posCloseTicket(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		Payments []struct {
			MethodID    uuid.UUID `json:"method_id"`
			AmountCents int       `json:"amount_cents"`
			TipCents    int       `json:"tip_cents"`
		} `json:"payments"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	tenders := make([]posdomain.Tender, len(req.Payments))
	for i, p := range req.Payments {
		tenders[i] = posdomain.Tender{MethodID: p.MethodID, AmountCents: p.AmountCents, TipCents: p.TipCents}
	}
	t, err := h.Pos.CloseTicket(r.Context(), restaurantID, id, u.ID, tenders)
	if writeAppErr(w, err) {
		return
	}
	// tenders now carry resolved payment groups (mutated in place).
	payments := make([]map[string]any, len(tenders))
	for i, td := range tenders {
		payments[i] = map[string]any{
			"method_id": td.MethodID, "payment_group": td.PaymentGroup,
			"amount_cents": td.AmountCents, "tip_cents": td.TipCents,
		}
	}
	closedAt := ""
	if t.ClosedAt != nil {
		closedAt = hhmm(*t.ClosedAt)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": map[string]any{
		"id": t.ID, "status": t.Status, "closed_at": closedAt,
		"total_cents": t.TotalCents(), "payments": payments,
	}})
}

// posCashOperation records a pay-in/out/drop (contract §4).
func (h *handler) posCashOperation(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		Kind        string `json:"kind"`
		AmountCents int    `json:"amount_cents"`
		Reason      string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	op, err := h.Pos.RecordCashOperation(r.Context(), restaurantID, id, u.ID, req.Kind, req.AmountCents, req.Reason)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"cash_operation": map[string]any{
		"id": op.ID, "kind": op.Kind, "amount_cents": op.AmountCents,
		"reason": op.Reason, "recorded_at": op.RecordedAt,
	}})
}

// posZReport is the cashier's shift breakdown (contract §4).
func (h *handler) posZReport(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	z, err := h.Pos.ZReport(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	tenders := make([]map[string]any, len(z.Tenders))
	for i, g := range z.Tenders {
		tenders[i] = map[string]any{"payment_group": g.Group, "amount_cents": g.AmountCents, "tip_cents": g.TipCents}
	}
	ops := make([]map[string]any, len(z.CashOps))
	for i, op := range z.CashOps {
		ops[i] = map[string]any{"kind": op.Kind, "amount_cents": op.AmountCents}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"opening_float_cents": z.OpeningFloatCents,
		"tenders":             tenders,
		"cash_operations":     ops,
		"expected_cash_cents": z.ExpectedCashCents,
		"declared_cents":      z.DeclaredCents,
		"variance_cents":      z.VarianceCents,
		"state":               z.State,
	})
}

func (h *handler) posAddLines(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	tableID, err := uuid.Parse(r.PathValue("tableID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		Lines []struct {
			MenuItemID uuid.UUID   `json:"menu_item_id"`
			OptionIDs  []uuid.UUID `json:"option_ids"`
			Options    []string    `json:"options"` // labels, POS client shape
			Qty        int         `json:"qty"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	inputs := make([]posapp.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		inputs[i] = posapp.LineInput{MenuItemID: l.MenuItemID, OptionIDs: l.OptionIDs, OptionLabels: l.Options, Qty: l.Qty}
	}
	ticket, err := h.Pos.AddLines(r.Context(), restaurantID, tableID, inputs, "")
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toPosTicketView(ticket))
}

func (h *handler) posFire(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ticket, err := h.Pos.Fire(r.Context(), restaurantID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toPosTicketView(ticket))
}

func (h *handler) posAckRequest(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	h.posRequestAction(w, r, restaurantID, h.Pos.AckRequest)
}

func (h *handler) posDismissRequest(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	h.posRequestAction(w, r, restaurantID, h.Pos.DismissRequest)
}

func (h *handler) posRequestAction(w http.ResponseWriter, r *http.Request, restaurantID uuid.UUID, act func(ctx context.Context, restaurantID, id uuid.UUID) error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if writeAppErr(w, act(r.Context(), restaurantID, id)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
