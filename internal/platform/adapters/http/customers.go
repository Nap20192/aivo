package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aivo/internal/platform/app"
	"aivo/internal/platform/domain"

	"github.com/google/uuid"
)

// CustomerCookie is the diner-account login cookie — deliberately a
// different name and session table than the staff SessionCookie: neither
// cookie ever resolves in the other's session store.
const CustomerCookie = "aivo_customer"

func setCustomerCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	setAuthCookie(w, CustomerCookie, token, ttl)
}

// customerFromRequest resolves the aivo_customer cookie to a customer,
// nil if absent/invalid (anonymous flow stays first-class).
func (h *handler) customerFromRequest(r *http.Request) *domain.Customer {
	c, err := r.Cookie(CustomerCookie)
	if err != nil {
		return nil
	}
	customer, err := h.Platform.CustomerByToken(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	return &customer
}

type customerView struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

func toCustomerView(c domain.Customer) customerView {
	return customerView{ID: c.ID, Email: c.Email, Name: c.Name, Phone: c.Phone}
}

type customerOrderLineView struct {
	Name           string   `json:"name"`
	Qty            int      `json:"qty"`
	UnitPriceCents int      `json:"unit_price_cents"`
	Options        []string `json:"options"`
}

func toCustomerOrderLineViews(lines []domain.CustomerOrderLine) []customerOrderLineView {
	out := make([]customerOrderLineView, len(lines))
	for i, l := range lines {
		opts := l.Options
		if opts == nil {
			opts = []string{}
		}
		out[i] = customerOrderLineView{Name: l.Name, Qty: l.Qty, UnitPriceCents: l.UnitPriceCents, Options: opts}
	}
	return out
}

// POST /api/v1/customer/register {email, password, name}
func (h *handler) customerRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	c, token, err := h.Platform.RegisterCustomer(r.Context(), req.Email, req.Password, req.Name)
	if writeAppErr(w, err) {
		return
	}
	setCustomerCookie(w, token, app.CustomerSessionTTL)
	writeJSON(w, http.StatusCreated, map[string]any{"customer": toCustomerView(c)})
}

// POST /api/v1/customer/login
func (h *handler) customerLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	c, token, err := h.Platform.LoginCustomer(r.Context(), req.Email, req.Password)
	if writeAppErr(w, err) {
		return
	}
	setCustomerCookie(w, token, app.CustomerSessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{"customer": toCustomerView(c)})
}

// POST /api/v1/customer/logout
func (h *handler) customerLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CustomerCookie); err == nil {
		if err := h.Platform.LogoutCustomer(r.Context(), c.Value); err != nil {
			writeAppErr(w, err)
			return
		}
	}
	setCustomerCookie(w, "", -time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/v1/customer/me → {customer, orders}
func (h *handler) customerMe(w http.ResponseWriter, r *http.Request) {
	customer := h.customerFromRequest(r)
	if customer == nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "customer login required")
		return
	}
	history, err := h.Platform.CustomerHistory(r.Context(), customer.ID)
	if writeAppErr(w, err) {
		return
	}
	type orderView struct {
		RestaurantName string                  `json:"restaurant_name"`
		CreatedAt      time.Time               `json:"created_at"`
		TotalCents     int                     `json:"total_cents"`
		Lines          []customerOrderLineView `json:"lines"`
	}
	orders := make([]orderView, len(history))
	for i, o := range history {
		orders[i] = orderView{RestaurantName: o.RestaurantName, CreatedAt: o.CreatedAt, TotalCents: o.TotalCents, Lines: toCustomerOrderLineViews(o.Lines)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"customer": toCustomerView(*customer), "orders": orders})
}

// --- CRM (manager+, restaurant-scoped) ---------------------------------

type guestSummaryView struct {
	Customer        customerView `json:"customer"`
	Visits          int          `json:"visits"`
	TotalSpentCents int          `json:"total_spent_cents"`
	LastSeen        time.Time    `json:"last_seen"`
	Tags            []string     `json:"tags"`
}

func toGuestSummaryView(g domain.GuestSummary) guestSummaryView {
	tags := g.Tags
	if tags == nil {
		tags = []string{}
	}
	return guestSummaryView{
		Customer: toCustomerView(g.Customer), Visits: g.Visits,
		TotalSpentCents: g.TotalSpentCents, LastSeen: g.LastSeen, Tags: tags,
	}
}

// GET /api/v1/restaurants/{id}/guests?query=&limit=
func (h *handler) listGuests(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	limit := 0
	if q := r.URL.Query().Get("limit"); q != "" {
		limit = atoiOr(q, 0)
	}
	guests, err := h.Platform.Guests(r.Context(), rest.ID, r.URL.Query().Get("query"), limit)
	if writeAppErr(w, err) {
		return
	}
	views := make([]guestSummaryView, len(guests))
	for i, g := range guests {
		views[i] = toGuestSummaryView(g)
	}
	writeJSON(w, http.StatusOK, views)
}

// crmLineView is the admin Guests screen's line shape.
type crmLineView struct {
	Name       string `json:"name"`
	Qty        int    `json:"qty"`
	TotalCents int    `json:"total_cents"`
}

func toCRMLineViews(lines []domain.CustomerOrderLine) []crmLineView {
	out := make([]crmLineView, len(lines))
	for i, l := range lines {
		out[i] = crmLineView{Name: l.Name, Qty: l.Qty, TotalCents: l.TotalCents}
	}
	return out
}

type guestOrderView struct {
	ID         uuid.UUID     `json:"id"`
	CreatedAt  time.Time     `json:"created_at"`
	TableLabel string        `json:"table_label"`
	TotalCents int           `json:"total_cents"`
	Lines      []crmLineView `json:"lines"`
}

func (h *handler) guestDetailResponse(p domain.GuestProfile, sum domain.GuestSummary, orders []domain.GuestOrder) map[string]any {
	orderViews := make([]guestOrderView, len(orders))
	for i, o := range orders {
		orderViews[i] = guestOrderView{ID: o.ID, CreatedAt: o.CreatedAt, TableLabel: o.TableLabel, TotalCents: o.TotalCents, Lines: toCRMLineViews(o.Lines)}
	}
	v := toGuestSummaryView(sum)
	return map[string]any{
		"customer": v.Customer, "visits": v.Visits, "total_spent_cents": v.TotalSpentCents,
		"first_seen": p.FirstSeen, "last_seen": p.LastSeen,
		"notes": p.Notes, "tags": v.Tags, "orders": orderViews,
	}
}

// GET /api/v1/restaurants/{id}/guests/{customerID}
func (h *handler) getGuest(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	customerID, err := uuid.Parse(r.PathValue("customerID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	p, sum, orders, err := h.Platform.GuestDetail(r.Context(), rest.ID, customerID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, h.guestDetailResponse(p, sum, orders))
}

// PATCH /api/v1/restaurants/{id}/guests/{customerID} {notes, tags}
func (h *handler) patchGuest(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	customerID, err := uuid.Parse(r.PathValue("customerID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		Notes *string   `json:"notes"`
		Tags  *[]string `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	// Partial patch: nil field keeps the current value.
	current, _, _, err := h.Platform.GuestDetail(r.Context(), rest.ID, customerID)
	if writeAppErr(w, err) {
		return
	}
	notes := current.Notes
	if req.Notes != nil {
		notes = *req.Notes
	}
	tags := current.Tags
	if req.Tags != nil {
		tags = *req.Tags
	}
	p, sum, err := h.Platform.UpdateGuest(r.Context(), rest.ID, customerID, notes, tags)
	if writeAppErr(w, err) {
		return
	}
	orders, err := h.Platform.GuestOrdersFor(r.Context(), rest.ID, customerID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, h.guestDetailResponse(p, sum, orders))
}

// sniffUpload reads the first 512 bytes and content-sniffs them
// (http.DetectContentType). The declared multipart type is never
// trusted: the sniffed type must be in the same class the caller
// expects, and the SNIFFED type is what gets stored — no text/html
// hosted on the public image origin.
func sniffUpload(f io.Reader, declared string) (io.Reader, string, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, "", err
	}
	sniffed := http.DetectContentType(buf[:n])
	reader := io.MultiReader(bytes.NewReader(buf[:n]), f)

	declaredClass := strings.SplitN(declared, "/", 2)[0]
	sniffedClass := strings.SplitN(sniffed, "/", 2)[0]
	// CSV/markdown sniff as text/plain — same class is the contract.
	if declaredClass != sniffedClass {
		return nil, "", fmt.Errorf("declared %s but content is %s", declared, sniffed)
	}
	return reader, sniffed, nil
}

func atoiOr(s string, def int) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return def
		}
	}
	return n
}
