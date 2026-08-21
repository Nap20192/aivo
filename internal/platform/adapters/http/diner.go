package http

import (
	"encoding/json"
	"net/http"
	"time"

	menuapp "aivo/internal/menu/app"
	menudomain "aivo/internal/menu/domain"
	"aivo/pkg/session"

	"github.com/google/uuid"
)

// Diner entry points, table-token scoped and anonymous: the token IS the
// credential. Everything here resolves the token first and returns the
// same generic 404 whether the token is unknown or foreign.

// GET /api/v1/t/{token} — restaurant, table, theme, menu, in one shot.
func (h *handler) dinerEntry(w http.ResponseWriter, r *http.Request) {
	table, err := h.MenuAdmin.TableByTokenGlobal(r.Context(), r.PathValue("token"))
	if writeAppErr(w, err) {
		return
	}
	rest, err := h.MenuAdmin.RestaurantByID(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}
	theme, err := h.Platform.Theme(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	cats, items, err := h.Menu.Menu(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}

	// Anonymous diner session for order rate limiting, success path only.
	session.IssueOrRefresh(w, r)

	catViews := make([]categoryView, len(cats))
	for i, c := range cats {
		catViews[i] = categoryView{ID: c.ID, Name: c.Name, Position: c.Position}
	}
	itemViews := make([]itemView, 0, len(items))
	for _, it := range items {
		itemViews = append(itemViews, toItemView(it))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"restaurant": map[string]any{"slug": rest.Slug, "name": rest.Name},
		"table":      map[string]any{"id": table.ID, "label": table.Label, "token": table.Token},
		"theme":      map[string]any{"theme": json.RawMessage(theme.ThemeJSON), "design_md": theme.DesignMD},
		"menu":       map[string]any{"categories": catViews, "items": itemViews},
	})
}

// POST /api/v1/t/{token}/orders
func (h *handler) dinerOrder(w http.ResponseWriter, r *http.Request) {
	table, err := h.MenuAdmin.TableByTokenGlobal(r.Context(), r.PathValue("token"))
	if writeAppErr(w, err) {
		return
	}
	rest, err := h.MenuAdmin.RestaurantByID(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}

	var req struct {
		Lines []struct {
			MenuItemID uuid.UUID   `json:"menu_item_id"`
			OptionIDs  []uuid.UUID `json:"option_ids"`
			Qty        int         `json:"qty"`
		} `json:"lines"`
		Comment string `json:"comment"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Lines) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "at least one line is required")
		return
	}

	sessionID := session.IssueOrRefresh(w, r)
	lines := make([]menuapp.OrderLineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = menuapp.OrderLineInput{MenuItemID: l.MenuItemID, OptionIDs: l.OptionIDs, Qty: l.Qty}
	}

	order, err := h.MenuApp.Commands.SubmitOrder.Handle(r.Context(), menuapp.SubmitOrder{
		RestaurantSlug: rest.Slug,
		TableToken:     table.Token,
		SessionID:      sessionID,
		Lines:          lines,
		Comment:        req.Comment,
	})
	if writeAppErr(w, err) {
		return
	}

	type lineOptView struct {
		Label           string `json:"label"`
		PriceDeltaCents int    `json:"price_delta_cents"`
	}
	type lineView struct {
		MenuItemID     uuid.UUID     `json:"menu_item_id"`
		Name           string        `json:"name"`
		UnitPriceCents int           `json:"unit_price_cents"`
		Qty            int           `json:"qty"`
		ChosenOptions  []lineOptView `json:"chosen_options"`
	}
	lineViews := make([]lineView, len(order.Lines))
	for i, l := range order.Lines {
		opts := make([]lineOptView, len(l.ChosenOptions))
		for j, o := range l.ChosenOptions {
			opts[j] = lineOptView{Label: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		lineViews[i] = lineView{MenuItemID: l.MenuItemID, Name: l.Name, UnitPriceCents: l.UnitPriceCents, Qty: l.Qty, ChosenOptions: opts}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": order.ID, "table_id": order.TableID, "lines": lineViews,
		"comment": order.Comment, "created_at": order.CreatedAt.Format(time.RFC3339Nano),
	})
}

// POST /api/v1/t/{token}/requests {kind: waiter|bill}
func (h *handler) dinerRequest(w http.ResponseWriter, r *http.Request) {
	table, err := h.MenuAdmin.TableByTokenGlobal(r.Context(), r.PathValue("token"))
	if writeAppErr(w, err) {
		return
	}
	rest, err := h.MenuAdmin.RestaurantByID(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}

	var req struct {
		Kind string `json:"kind"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	// Accept both the contract's short names and the menu context's kinds.
	var kind menudomain.ServiceRequestKind
	switch req.Kind {
	case "waiter", string(menudomain.CallWaiter):
		kind = menudomain.CallWaiter
	case "bill", string(menudomain.RequestBill):
		kind = menudomain.RequestBill
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "kind must be waiter or bill")
		return
	}

	sr, err := h.MenuApp.Commands.SubmitServiceRequest.Handle(r.Context(), menuapp.SubmitServiceRequest{
		RestaurantSlug: rest.Slug,
		TableToken:     table.Token,
		Kind:           kind,
	})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toRequestView(sr))
}
