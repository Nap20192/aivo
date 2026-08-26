// Package http is cmd/aivo-inventory's own REST surface: the same
// endpoint shapes inventory served under cmd/aivo-server's /api/v1 mux
// (internal/platform/adapters/http, pre-split), now authenticated by a
// verified JWT instead of the platform session cookie (specs/inventory-service
// §"Inventory verifies a caller's token locally").
package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/adapters/jwtauth"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"

	"uuid"
)

type handler struct {
	Inventory *inventoryapp.App
	Verifier  jwtauth.Verifier
}

// NewMux builds inventory's REST API. Paths are absolute and identical to
// the ones it served under cmd/aivo-server, so the admin frontend's calls
// are unchanged apart from host:port.
func NewMux(app *inventoryapp.App, verifier jwtauth.Verifier) http.Handler {
	h := &handler{Inventory: app, Verifier: verifier}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products", h.restaurant(h.invListProducts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/products", h.restaurant(h.invCreateProduct))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}", h.restaurant(h.invGetProduct))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/products/{pid}", h.restaurant(h.invPatchProduct))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards", h.restaurant(h.invTechCards))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards/active", h.restaurant(h.invActiveTechCard))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/products/{pid}/tech-cards", h.restaurant(h.invCreateTechCard))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/tech-cards/{tcid}", h.restaurant(h.invGetTechCard))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/tech-cards/{tcid}/recost", h.restaurant(h.invRecost))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/suppliers", h.restaurant(h.invListSuppliers))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/suppliers", h.restaurant(h.invCreateSupplier))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/suppliers/{sid}", h.restaurant(h.invPatchSupplier))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/receipts", h.restaurant(h.invReceipts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts", h.restaurant(h.invCreateReceipt))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/receipts/{rid}", h.restaurant(h.invGetReceipt))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts/{rid}/post", h.restaurant(h.invPostReceipt))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/receipts/{rid}/cancel", h.restaurant(h.invCancelReceipt))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/write-offs", h.restaurant(h.invWriteOffs))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs", h.restaurant(h.invCreateWriteOff))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/write-offs/{wid}", h.restaurant(h.invGetWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs/{wid}/post", h.restaurant(h.invPostWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/write-offs/{wid}/cancel", h.restaurant(h.invCancelWriteOff))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes", h.restaurant(h.invCreateStocktake))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stocktakes", h.restaurant(h.invStocktakes))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stocktakes/{sid}", h.restaurant(h.invGetStocktake))
	mux.HandleFunc("PATCH /api/v1/restaurants/{id}/inventory/stocktakes/{sid}", h.restaurant(h.invEnterCounts))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/dry-run", h.restaurant(h.invDryRunStocktake))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/post", h.restaurant(h.invPostStocktake))
	mux.HandleFunc("POST /api/v1/restaurants/{id}/inventory/stocktakes/{sid}/cancel", h.restaurant(h.invCancelStocktake))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/on-hand", h.restaurant(h.invOnHand))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/stock-moves", h.restaurant(h.invStockMoves))
	mux.HandleFunc("GET /api/v1/restaurants/{id}/inventory/reports/food-cost", h.restaurant(h.invFoodCost))

	return mux
}

// restaurantFunc runs with a JWT-verified, tenant-checked caller —
// inventory's replacement for platform's session-cookie restaurantFunc.
type restaurantFunc func(w http.ResponseWriter, r *http.Request, claims jwtauth.Claims, restaurantID uuid.UUID)

var errUnauthorized = errors.New("inventory: unauthorized")

// restaurant verifies the request's bearer token, requires a manage role
// (owner/manager — every inventory route was manager+ under the old
// session middleware too; storekeeper role deferred), and checks the
// token's tenant claim against the {id} path segment before calling next
// (specs/inventory-service: reject a token scoped to a different tenant).
func (h *handler) restaurant(next restaurantFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := h.authenticate(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if !claims.HasRole("owner", "manager") {
			writeErr(w, http.StatusForbidden, "forbidden", "insufficient permissions")
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if claims.TenantID != id {
			writeErr(w, http.StatusForbidden, "forbidden", "insufficient permissions")
			return
		}
		next(w, r, claims, id)
	}
}

func (h *handler) authenticate(r *http.Request) (jwtauth.Claims, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return jwtauth.Claims{}, errUnauthorized
	}
	return h.Verifier.Verify(token)
}

// --- JSON + error helpers (small, inventory-only subset of
// internal/platform/adapters/http's — not shared across services, D4) ----

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("inventory api: encode response: %v", err)
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

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return uuid.Nil(), false
	}
	return id, true
}

// writeAppErr maps inventory app/domain/store errors to responses — the
// inventory-only subset of internal/platform/adapters/http's writeAppErr
// switch. Returns true if it wrote a response.
func writeAppErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ports.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, inv.ErrUnitIncompatible), errors.Is(err, inv.ErrInvalidUnit),
		errors.Is(err, inv.ErrNonBaseStockUnit):
		writeErr(w, http.StatusUnprocessableEntity, "unit_incompatible", err.Error())
	case errors.Is(err, inv.ErrRecipeCycle):
		writeErr(w, http.StatusUnprocessableEntity, "recipe_cycle", err.Error())
	case errors.Is(err, inv.ErrEmptyRecipe):
		writeErr(w, http.StatusUnprocessableEntity, "empty_recipe", err.Error())
	case errors.Is(err, inv.ErrDuplicateIngredient):
		writeErr(w, http.StatusUnprocessableEntity, "duplicate_ingredient", err.Error())
	case errors.Is(err, inv.ErrEmptyDocument):
		writeErr(w, http.StatusUnprocessableEntity, "empty_document", err.Error())
	case errors.Is(err, inv.ErrInvalidType), errors.Is(err, inv.ErrInvalidQty),
		errors.Is(err, inv.ErrMenuItemOnNonDish), errors.Is(err, inv.ErrInvalidConsumption),
		errors.Is(err, inv.ErrInvalidReason), errors.Is(err, inv.ErrBadInterval),
		errors.Is(err, inv.ErrInvalidFormat), errors.Is(err, inv.ErrInvalidYieldPct),
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
	case errors.Is(err, ports.ErrConflict):
		writeErr(w, http.StatusConflict, "conflict", err.Error())
	default:
		log.Printf("inventory api: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal", "internal error")
	}
	return true
}
