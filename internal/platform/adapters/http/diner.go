package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	menuapp "aivo/internal/menu/app"
	menudomain "aivo/internal/menu/domain"
	"aivo/pkg/session"

	"github.com/google/uuid"
)

// Diner entry points, table-token scoped and anonymous: the token IS the
// credential. Shapes match the menu stream's client types
// (web/menu/src/types.ts). Everything here resolves the token first and
// returns the same generic 404 whether the token is unknown or foreign.

type dinerHoursRow struct {
	Label string `json:"label"`
	Open  string `json:"open"`
	Close string `json:"close"`
}

type dinerRestaurantView struct {
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Tagline   *string         `json:"tagline"`
	Hours     []dinerHoursRow `json:"hours"`
	Address   *string         `json:"address"`
	MapURL    *string         `json:"map_url"`
	Phone     *string         `json:"phone"`
	Instagram *string         `json:"instagram"`
}

type dinerOptionView struct {
	ID              uuid.UUID `json:"id"`
	Label           string    `json:"label"`
	PriceDeltaCents int       `json:"price_delta_cents"`
}

type dinerOptionGroupView struct {
	ID      uuid.UUID         `json:"id"`
	Name    string            `json:"name"`
	Select  string            `json:"select"` // "single" | "multi"
	Options []dinerOptionView `json:"options"`
}

type dinerItemView struct {
	ID           uuid.UUID              `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	PriceCents   int                    `json:"price_cents"`
	ImageURL     *string                `json:"image_url"`
	Allergens    []string               `json:"allergens"`
	OptionGroups []dinerOptionGroupView `json:"option_groups"`
	Available    bool                   `json:"available"`
	SoldOutAt    *string                `json:"sold_out_at"`
}

type dinerCategoryView struct {
	ID    uuid.UUID       `json:"id"`
	Name  string          `json:"name"`
	Items []dinerItemView `json:"items"`
}

type openRequestView struct {
	Type      string `json:"type"` // "waiter" | "bill"
	CreatedAt string `json:"created_at"`
}

func requestType(kind menudomain.ServiceRequestKind) string {
	if kind == menudomain.RequestBill {
		return "bill"
	}
	return "waiter"
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// dinerTheme flattens the stored theme JSON to the client's Theme shape,
// filling defaults (brand name, accent, bold) for keys the restaurant
// hasn't customized.
func dinerTheme(themeJSON []byte, restaurantName string) map[string]any {
	theme := map[string]any{}
	json.Unmarshal(themeJSON, &theme) // stored value is always valid JSON; {} on empty
	if _, ok := theme["brand_name"]; !ok {
		theme["brand_name"] = restaurantName
	}
	if _, ok := theme["accent"]; !ok {
		theme["accent"] = "Blood red"
	}
	if _, ok := theme["bold"]; !ok {
		theme["bold"] = false
	}
	return theme
}

// resolveDinerTable maps the {token} path segment to its table and
// restaurant, or writes a generic 404.
func (h *handler) resolveDinerTable(w http.ResponseWriter, r *http.Request) (menudomain.Table, bool) {
	table, err := h.MenuAdmin.TableByTokenGlobal(r.Context(), r.PathValue("token"))
	if err != nil {
		writeAppErr(w, err)
		return menudomain.Table{}, false
	}
	return table, true
}

// GET /api/v1/t/{token} — the whole table session in one shot.
func (h *handler) dinerEntry(w http.ResponseWriter, r *http.Request) {
	table, ok := h.resolveDinerTable(w, r)
	if !ok {
		return
	}
	rest, err := h.Platform.RestaurantPublic(r.Context(), table.RestaurantID)
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
	open, err := h.MenuAdmin.PendingServiceRequestsForTable(r.Context(), rest.ID, table.ID)
	if writeAppErr(w, err) {
		return
	}

	menus, err := h.MenuAdmin.Menus(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}

	// Anonymous diner session for order rate limiting, success path only.
	session.IssueOrRefresh(w, r)

	openViews := make([]openRequestView, len(open))
	for i, sr := range open {
		openViews[i] = openRequestView{Type: requestType(sr.Kind), CreatedAt: sr.CreatedAt.Format(time.RFC3339)}
	}

	hours := make([]dinerHoursRow, len(rest.Hours))
	for i, hr := range rest.Hours {
		hours[i] = dinerHoursRow{Label: hr.Label, Open: hr.Open, Close: hr.Close}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"restaurant": dinerRestaurantView{
			Name: rest.Name, Slug: rest.Slug,
			Hours:   hours,
			Address: optStr(rest.Address),
			MapURL:  optStr(rest.Contacts["map_url"]),
			Phone:   optStr(rest.Contacts["phone"]), Instagram: optStr(rest.Contacts["instagram"]),
		},
		"table":         map[string]any{"id": table.ID, "label": table.Label},
		"theme":         dinerTheme(theme.ThemeJSON, rest.Name),
		"menus":         buildDinerMenus(menus, cats, items),
		"open_requests": openViews,
	})
}

type dinerMenuView struct {
	ID         uuid.UUID           `json:"id"`
	Slug       string              `json:"slug"`
	Name       string              `json:"name"`
	IsDefault  bool                `json:"is_default"`
	Categories []dinerCategoryView `json:"categories"`
}

// buildDinerMenus groups categories (with their items) under menus,
// ordered as the store returns them (default first, then position).
func buildDinerMenus(menus []menudomain.Menu, cats []menudomain.Category, items []menudomain.MenuItem) []dinerMenuView {
	itemsByCat := map[uuid.UUID][]dinerItemView{}
	for _, it := range items {
		itemsByCat[it.CategoryID] = append(itemsByCat[it.CategoryID], toDinerItemView(it))
	}
	catsByMenu := map[uuid.UUID][]dinerCategoryView{}
	for _, c := range cats {
		views := itemsByCat[c.ID]
		if views == nil {
			views = []dinerItemView{}
		}
		catsByMenu[c.MenuID] = append(catsByMenu[c.MenuID], dinerCategoryView{ID: c.ID, Name: c.Name, Items: views})
	}
	out := make([]dinerMenuView, len(menus))
	for i, m := range menus {
		categories := catsByMenu[m.ID]
		if categories == nil {
			categories = []dinerCategoryView{}
		}
		out[i] = dinerMenuView{ID: m.ID, Slug: m.Slug, Name: m.Name, IsDefault: m.IsDefault, Categories: categories}
	}
	return out
}

func toDinerItemView(it menudomain.MenuItem) dinerItemView {
	groups := make([]dinerOptionGroupView, len(it.OptionGroups))
	for i, g := range it.OptionGroups {
		sel := "single"
		if g.Multi {
			sel = "multi"
		}
		opts := make([]dinerOptionView, len(g.Options))
		for j, o := range g.Options {
			opts[j] = dinerOptionView{ID: o.ID, Label: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		groups[i] = dinerOptionGroupView{ID: g.ID, Name: g.Name, Select: sel, Options: opts}
	}
	allergens := make([]string, len(it.Allergens))
	for i, a := range it.Allergens {
		allergens[i] = string(a)
	}
	return dinerItemView{
		ID: it.ID, Name: it.Name, Description: it.Description, PriceCents: it.PriceCents,
		ImageURL: optStr(it.ImageURL), Allergens: allergens, OptionGroups: groups,
		Available: it.Available, SoldOutAt: nil, // 86-time not tracked yet
	}
}

// GET /api/v1/m/{restaurantSlug}/{menuSlug} — public read-only browse of
// one menu: no table, no session cookie, no ordering/service data.
func (h *handler) dinerBrowseMenu(w http.ResponseWriter, r *http.Request) {
	rest, err := h.Menu.RestaurantBySlug(r.Context(), r.PathValue("restaurantSlug"))
	if writeAppErr(w, err) {
		return
	}
	menu, err := h.MenuAdmin.MenuBySlug(r.Context(), rest.ID, r.PathValue("menuSlug"))
	if writeAppErr(w, err) {
		return
	}
	platformRest, err := h.Platform.RestaurantPublic(r.Context(), rest.ID)
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

	views := buildDinerMenus([]menudomain.Menu{menu}, cats, items)
	hours := make([]dinerHoursRow, len(platformRest.Hours))
	for i, hr := range platformRest.Hours {
		hours[i] = dinerHoursRow{Label: hr.Label, Open: hr.Open, Close: hr.Close}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"restaurant": dinerRestaurantView{
			Name: platformRest.Name, Slug: platformRest.Slug,
			Hours:   hours,
			Address: optStr(platformRest.Address),
			MapURL:  optStr(platformRest.Contacts["map_url"]),
			Phone:   optStr(platformRest.Contacts["phone"]), Instagram: optStr(platformRest.Contacts["instagram"]),
		},
		"theme": dinerTheme(theme.ThemeJSON, platformRest.Name),
		"menu": map[string]any{
			"id": menu.ID, "slug": menu.Slug, "name": menu.Name,
			"categories": views[0].Categories,
		},
	})
}

// POST /api/v1/t/{token}/orders — body per web/menu/src/types.ts
// OrderInput; options come grouped, the flat option-ID list is what the
// menu app validates.
func (h *handler) dinerOrder(w http.ResponseWriter, r *http.Request) {
	table, ok := h.resolveDinerTable(w, r)
	if !ok {
		return
	}
	rest, err := h.MenuAdmin.RestaurantByID(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}

	var req struct {
		Lines []struct {
			MenuItemID uuid.UUID `json:"menu_item_id"`
			Qty        int       `json:"qty"`
			Options    []struct {
				GroupID   uuid.UUID   `json:"group_id"`
				OptionIDs []uuid.UUID `json:"option_ids"`
			} `json:"options"`
		} `json:"lines"`
		Note string `json:"note"`
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
		var optionIDs []uuid.UUID
		for _, g := range l.Options {
			optionIDs = append(optionIDs, g.OptionIDs...)
		}
		lines[i] = menuapp.OrderLineInput{MenuItemID: l.MenuItemID, OptionIDs: optionIDs, Qty: l.Qty}
	}

	// Logged-in customer? Link the order (anonymous stays fine).
	var customerID *uuid.UUID
	if customer := h.customerFromRequest(r); customer != nil {
		customerID = &customer.ID
	}

	_, err = h.MenuApp.Commands.SubmitOrder.Handle(r.Context(), menuapp.SubmitOrder{
		RestaurantSlug: rest.Slug,
		TableToken:     table.Token,
		SessionID:      sessionID,
		CustomerID:     customerID,
		Lines:          lines,
		Comment:        req.Note,
	})
	if writeAppErr(w, err) {
		return
	}
	if customerID != nil {
		if err := h.Platform.TouchGuest(r.Context(), rest.ID, *customerID); err != nil {
			log.Printf("api: touch guest: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/t/{token}/requests {type: waiter|bill} → {request}
func (h *handler) dinerRequest(w http.ResponseWriter, r *http.Request) {
	table, ok := h.resolveDinerTable(w, r)
	if !ok {
		return
	}
	rest, err := h.MenuAdmin.RestaurantByID(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}

	var req struct {
		Type string `json:"type"`
		Kind string `json:"kind"` // accepted alias
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Type == "" {
		req.Type = req.Kind
	}
	var kind menudomain.ServiceRequestKind
	switch req.Type {
	case "waiter", string(menudomain.CallWaiter):
		kind = menudomain.CallWaiter
	case "bill", string(menudomain.RequestBill):
		kind = menudomain.RequestBill
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "type must be waiter or bill")
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
	writeJSON(w, http.StatusCreated, map[string]any{
		"request": openRequestView{Type: requestType(sr.Kind), CreatedAt: sr.CreatedAt.Format(time.RFC3339)},
	})
}
