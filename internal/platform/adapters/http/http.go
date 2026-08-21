// Package http wires the platform's /api/v1 surface on the stdlib
// ServeMux: auth/org/subscription/restaurant admin endpoints, the POS
// endpoints (thin composition over the pos app — kept here so session
// auth and error mapping exist exactly once), and the token-scoped diner
// entry points. Shapes per docs/PLATFORM.md.
package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	menuapp "aivo/internal/menu/app"
	menudomain "aivo/internal/menu/domain"
	menuports "aivo/internal/menu/ports"
	"aivo/internal/platform/app"
	platformports "aivo/internal/platform/ports"
	posapp "aivo/internal/pos/app"
	posdomain "aivo/internal/pos/domain"
	posports "aivo/internal/pos/ports"
	"aivo/pkg/session"
)

// SessionCookie is the platform login cookie (HttpOnly, server-side
// sessions in Postgres).
const SessionCookie = "aivo_session"

// Deps is everything the API needs, wired by cmd/aivo-server.
type Deps struct {
	Platform  *app.App
	Pos       *posapp.App
	Menu      menuports.Store
	MenuAdmin menuports.AdminStore
	MenuApp   menuapp.Application
	Images    platformports.ImageStore // nil disables image upload (503)
	BaseURL   string                   // origin table links are built under
}

type handler struct {
	Deps
}

// NewMux builds the /api/v1 routes. Paths are absolute (the caller
// mounts this on the root mux).
func NewMux(d Deps) http.Handler {
	h := &handler{Deps: d}
	mux := http.NewServeMux()

	// Auth (no session required except logout/me).
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.HandleFunc("GET /api/v1/auth/me", h.auth(h.me))

	// Org + subscription (owner/manager).
	mux.HandleFunc("GET /api/v1/org", h.auth(h.getOrg))
	mux.HandleFunc("PATCH /api/v1/org", h.manage(h.patchOrg))
	mux.HandleFunc("GET /api/v1/org/subscription", h.auth(h.getSubscription))
	mux.HandleFunc("POST /api/v1/org/subscription", h.manage(h.changePlan))

	// Restaurants.
	mux.HandleFunc("GET /api/v1/restaurants", h.auth(h.listRestaurants))
	mux.HandleFunc("POST /api/v1/restaurants", h.manage(h.createRestaurant))
	mux.HandleFunc("GET /api/v1/restaurants/{id}", h.restaurant(false, h.getRestaurant))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}", h.restaurant(true, h.patchRestaurant))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/theme", h.restaurant(false, h.getTheme))
	mux.HandleFunc("PUT /api/v1/restaurants/{id}/theme", h.restaurant(true, h.putTheme))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/theme/generate", h.restaurant(true, h.generateTheme))

	// Menus (1..N per restaurant).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/menus", h.restaurant(false, h.listMenus))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/menus", h.restaurant(true, h.createMenu))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/menus/{menuID}", h.restaurant(true, h.updateMenu))
	mux.HandleFunc("DELETE /api/v1/restaurants/{id}/menus/{menuID}", h.restaurant(true, h.deleteMenu))

	// Menu content (categories, items).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/categories", h.restaurant(false, h.listCategories))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/categories", h.restaurant(true, h.createCategory))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/categories/{catID}", h.restaurant(true, h.updateCategory))
	mux.HandleFunc("DELETE /api/v1/restaurants/{id}/categories/{catID}", h.restaurant(true, h.deleteCategory))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/items", h.restaurant(false, h.listItems))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/items", h.restaurant(true, h.createItem))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/items/{itemID}", h.restaurant(true, h.updateItem))
	mux.HandleFunc("DELETE /api/v1/restaurants/{id}/items/{itemID}", h.restaurant(true, h.deleteItem))

	// Tables.
	mux.HandleFunc("GET /api/v1/restaurants/{id}/tables", h.restaurant(false, h.listTables))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/tables", h.restaurant(true, h.createTable))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/tables/{tableID}/regenerate", h.restaurant(true, h.regenerateTable))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/tables/{tableID}/qr", h.restaurant(false, h.tableQR))

	// Images + staff.
	mux.HandleFunc("POST /api/v1/restaurants/{id}/images", h.restaurant(true, h.uploadImage))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/staff", h.restaurant(false, h.listStaff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/staff", h.restaurant(true, h.addStaff))

	// POS (any authenticated role, scoped to the user's restaurant).
	mux.HandleFunc("GET /api/v1/pos/state", h.pos(h.posState))
	mux.HandleFunc("POST /api/v1/pos/shifts", h.pos(h.posOpenShift))
	mux.HandleFunc("POST /api/v1/pos/shifts/{id}/close", h.pos(h.posCloseShift))
	mux.HandleFunc("POST /api/v1/pos/tables/{tableID}/lines", h.pos(h.posAddLines))
	mux.HandleFunc("POST /api/v1/pos/tickets/{id}/fire", h.pos(h.posFire))
	mux.HandleFunc("POST /api/v1/pos/requests/{id}/ack", h.pos(h.posAckRequest))
	mux.HandleFunc("POST /api/v1/pos/requests/{id}/dismiss", h.pos(h.posDismissRequest))

	// Diner (table-token scoped, anonymous).
	mux.HandleFunc("GET /api/v1/m/{restaurantSlug}/{menuSlug}", h.dinerBrowseMenu)
	mux.HandleFunc("GET /api/v1/t/{token}", h.dinerEntry)
	mux.HandleFunc("POST /api/v1/t/{token}/orders", h.dinerOrder)
	mux.HandleFunc("POST /api/v1/t/{token}/requests", h.dinerRequest)

	return mux
}

// --- JSON + error helpers ----------------------------------------------

type apiError struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: msg}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", "invalid JSON body")
		return false
	}
	return true
}

// writeAppErr maps app/store errors to responses. Returns true if it
// wrote one.
func writeAppErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, app.ErrForbidden):
		writeErr(w, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, platformports.ErrNotFound),
		errors.Is(err, menuports.ErrNotFound),
		errors.Is(err, posports.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, platformports.ErrConflict), errors.Is(err, posports.ErrConflict),
		errors.Is(err, menuports.ErrConflict), errors.Is(err, posdomain.ErrShiftClosed):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, menudomain.ErrDefaultMenuDelete),
		errors.Is(err, menudomain.ErrLastMenuDelete),
		errors.Is(err, menudomain.ErrMenuNotEmpty):
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
	case errors.Is(err, menuports.ErrItemReferenced):
		writeErr(w, http.StatusConflict, "referenced", err.Error())
	case errors.Is(err, app.ErrInvalid), errors.Is(err, posapp.ErrInvalid),
		errors.Is(err, app.ErrPlanLimit),
		errors.Is(err, menudomain.ErrInvalidQty),
		errors.Is(err, menudomain.ErrItemUnavailable),
		errors.Is(err, menudomain.ErrUnknownOption),
		errors.Is(err, menuapp.ErrUnknownMenuItem):
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
	case errors.Is(err, app.ErrGeneratorUnavailable):
		writeErr(w, http.StatusServiceUnavailable, "generator_unconfigured", err.Error())
	case errors.Is(err, app.ErrNoDesignMD):
		writeErr(w, http.StatusConflict, "no_design_md", err.Error())
	case errors.Is(err, platformports.ErrThemeGeneration):
		log.Printf("api: %v", err)
		writeErr(w, http.StatusBadGateway, "generation_failed", "theme generation failed; try again")
	case errors.Is(err, posapp.ErrNoOpenShift):
		writeErr(w, http.StatusUnprocessableEntity, "no_open_shift", err.Error())
	case errors.Is(err, menuapp.ErrServiceRequestAlreadyOpen):
		// Duplicate open request for the table: 409 per the menu client
		// contract (it renders the existing open-request state).
		writeErr(w, http.StatusConflict, "already_open", err.Error())
	case errors.Is(err, menuapp.ErrOrderRateLimited):
		w.Header().Set("Retry-After", strconv.Itoa(session.OrderDebounceSeconds))
		writeJSON(w, http.StatusTooManyRequests, map[string]apiError{"error": {
			Code: "rate_limited", Message: err.Error(), RetryAfterSeconds: session.OrderDebounceSeconds,
		}})
	default:
		log.Printf("api: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal", "internal error")
	}
	return true
}
