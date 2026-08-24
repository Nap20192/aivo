// Package http wires the platform's /api/v1 surface on the stdlib
// ServeMux: auth/org/subscription/restaurant admin endpoints, the POS
// endpoints (thin composition over the pos app — kept here so session
// auth and error mapping exist exactly once), and the token-scoped diner
// entry points. Shapes per docs/PLATFORM.md.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	inventorydomain "aivo/internal/domain/inventory"
	ledgerdomain "aivo/internal/domain/ledger"
	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/platform"
	posdomain "aivo/internal/domain/pos"
	inventoryapp "aivo/internal/inventory/app"
	inventoryports "aivo/internal/inventory/ports"
	ledgerapp "aivo/internal/ledger/app"
	ledgerports "aivo/internal/ledger/ports"
	menuapp "aivo/internal/menu/app"
	menuports "aivo/internal/menu/ports"
	"aivo/internal/platform/app"
	platformports "aivo/internal/platform/ports"
	posapp "aivo/internal/pos/app"
	posports "aivo/internal/pos/ports"
	"aivo/pkg/session"

	"uuid"
)

// SessionCookie is the platform login cookie (HttpOnly, server-side
// sessions in Postgres).
const SessionCookie = "aivo_session"

// AssistantStore is the narrow slice of platform persistence the
// assistant chat handlers need (precedent: Deps.Menu/MenuAdmin) — no
// one-line pass-throughs on the App.
type AssistantStore interface {
	AssistantThread(ctx context.Context, restaurantID uuid.UUID) (uuid.UUID, error)
	CreateAssistantMessage(ctx context.Context, restaurantID uuid.UUID, m domain.AssistantMessage) error
	AssistantMessages(ctx context.Context, restaurantID uuid.UUID, limit int) ([]domain.AssistantMessage, error)
	AssistantMessageByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.AssistantMessage, error)
	SetAssistantMessageStatus(ctx context.Context, restaurantID, id uuid.UUID, status string) error
}

// Deps is everything the API needs, wired by cmd/aivo-server.
type Deps struct {
	Platform       *app.App
	Pos            *posapp.App
	Ledger         *ledgerapp.App
	Inventory      *inventoryapp.App
	Menu           menuports.Store
	MenuAdmin      menuports.AdminStore
	MenuApp        menuapp.Application
	Images         platformports.ImageStore // nil disables image upload (503)
	Assistant      platformports.Assistant  // nil disables the admin assistant (503)
	AssistantStore AssistantStore           // chat thread/message persistence
	// ImagePrefix is the public URL prefix of our image bucket
	// (e.g. "http://localhost:9000/aivo-menu-images/") — the only host
	// assistant-proposed image_url values may point at.
	ImagePrefix string
	BaseURL     string // origin table links are built under
	// POSLocation renders POS "HH:MM" display times (RESTAURANT_TZ);
	// nil = server-local.
	POSLocation *time.Location
}

type handler struct {
	Deps
}

// NewMux builds the /api/v1 routes. Paths are absolute (the caller
// mounts this on the root mux).
func NewMux(d Deps) http.Handler {
	h := &handler{Deps: d}
	if d.POSLocation != nil {
		posLocation = d.POSLocation
	}
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

	// Admin AI assistant (manager+).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/assistant/messages", h.restaurant(true, h.assistantHistory))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/assistant/messages", h.restaurant(true, h.assistantSend))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/assistant/messages/{msgID}/apply", h.restaurant(true, h.assistantApply))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/assistant/messages/{msgID}/discard", h.restaurant(true, h.assistantDiscard))

	// Shift acceptance (manager+, restaurant-scoped).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/shifts", h.restaurant(true, h.listShifts))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/shifts/{shift_id}/acceptance", h.restaurant(true, h.getAcceptance))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/shifts/{shift_id}/acceptance", h.restaurant(true, h.patchAcceptance))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/shifts/{shift_id}/accept", h.restaurant(true, h.postAccept))

	// Ledger back office (manager+, restaurant-scoped).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/ledger/accounts", h.restaurant(true, h.ledgerAccounts))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/ledger/cost-centers", h.restaurant(true, h.ledgerCostCenters))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/ledger/account-map", h.restaurant(true, h.getAccountMap))
	mux.HandleFunc("PUT /api/v1/restaurants/{id}/ledger/account-map", h.restaurant(true, h.putAccountMap))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/ledger/journals", h.restaurant(true, h.listJournals))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/ledger/journals", h.restaurant(true, h.postJournal))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/ledger/journals/{docID}", h.restaurant(true, h.getJournal))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/ledger/journals/{docID}/cancel", h.restaurant(true, h.cancelJournal))

	// Inventory (increment-2; manager+, restaurant-scoped — storekeeper role deferred).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products", h.restaurant(true, h.invListProducts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/products", h.restaurant(true, h.invCreateProduct))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}", h.restaurant(true, h.invGetProduct))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/products/{pid}", h.restaurant(true, h.invPatchProduct))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards", h.restaurant(true, h.invTechCards))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards/active", h.restaurant(true, h.invActiveTechCard))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards", h.restaurant(true, h.invCreateTechCard))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/tech-cards/{tcid}", h.restaurant(true, h.invGetTechCard))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/tech-cards/{tcid}/recost", h.restaurant(true, h.invRecost))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/suppliers", h.restaurant(true, h.invListSuppliers))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/suppliers", h.restaurant(true, h.invCreateSupplier))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/suppliers/{sid}", h.restaurant(true, h.invPatchSupplier))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/receipts", h.restaurant(true, h.invReceipts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts", h.restaurant(true, h.invCreateReceipt))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/receipts/{rid}", h.restaurant(true, h.invGetReceipt))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts/{rid}/post", h.restaurant(true, h.invPostReceipt))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts/{rid}/cancel", h.restaurant(true, h.invCancelReceipt))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/write-offs", h.restaurant(true, h.invWriteOffs))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs", h.restaurant(true, h.invCreateWriteOff))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/write-offs/{wid}", h.restaurant(true, h.invGetWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs/{wid}/post", h.restaurant(true, h.invPostWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs/{wid}/cancel", h.restaurant(true, h.invCancelWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes", h.restaurant(true, h.invCreateStocktake))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stocktakes", h.restaurant(true, h.invStocktakes))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stocktakes/{sid}", h.restaurant(true, h.invGetStocktake))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/stocktakes/{sid}", h.restaurant(true, h.invEnterCounts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/dry-run", h.restaurant(true, h.invDryRunStocktake))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/post", h.restaurant(true, h.invPostStocktake))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/cancel", h.restaurant(true, h.invCancelStocktake))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/on-hand", h.restaurant(true, h.invOnHand))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stock-moves", h.restaurant(true, h.invStockMoves))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/reports/food-cost", h.restaurant(true, h.invFoodCost))

	// POS (any authenticated role, scoped to the user's restaurant).
	mux.HandleFunc("GET /api/v1/pos/state", h.pos(h.posState))
	mux.HandleFunc("POST /api/v1/pos/shifts", h.pos(h.posOpenShift))
	mux.HandleFunc("POST /api/v1/pos/shifts/{id}/close", h.pos(h.posCloseShift))
	mux.HandleFunc("POST /api/v1/pos/shifts/{id}/cash-operations", h.pos(h.posCashOperation))
	mux.HandleFunc("GET /api/v1/pos/shifts/{id}/z-report", h.pos(h.posZReport))
	mux.HandleFunc("POST /api/v1/pos/tickets/{id}/close", h.pos(h.posCloseTicket))
	mux.HandleFunc("POST /api/v1/pos/tables/{tableID}/lines", h.pos(h.posAddLines))
	mux.HandleFunc("POST /api/v1/pos/tickets/{id}/fire", h.pos(h.posFire))
	mux.HandleFunc("POST /api/v1/pos/requests/{id}/ack", h.pos(h.posAckRequest))
	mux.HandleFunc("POST /api/v1/pos/requests/{id}/dismiss", h.pos(h.posDismissRequest))

	// Customer accounts (diner logins, separate cookie/session store).
	// Register/login run bcrypt — the per-IP limit matters most here.
	mux.HandleFunc("POST /api/v1/customer/register", public(h.customerRegister))
	mux.HandleFunc("POST /api/v1/customer/login", public(h.customerLogin))
	mux.HandleFunc("POST /api/v1/customer/logout", public(h.customerLogout))
	mux.HandleFunc("GET /api/v1/customer/me", public(h.customerMe))

	// CRM (manager+).
	mux.HandleFunc("GET /api/v1/restaurants/{id}/guests", h.restaurant(true, h.listGuests))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/guests/{customerID}", h.restaurant(true, h.getGuest))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/guests/{customerID}", h.restaurant(true, h.patchGuest))

	// POS handoff pickup — IP-limited on top of staff auth: 6-char codes
	// must not be brute-forceable even by a hostile staff session.
	mux.HandleFunc("GET /api/v1/pos/handoff/{code}", public(h.pos(h.posHandoffPreview)))
	mux.HandleFunc("POST /api/v1/pos/handoff/{code}/accept", public(h.pos(h.posHandoffAccept)))

	// Diner (table-token scoped, anonymous; per-IP limited).
	mux.HandleFunc("GET /api/v1/m/{restaurantSlug}/{menuSlug}", public(h.dinerBrowseMenu))
	mux.HandleFunc("GET /api/v1/t/{token}", public(h.dinerEntry))
	mux.HandleFunc("POST /api/v1/t/{token}/handoff", public(h.dinerHandoff))
	mux.HandleFunc("GET /api/v1/t/{token}/handoff/qr", public(h.dinerHandoffQR))
	mux.HandleFunc("POST /api/v1/t/{token}/orders", public(h.dinerOrder))
	mux.HandleFunc("POST /api/v1/t/{token}/requests", public(h.dinerRequest))

	return mux
}

// public wraps unauthenticated endpoints with the per-IP fixed-window
// rate limit (pkg/session.AllowIP, same mechanism the legacy diner API
// used) — the bcrypt auth endpoints and anonymous diner surface are the
// abuse targets.
func public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !session.AllowIP(clientIP(r)) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "rate limited")
			return
		}
		next(w, r)
	}
}

// clientIP is the direct socket peer, not X-Forwarded-For — spoofable
// without a trusted, configured proxy in front (same note as the legacy
// menu adapter).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
	case errors.Is(err, platformports.ErrAssistant):
		log.Printf("api: %v", err)
		writeErr(w, http.StatusBadGateway, "assistant_failed", "assistant call failed; try again")
	case errors.Is(err, domain.ErrInvalidAction):
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
	case errors.Is(err, posapp.ErrNoOpenShift):
		writeErr(w, http.StatusUnprocessableEntity, "no_open_shift", err.Error())
	// --- ledger / shift acceptance (increment-1) ---
	case errors.Is(err, ledgerports.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, posapp.ErrShiftNotOpen):
		writeErr(w, http.StatusConflict, "shift_not_open", err.Error())
	case errors.Is(err, posapp.ErrOpenTicketsUnpaid):
		writeErr(w, http.StatusConflict, "open_tickets_unpaid", err.Error())
	case errors.Is(err, posdomain.ErrTicketClosed):
		writeErr(w, http.StatusConflict, "ticket_closed", err.Error())
	case errors.Is(err, posdomain.ErrTendersMismatch):
		writeErr(w, http.StatusUnprocessableEntity, "tenders_mismatch", err.Error())
	case errors.Is(err, posdomain.ErrShiftNotClosed):
		writeErr(w, http.StatusConflict, "shift_not_closed", err.Error())
	case errors.Is(err, posdomain.ErrAlreadyAccepted):
		writeErr(w, http.StatusConflict, "already_accepted", err.Error())
	case errors.Is(err, ledgerdomain.ErrNotDraft):
		writeErr(w, http.StatusConflict, "document_posted", err.Error())
	case errors.Is(err, ledgerdomain.ErrAlreadyCancelled):
		writeErr(w, http.StatusConflict, "already_cancelled", err.Error())
	case errors.Is(err, ledgerdomain.ErrNotPosted):
		writeErr(w, http.StatusConflict, "not_posted", err.Error())
	case errors.Is(err, ledgerapp.ErrUnknownPurpose):
		writeErr(w, http.StatusUnprocessableEntity, "unknown_purpose", err.Error())
	case errors.Is(err, ledgerapp.ErrAccountNotPostable):
		writeErr(w, http.StatusUnprocessableEntity, "account_not_postable", err.Error())
	case errors.Is(err, ledgerdomain.ErrUnbalanced):
		writeErr(w, http.StatusUnprocessableEntity, "unbalanced", err.Error())
	case errors.Is(err, ledgerdomain.ErrInvalidSide):
		writeErr(w, http.StatusUnprocessableEntity, "line_side", err.Error())
	case errors.Is(err, ledgerdomain.ErrInvalidAmount), errors.Is(err, ledgerapp.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
	case errors.Is(err, ledgerports.ErrConflict):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
	// --- inventory (increment-2) ---
	case errors.Is(err, inventoryports.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, inventorydomain.ErrUnitIncompatible), errors.Is(err, inventorydomain.ErrInvalidUnit),
		errors.Is(err, inventorydomain.ErrNonBaseStockUnit):
		writeErr(w, http.StatusUnprocessableEntity, "unit_incompatible", err.Error())
	case errors.Is(err, inventorydomain.ErrRecipeCycle):
		writeErr(w, http.StatusUnprocessableEntity, "recipe_cycle", err.Error())
	case errors.Is(err, inventorydomain.ErrEmptyRecipe):
		writeErr(w, http.StatusUnprocessableEntity, "empty_recipe", err.Error())
	case errors.Is(err, inventorydomain.ErrDuplicateIngredient):
		writeErr(w, http.StatusUnprocessableEntity, "duplicate_ingredient", err.Error())
	case errors.Is(err, inventorydomain.ErrEmptyDocument):
		writeErr(w, http.StatusUnprocessableEntity, "empty_document", err.Error())
	case errors.Is(err, inventorydomain.ErrInvalidType), errors.Is(err, inventorydomain.ErrInvalidQty),
		errors.Is(err, inventorydomain.ErrMenuItemOnNonDish), errors.Is(err, inventorydomain.ErrInvalidConsumption),
		errors.Is(err, inventorydomain.ErrInvalidReason), errors.Is(err, inventorydomain.ErrBadInterval),
		errors.Is(err, inventoryapp.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
	case errors.Is(err, inventoryapp.ErrSKUTaken):
		writeErr(w, http.StatusUnprocessableEntity, "sku_taken", err.Error())
	case errors.Is(err, inventoryapp.ErrBackdated):
		writeErr(w, http.StatusUnprocessableEntity, "backdated_before_last_move", err.Error())
	case errors.Is(err, inventoryapp.ErrMenuItemTaken):
		writeErr(w, http.StatusConflict, "menu_item_taken", err.Error())
	case errors.Is(err, inventoryapp.ErrVersionExists):
		writeErr(w, http.StatusConflict, "version_exists", err.Error())
	case errors.Is(err, inventoryapp.ErrStocktakeOpen):
		writeErr(w, http.StatusConflict, "stocktake_open_exists", err.Error())
	case errors.Is(err, inventoryapp.ErrSupplierNameTaken):
		writeErr(w, http.StatusConflict, "supplier_name_taken", err.Error())
	case errors.Is(err, inventoryapp.ErrAlreadyPosted):
		writeErr(w, http.StatusConflict, "already_posted", err.Error())
	case errors.Is(err, inventoryapp.ErrAlreadyCancelled):
		writeErr(w, http.StatusConflict, "already_cancelled", err.Error())
	case errors.Is(err, inventoryapp.ErrNotDraft), errors.Is(err, inventoryapp.ErrNotPosted):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, inventoryports.ErrConflict):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
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
