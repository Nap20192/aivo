package http

import (
	"context"
	"net/http"
	"time"

	menudomain "aivo/internal/menu/domain"
	"aivo/internal/platform/domain"
	posapp "aivo/internal/pos/app"
	posdomain "aivo/internal/pos/domain"

	"github.com/google/uuid"
)

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

type shiftView struct {
	ID                uuid.UUID  `json:"id"`
	OpenedBy          uuid.UUID  `json:"opened_by"`
	OpenedAt          time.Time  `json:"opened_at"`
	OpeningFloatCents int        `json:"opening_float_cents"`
	ClosedAt          *time.Time `json:"closed_at"`
	DeclaredCents     *int       `json:"declared_cents"`
	ExpectedCents     *int       `json:"expected_cents"`
	VarianceCents     *int       `json:"variance_cents"`
}

func toShiftView(s posdomain.Shift) shiftView {
	return shiftView{
		ID: s.ID, OpenedBy: s.OpenedBy, OpenedAt: s.OpenedAt, OpeningFloatCents: s.OpeningFloatCents,
		ClosedAt: s.ClosedAt, DeclaredCents: s.DeclaredCents, ExpectedCents: s.ExpectedCents, VarianceCents: s.VarianceCents,
	}
}

type ticketLineView struct {
	ID             uuid.UUID              `json:"id"`
	MenuItemID     uuid.UUID              `json:"menu_item_id"`
	Name           string                 `json:"name"`
	UnitPriceCents int                    `json:"unit_price_cents"`
	Qty            int                    `json:"qty"`
	Options        []posdomain.LineOption `json:"options"`
	TotalCents     int                    `json:"total_cents"`
	FiredAt        *time.Time             `json:"fired_at"`
	CreatedAt      time.Time              `json:"created_at"`
}

type ticketView struct {
	ID         uuid.UUID        `json:"id"`
	TableID    uuid.UUID        `json:"table_id"`
	ShiftID    uuid.UUID        `json:"shift_id"`
	Status     string           `json:"status"`
	Lines      []ticketLineView `json:"lines"`
	TotalCents int              `json:"total_cents"`
	CreatedAt  time.Time        `json:"created_at"`
}

func toTicketView(t posdomain.Ticket) ticketView {
	lines := make([]ticketLineView, len(t.Lines))
	for i, l := range t.Lines {
		opts := l.Options
		if opts == nil {
			opts = []posdomain.LineOption{}
		}
		lines[i] = ticketLineView{
			ID: l.ID, MenuItemID: l.MenuItemID, Name: l.Name, UnitPriceCents: l.UnitPriceCents,
			Qty: l.Qty, Options: opts, TotalCents: l.TotalCents(), FiredAt: l.FiredAt, CreatedAt: l.CreatedAt,
		}
	}
	return ticketView{
		ID: t.ID, TableID: t.TableID, ShiftID: t.ShiftID, Status: t.Status,
		Lines: lines, TotalCents: t.TotalCents(), CreatedAt: t.CreatedAt,
	}
}

type requestView struct {
	ID        uuid.UUID `json:"id"`
	TableID   uuid.UUID `json:"table_id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func toRequestView(sr menudomain.ServiceRequest) requestView {
	return requestView{ID: sr.ID, TableID: sr.TableID, Kind: string(sr.Kind), Status: sr.Status, CreatedAt: sr.CreatedAt}
}

// --- Handlers ----------------------------------------------------------

func (h *handler) posState(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	st, err := h.Pos.State(r.Context(), restaurantID)
	if writeAppErr(w, err) {
		return
	}
	rest, err := h.Platform.Restaurant(r.Context(), u.OrgID, restaurantID)
	if writeAppErr(w, err) {
		return
	}

	var shift *shiftView
	if st.Shift != nil {
		v := toShiftView(*st.Shift)
		shift = &v
	}
	type tableStateView struct {
		Table  map[string]any `json:"table"`
		Ticket *ticketView    `json:"ticket"`
	}
	tables := make([]tableStateView, len(st.Tables))
	for i, ts := range st.Tables {
		tv := tableStateView{Table: map[string]any{"id": ts.Table.ID, "label": ts.Table.Label}}
		if ts.Ticket != nil {
			v := toTicketView(*ts.Ticket)
			tv.Ticket = &v
		}
		tables[i] = tv
	}
	requests := make([]requestView, len(st.Requests))
	for i, sr := range st.Requests {
		requests[i] = toRequestView(sr)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"restaurant": map[string]any{"id": rest.ID, "slug": rest.Slug, "name": rest.Name},
		"shift":      shift,
		"tables":     tables,
		"requests":   requests,
	})
}

func (h *handler) posOpenShift(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	var req struct {
		OpeningFloatCents int `json:"opening_float_cents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sh, err := h.Pos.OpenShift(r.Context(), restaurantID, u.ID, req.OpeningFloatCents)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toShiftView(sh))
}

func (h *handler) posCloseShift(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
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
	sh, err := h.Pos.CloseShift(r.Context(), restaurantID, id, req.DeclaredCents)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toShiftView(sh))
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
			Qty        int         `json:"qty"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	inputs := make([]posapp.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		inputs[i] = posapp.LineInput{MenuItemID: l.MenuItemID, OptionIDs: l.OptionIDs, Qty: l.Qty}
	}
	ticket, err := h.Pos.AddLines(r.Context(), restaurantID, tableID, inputs)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toTicketView(ticket))
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
	writeJSON(w, http.StatusOK, toTicketView(ticket))
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
