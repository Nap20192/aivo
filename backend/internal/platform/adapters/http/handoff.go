package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/platform"
	menuports "aivo/internal/menu/ports"
	posapp "aivo/internal/pos/app"
	"aivo/pkg/qrcode"
	"aivo/pkg/session"

	"github.com/google/uuid"
)

// Cart handoff: the diner stores their cart under a short pickup code
// and shows it (QR or 6 chars) to the waiter, who pulls it into the
// table ticket. Coexists with direct kitchen send.

// POST /api/v1/t/{token}/handoff {lines, note} → {code, qr_url, expires_at}
func (h *handler) dinerHandoff(w http.ResponseWriter, r *http.Request) {
	table, ok := h.resolveDinerTable(w, r)
	if !ok {
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
			OptionIDs []uuid.UUID `json:"option_ids"` // flat form accepted too
		} `json:"lines"`
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Lines) == 0 || len(req.Lines) > 50 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "between 1 and 50 lines required")
		return
	}
	if len(req.Note) > 500 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "note too long")
		return
	}

	session.IssueOrRefresh(w, r)

	// Snapshot-validate every line against the live menu (same rules as
	// order submit: qty, availability, option ownership). Validation
	// runs BEFORE the cooldown slot is consumed — a 422 (item just
	// 86'd) must not burn the diner's 30s window.
	_, items, err := h.Menu.Menu(r.Context(), table.RestaurantID)
	if writeAppErr(w, err) {
		return
	}
	itemsByID := map[uuid.UUID]menudomain.MenuItem{}
	for _, it := range items {
		itemsByID[it.ID] = it
	}
	lines := make([]menudomain.HandoffLine, 0, len(req.Lines))
	for _, lr := range req.Lines {
		item, ok := itemsByID[lr.MenuItemID]
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", "unknown menu_item_id")
			return
		}
		optionIDs := lr.OptionIDs
		for _, g := range lr.Options {
			optionIDs = append(optionIDs, g.OptionIDs...)
		}
		snap, err := menudomain.NewOrderLine(item, optionIDs, lr.Qty)
		if writeAppErr(w, err) {
			return
		}
		lines = append(lines, menudomain.HandoffLine{
			MenuItemID: snap.MenuItemID, OptionIDs: optionIDs, Qty: snap.Qty,
			Name: snap.Name, UnitPriceCents: snap.UnitPriceCents, Options: snap.ChosenOptions,
		})
	}

	// Cooldown keys on the table token (shared with order submit — a
	// handoff and an order trade off the same 30s slot; unforgeable
	// unlike the session cookie). Consumed only after validation passed.
	if !session.AllowOrder(table.Token) {
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusTooManyRequests, map[string]apiError{"error": {
			Code: "rate_limited", Message: "a cart just went in from this table", RetryAfterSeconds: session.OrderDebounceSeconds,
		}})
		return
	}

	var customerID *uuid.UUID
	if customer := h.customerFromRequest(r); customer != nil {
		customerID = &customer.ID
		if err := h.Platform.TouchGuest(r.Context(), table.RestaurantID, customer.ID); err != nil {
			slog.Warn("handoff: touch guest", "error", err)
		}
	}

	handoff := menudomain.Handoff{
		ID: uuid.New(), RestaurantID: table.RestaurantID, TableID: table.ID,
		CustomerID: customerID, Lines: lines, Note: req.Note,
		ExpiresAt: time.Now().UTC().Add(menudomain.HandoffTTL),
	}
	// Code collisions are ~1e-9 likely; retry a few times, then give up.
	for attempt := 0; ; attempt++ {
		handoff.Code, err = menudomain.NewHandoffCode()
		if writeAppErr(w, err) {
			return
		}
		err = h.MenuAdmin.CreateHandoff(r.Context(), handoff)
		if err == nil {
			break
		}
		if !errors.Is(err, menuports.ErrConflict) || attempt >= 3 {
			writeAppErr(w, err)
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"code":       handoff.Code,
		"qr_url":     "/api/v1/t/" + url.PathEscape(table.Token) + "/handoff/qr?code=" + handoff.Code,
		"expires_at": handoff.ExpiresAt,
	})
}

// GET /api/v1/t/{token}/handoff/qr?code=X → PNG
func (h *handler) dinerHandoffQR(w http.ResponseWriter, r *http.Request) {
	table, ok := h.resolveDinerTable(w, r)
	if !ok {
		return
	}
	code := strings.ToUpper(r.URL.Query().Get("code"))
	handoff, err := h.MenuAdmin.HandoffByCode(r.Context(), table.RestaurantID, code)
	if err != nil || handoff.TableID != table.ID {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	png, err := qrcode.PNG(handoff.Code, 512)
	if writeAppErr(w, err) {
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// --- POS side ----------------------------------------------------------

// handoffPreview matches the POS client's HandoffPreview type
// (frontend/pos/src/types.ts): flat table fields, lines in the ticket-line
// shape (labels, unit price inclusive of option deltas), note null when
// empty.
func (h *handler) handoffPreview(r *http.Request, restaurantID uuid.UUID, handoff menudomain.Handoff) (map[string]any, error) {
	table, err := h.MenuAdmin.TableByID(r.Context(), restaurantID, handoff.TableID)
	if err != nil {
		return nil, err
	}
	lines := make([]posLineView, len(handoff.Lines))
	for i, l := range handoff.Lines {
		unit := l.UnitPriceCents
		labels := []string{}
		for _, o := range l.Options {
			unit += o.PriceDeltaCents
			labels = append(labels, o.Label)
		}
		lines[i] = posLineView{ID: uuid.NewSHA1(handoff.ID, []byte(fmt.Sprint(i))), MenuItemID: l.MenuItemID, Name: l.Name, Qty: l.Qty, Options: labels, UnitPriceCents: unit}
	}
	var customerName *string
	if handoff.CustomerID != nil {
		// Waiters get the name only — never email/phone.
		if c, err := h.Platform.Customer(r.Context(), *handoff.CustomerID); err == nil {
			customerName = &c.Name
		}
	}
	var note *string
	if handoff.Note != "" {
		note = &handoff.Note
	}
	return map[string]any{
		"code":          handoff.Code,
		"table_id":      table.ID,
		"table_number":  table.Label,
		"customer_name": customerName,
		"note":          note,
		"lines":         lines,
		"total_cents":   handoff.TotalCents(),
		"expires_at":    handoff.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// GET /api/v1/pos/handoff/{code} — lookup is case-insensitive (codes
// are stored uppercase).
func (h *handler) posHandoffPreview(w http.ResponseWriter, r *http.Request, _ domain.User, restaurantID uuid.UUID) {
	handoff, err := h.MenuAdmin.HandoffByCode(r.Context(), restaurantID, strings.ToUpper(r.PathValue("code")))
	if writeAppErr(w, err) {
		return
	}
	view, err := h.handoffPreview(r, restaurantID, handoff)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// POST /api/v1/pos/handoff/{code}/accept {table_id?} → updated ticket
func (h *handler) posHandoffAccept(w http.ResponseWriter, r *http.Request, u domain.User, restaurantID uuid.UUID) {
	handoff, err := h.MenuAdmin.HandoffByCode(r.Context(), restaurantID, strings.ToUpper(r.PathValue("code")))
	if writeAppErr(w, err) {
		return
	}
	tableID := handoff.TableID
	var req struct {
		TableID *uuid.UUID `json:"table_id"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	if req.TableID != nil {
		tableID = *req.TableID
	}

	// Consume first (single-use, blocks a concurrent double accept),
	// compensate if appending fails. Compensation runs on a
	// non-cancelable context: the accept context being canceled is
	// exactly the situation where the unmark must still go through.
	// ponytail: one cross-store DB transaction would remove the
	// compensation entirely; do that if this two-step ever bites.
	if writeAppErr(w, h.MenuAdmin.MarkHandoffUsed(r.Context(), restaurantID, handoff.ID)) {
		return
	}
	inputs := make([]posapp.LineInput, len(handoff.Lines))
	for i, l := range handoff.Lines {
		inputs[i] = posapp.LineInput{MenuItemID: l.MenuItemID, OptionIDs: l.OptionIDs, Qty: l.Qty}
	}
	ticket, err := h.Pos.AddLines(r.Context(), restaurantID, tableID, inputs, handoff.Note)
	if err != nil {
		if unmarkErr := h.MenuAdmin.UnmarkHandoffUsed(context.WithoutCancel(r.Context()), handoff.ID); unmarkErr != nil {
			slog.Error("handoff accept: unmark after failure", "error", unmarkErr)
		}
		writeAppErr(w, err)
		return
	}
	if handoff.CustomerID != nil {
		// Link the customer on the ticket so CRM spend counts handoff
		// sales, and bump the guest profile.
		if err := h.Pos.LinkTicketCustomer(r.Context(), restaurantID, ticket.ID, *handoff.CustomerID); err != nil {
			slog.Warn("handoff accept: link ticket customer", "error", err)
		}
		if err := h.Platform.TouchGuest(r.Context(), restaurantID, *handoff.CustomerID); err != nil {
			slog.Warn("handoff accept: touch guest", "error", err)
		}
	}
	slog.Info("handoff accepted", "restaurant_id", restaurantID, "code", handoff.Code,
		"table_id", tableID, "by_user", u.ID, "lines", len(handoff.Lines))
	writeJSON(w, http.StatusOK, toPosTicketView(ticket))
}
