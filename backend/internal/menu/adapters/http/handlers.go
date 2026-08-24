// Package http wires the Menu context's REST/JSON endpoints on the stdlib
// net/http.ServeMux (Go 1.22+ method+path patterns). It is a thin
// adapter: parse the request, build a command/query, call the app layer,
// translate the result/error to a response. Every tenant-lookup failure —
// unknown slug or a token that doesn't belong to that restaurant —
// collapses to the same generic 404 so a client can't tell which one was
// wrong; that scoping itself lives in the app layer, not here.
package http

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/app"
	"aivo/internal/menu/ports"
	"aivo/pkg/session"

	"uuid"
)

type handler struct {
	app app.Application
}

// NewMux builds the Menu API's routes and wraps them with the IP-level
// rate limit as global middleware. app is the fully-wired app layer (see
// internal/menu/app.NewApplication) — this adapter never talks to
// ports.Store or ports.Notifier directly.
func NewMux(application app.Application) http.Handler {
	h := &handler{app: application}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/landing/{slug}/{table_token}", h.landing)
	mux.HandleFunc("GET /api/menu/{slug}", h.menu)
	mux.HandleFunc("POST /api/orders", h.createOrder)
	mux.HandleFunc("POST /api/service-requests", h.createServiceRequest)
	mux.HandleFunc("GET /api/qr/{slug}/{table_token}", h.qr)

	return withIPLimit(mux)
}

func withIPLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !session.AllowIP(clientIP(r)) {
			writeErr(w, http.StatusTooManyRequests, "rate limited")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the direct socket peer, not X-Forwarded-For — spoofable
// without a trusted, configured proxy in front. ponytail: fine behind a
// single trusted reverse proxy that strips/sets it itself; add explicit
// proxy-header trust config if this ever sits behind an untrusted one.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeAppErr maps an app-layer (command/query) error to the right HTTP
// status. Returns true if it wrote a response (caller should stop).
func writeAppErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ports.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, app.ErrOrderRateLimited), errors.Is(err, app.ErrServiceRequestAlreadyOpen):
		writeErr(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, app.ErrUnknownMenuItem),
		errors.Is(err, domain.ErrInvalidQty),
		errors.Is(err, domain.ErrItemUnavailable),
		errors.Is(err, domain.ErrUnknownOption):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("http: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
	return true
}

// --- GET /api/landing/{slug}/{table_token} ------------------------------

type restaurantView struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type tableView struct {
	Label string `json:"label"`
	Token string `json:"token"`
}

type landingBlockView struct {
	ID       uuid.UUID               `json:"id"`
	Type     domain.LandingBlockType `json:"type"`
	Position int                     `json:"position"`
	Data     map[string]string       `json:"data"`
}

type landingResponse struct {
	Restaurant    restaurantView     `json:"restaurant"`
	Table         tableView          `json:"table"`
	LandingBlocks []landingBlockView `json:"landing_blocks"`
}

func (h *handler) landing(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Queries.GetLanding.Handle(r.Context(), app.GetLanding{
		RestaurantSlug: r.PathValue("slug"),
		TableToken:     r.PathValue("table_token"),
	})
	if writeAppErr(w, err) {
		return
	}

	// Issued only on the success path, same as before the refactor: a
	// diner scanning a bogus/foreign table link never gets a session
	// cookie.
	session.IssueOrRefresh(w, r)

	blockViews := make([]landingBlockView, len(result.LandingBlocks))
	for i, b := range result.LandingBlocks {
		blockViews[i] = landingBlockView{ID: b.ID, Type: b.Type, Position: b.Position, Data: b.Data}
	}

	writeJSON(w, http.StatusOK, landingResponse{
		Restaurant:    restaurantView{Slug: result.Restaurant.Slug, Name: result.Restaurant.Name},
		Table:         tableView{Label: result.Table.Label, Token: result.Table.Token},
		LandingBlocks: blockViews,
	})
}

// --- GET /api/menu/{slug} ------------------------------------------------

type optionView struct {
	ID              uuid.UUID `json:"id"`
	Label           string    `json:"label"`
	PriceDeltaCents int       `json:"price_delta_cents"`
}

type optionGroupView struct {
	ID      uuid.UUID    `json:"id"`
	Name    string       `json:"name"`
	Multi   bool         `json:"multi"`
	Options []optionView `json:"options"`
}

type menuItemView struct {
	ID           uuid.UUID         `json:"id"`
	CategoryID   uuid.UUID         `json:"category_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	PriceCents   int               `json:"price_cents"`
	ImageURL     string            `json:"image_url"`
	Allergens    []domain.Allergen `json:"allergens"`
	Available    bool              `json:"available"`
	OptionGroups []optionGroupView `json:"option_groups"`
}

type categoryView struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
}

type menuResponse struct {
	Categories []categoryView `json:"categories"`
	Items      []menuItemView `json:"items"`
}

func (h *handler) menu(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Queries.GetMenu.Handle(r.Context(), app.GetMenu{
		RestaurantSlug: r.PathValue("slug"),
	})
	if writeAppErr(w, err) {
		return
	}

	catViews := make([]categoryView, len(result.Categories))
	for i, c := range result.Categories {
		catViews[i] = categoryView{ID: c.ID, Name: c.Name, Position: c.Position}
	}

	itemViews := make([]menuItemView, len(result.Items))
	for i, it := range result.Items {
		itemViews[i] = toMenuItemView(it)
	}

	writeJSON(w, http.StatusOK, menuResponse{Categories: catViews, Items: itemViews})
}

func toMenuItemView(it domain.MenuItem) menuItemView {
	groups := make([]optionGroupView, len(it.OptionGroups))
	for i, g := range it.OptionGroups {
		opts := make([]optionView, len(g.Options))
		for j, o := range g.Options {
			opts[j] = optionView{ID: o.ID, Label: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		groups[i] = optionGroupView{ID: g.ID, Name: g.Name, Multi: g.Multi, Options: opts}
	}
	allergens := it.Allergens
	if allergens == nil {
		allergens = []domain.Allergen{}
	}
	return menuItemView{
		ID:           it.ID,
		CategoryID:   it.CategoryID,
		Name:         it.Name,
		Description:  it.Description,
		PriceCents:   it.PriceCents,
		ImageURL:     it.ImageURL,
		Allergens:    allergens,
		Available:    it.Available,
		OptionGroups: groups,
	}
}

// --- POST /api/orders ------------------------------------------------------

type orderLineRequest struct {
	MenuItemID uuid.UUID   `json:"menu_item_id"`
	OptionIDs  []uuid.UUID `json:"option_ids"`
	Qty        int         `json:"qty"`
}

type createOrderRequest struct {
	RestaurantSlug string             `json:"restaurant_slug"`
	TableToken     string             `json:"table_token"`
	Lines          []orderLineRequest `json:"lines"`
	Comment        string             `json:"comment"`
}

type orderLineOptionView struct {
	Label           string `json:"label"`
	PriceDeltaCents int    `json:"price_delta_cents"`
}

type orderLineView struct {
	MenuItemID     uuid.UUID             `json:"menu_item_id"`
	Name           string                `json:"name"`
	UnitPriceCents int                   `json:"unit_price_cents"`
	Qty            int                   `json:"qty"`
	ChosenOptions  []orderLineOptionView `json:"chosen_options"`
}

type orderView struct {
	ID        uuid.UUID       `json:"id"`
	TableID   uuid.UUID       `json:"table_id"`
	Lines     []orderLineView `json:"lines"`
	Comment   string          `json:"comment"`
	CreatedAt time.Time       `json:"created_at"`
}

func toOrderView(o domain.Order) orderView {
	lines := make([]orderLineView, len(o.Lines))
	for i, l := range o.Lines {
		opts := make([]orderLineOptionView, len(l.ChosenOptions))
		for j, opt := range l.ChosenOptions {
			opts[j] = orderLineOptionView{Label: opt.Label, PriceDeltaCents: opt.PriceDeltaCents}
		}
		lines[i] = orderLineView{
			MenuItemID:     l.MenuItemID,
			Name:           l.Name,
			UnitPriceCents: l.UnitPriceCents,
			Qty:            l.Qty,
			ChosenOptions:  opts,
		}
	}
	return orderView{ID: o.ID, TableID: o.TableID, Lines: lines, Comment: o.Comment, CreatedAt: o.CreatedAt}
}

func (h *handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RestaurantSlug == "" || req.TableToken == "" || len(req.Lines) == 0 {
		writeErr(w, http.StatusBadRequest, "restaurant_slug, table_token, and at least one line are required")
		return
	}

	// Issued unconditionally, before we know whether the table/rate-limit
	// checks (both now app-layer concerns) will pass. Diner sessions are
	// anonymous, non-sensitive rate-limit tokens (see CONTEXT.md "Diner
	// session"), not accounts, so handing one out for a request that goes
	// on to 404/429/400 is harmless — this is the one deliberate,
	// documented behavior nuance of moving table resolution out of this
	// adapter (see this package's move notes).
	sessionID := session.IssueOrRefresh(w, r)

	lines := make([]app.OrderLineInput, len(req.Lines))
	for i, lr := range req.Lines {
		lines[i] = app.OrderLineInput{MenuItemID: lr.MenuItemID, OptionIDs: lr.OptionIDs, Qty: lr.Qty}
	}

	order, err := h.app.Commands.SubmitOrder.Handle(r.Context(), app.SubmitOrder{
		RestaurantSlug: req.RestaurantSlug,
		TableToken:     req.TableToken,
		SessionID:      sessionID,
		Lines:          lines,
		Comment:        req.Comment,
	})
	if writeAppErr(w, err) {
		return
	}

	writeJSON(w, http.StatusCreated, toOrderView(order))
}

// --- POST /api/service-requests --------------------------------------------

type createServiceRequestRequest struct {
	RestaurantSlug string                    `json:"restaurant_slug"`
	TableToken     string                    `json:"table_token"`
	Kind           domain.ServiceRequestKind `json:"kind"`
}

type serviceRequestView struct {
	ID        uuid.UUID                 `json:"id"`
	TableID   uuid.UUID                 `json:"table_id"`
	Kind      domain.ServiceRequestKind `json:"kind"`
	Status    string                    `json:"status"`
	CreatedAt time.Time                 `json:"created_at"`
}

func (h *handler) createServiceRequest(w http.ResponseWriter, r *http.Request) {
	var req createServiceRequestRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Kind != domain.CallWaiter && req.Kind != domain.RequestBill {
		writeErr(w, http.StatusBadRequest, "kind must be call_waiter or request_bill")
		return
	}

	sr, err := h.app.Commands.SubmitServiceRequest.Handle(r.Context(), app.SubmitServiceRequest{
		RestaurantSlug: req.RestaurantSlug,
		TableToken:     req.TableToken,
		Kind:           req.Kind,
	})
	if writeAppErr(w, err) {
		return
	}

	writeJSON(w, http.StatusCreated, serviceRequestView{
		ID: sr.ID, TableID: sr.TableID, Kind: sr.Kind, Status: sr.Status, CreatedAt: sr.CreatedAt,
	})
}

// --- GET /api/qr/{slug}/{table_token} ---------------------------------------

func (h *handler) qr(w http.ResponseWriter, r *http.Request) {
	png, err := h.app.Queries.GetQR.Handle(r.Context(), app.GetQR{
		RestaurantSlug: r.PathValue("slug"),
		TableToken:     r.PathValue("table_token"),
	})
	if writeAppErr(w, err) {
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// --- JSON helpers ------------------------------------------------------

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB, guards against oversized bodies
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("http: encode response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
