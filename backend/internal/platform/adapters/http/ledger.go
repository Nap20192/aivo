package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ledgerdomain "aivo/internal/domain/ledger"
	"aivo/internal/domain/platform"
	posdomain "aivo/internal/domain/pos"
	ledgerapp "aivo/internal/ledger/app"

	"uuid"
)

// Shift acceptance + ledger back office (manager+, restaurant-scoped).
// Money is integer cents, ids are uuid strings (contract §4).

// --- account lookup helper ---------------------------------------------

type accountInfo struct{ code, name string }

func (h *handler) accountIndex(ctx context.Context, restaurantID uuid.UUID) (map[uuid.UUID]accountInfo, error) {
	accs, err := h.Ledger.Accounts(ctx, restaurantID)
	if err != nil {
		return nil, err
	}
	idx := map[uuid.UUID]accountInfo{}
	for _, a := range accs {
		idx[a.ID] = accountInfo{a.Code, a.Name}
	}
	return idx, nil
}

func balanced(d ledgerdomain.JournalDocument) bool { return d.Balanced() }

// --- shift acceptance --------------------------------------------------

func (h *handler) listShifts(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = posdomain.ShiftClosed
	}
	shifts, err := h.Pos.ShiftsByState(r.Context(), rest.ID, state)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(shifts))
	for i, s := range shifts {
		out[i] = h.shiftRowView(r.Context(), rest.ID, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// shiftRowView is the full ShiftRow the admin UI expects (contract §4 list
// shape): number as the display string "shift-N", cashier, timestamps, and
// the cash figures — so the acceptance header/summary never render NaN.
func (h *handler) shiftRowView(ctx context.Context, restaurantID uuid.UUID, s posdomain.Shift) map[string]any {
	number, _ := h.Pos.ShiftNumber(ctx, restaurantID, s.ID)
	return map[string]any{
		"id": s.ID, "number": fmt.Sprintf("shift-%d", number), "cashier": s.Cashier,
		"opened_at": s.OpenedAt, "closed_at": s.ClosedAt, "accepted_at": s.AcceptedAt,
		"state":          s.State(),
		"expected_cents": derefInt(s.ExpectedCents),
		"declared_cents": derefInt(s.DeclaredCents),
		"variance_cents": derefInt(s.VarianceCents),
	}
}

// acceptanceView is the draft-journal review payload (GET/PATCH share it).
// The shift is the full ShiftRow (M1) so the UI can show cashier + the
// declared/expected/variance the manager posts against.
func (h *handler) acceptanceView(ctx context.Context, restaurantID uuid.UUID, sh posdomain.Shift, doc ledgerdomain.JournalDocument) (map[string]any, error) {
	idx, err := h.accountIndex(ctx, restaurantID)
	if err != nil {
		return nil, err
	}
	lines := make([]map[string]any, len(doc.Lines))
	for i, l := range doc.Lines {
		info := idx[l.AccountID]
		lines[i] = map[string]any{
			"line_id": l.ID, "account_id": l.AccountID, "account_code": info.code,
			"account_name": info.name, "side": l.Side, "amount_cents": l.AmountCents,
			"cost_center_id": l.CostCenterID, "memo": l.Memo,
			"editable": doc.State == ledgerdomain.StateDraft,
		}
	}
	return map[string]any{
		"shift": h.shiftRowView(ctx, restaurantID, sh),
		"document": map[string]any{
			"id": doc.ID, "state": doc.State,
			"accounting_date": doc.AccountingDate.Format("2006-01-02"),
			"recorded_at":     doc.RecordedAt.Format(time.RFC3339),
			"lines":           lines,
		},
		"variance_cents": derefInt(sh.VarianceCents),
		"balanced":       balanced(doc),
	}, nil
}

func (h *handler) getAcceptance(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	shiftID, ok := pathUUID(w, r, "shift_id")
	if !ok {
		return
	}
	sh, err := h.Pos.Shift(r.Context(), rest.ID, shiftID)
	if writeAppErr(w, err) {
		return
	}
	doc, err := h.Ledger.LiveDocumentForShift(r.Context(), rest.ID, shiftID)
	if writeAppErr(w, err) {
		return
	}
	view, err := h.acceptanceView(r.Context(), rest.ID, sh, doc)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *handler) patchAcceptance(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	shiftID, ok := pathUUID(w, r, "shift_id")
	if !ok {
		return
	}
	var req struct {
		Lines []struct {
			LineID       uuid.UUID  `json:"line_id"`
			AccountID    *uuid.UUID `json:"account_id"`
			CostCenterID *uuid.UUID `json:"cost_center_id"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sh, err := h.Pos.Shift(r.Context(), rest.ID, shiftID)
	if writeAppErr(w, err) {
		return
	}
	doc, err := h.Ledger.LiveDocumentForShift(r.Context(), rest.ID, shiftID)
	if writeAppErr(w, err) {
		return
	}
	// Apply overrides onto the existing lines (side/amount unchanged).
	override := map[uuid.UUID]struct {
		acc *uuid.UUID
		cc  *uuid.UUID
	}{}
	for _, l := range req.Lines {
		override[l.LineID] = struct {
			acc *uuid.UUID
			cc  *uuid.UUID
		}{l.AccountID, l.CostCenterID}
	}
	lines := make([]ledgerdomain.JournalLine, len(doc.Lines))
	for i, l := range doc.Lines {
		if o, ok := override[l.ID]; ok {
			if o.acc != nil {
				l.AccountID = *o.acc
			}
			if o.cc != nil {
				l.CostCenterID = *o.cc
			}
		}
		lines[i] = l
	}
	if err := h.Ledger.OverrideDraftLines(r.Context(), rest.ID, doc.ID, lines); writeAppErr(w, err) {
		return
	}
	doc, err = h.Ledger.GetJournal(r.Context(), rest.ID, doc.ID)
	if writeAppErr(w, err) {
		return
	}
	view, err := h.acceptanceView(r.Context(), rest.ID, sh, doc)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *handler) postAccept(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	shiftID, ok := pathUUID(w, r, "shift_id")
	if !ok {
		return
	}
	doc, err := h.Ledger.LiveDocumentForShift(r.Context(), rest.ID, shiftID)
	if writeAppErr(w, err) {
		return
	}
	sh, err := h.Pos.AcceptShift(r.Context(), rest.ID, shiftID, doc.ID, u.ID)
	if writeAppErr(w, err) {
		return
	}
	posted, err := h.Ledger.GetJournal(r.Context(), rest.ID, doc.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shift":    map[string]any{"id": sh.ID, "state": sh.State(), "accepted_at": sh.AcceptedAt},
		"document": map[string]any{"id": posted.ID, "state": posted.State, "posted_at": posted.PostedAt},
	})
}

// --- ledger back office ------------------------------------------------

func (h *handler) ledgerAccounts(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	accs, err := h.Ledger.Accounts(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(accs))
	for i, a := range accs {
		out[i] = map[string]any{
			"id": a.ID, "code": a.Code, "name": a.Name,
			"type": a.Type, "normal_side": a.NormalSide, "postable": a.Postable,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) ledgerCostCenters(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	centers, err := h.Ledger.CostCenters(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(centers))
	for i, c := range centers {
		out[i] = map[string]any{"id": c.ID, "code": c.Code, "name": c.Name}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) getAccountMap(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	entries, err := h.Ledger.AccountMapGet(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		out[i] = map[string]any{"purpose": e.Purpose, "account_id": e.AccountID, "account_code": e.AccountCode}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) putAccountMap(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req struct {
		Map []struct {
			Purpose   string    `json:"purpose"`
			AccountID uuid.UUID `json:"account_id"`
		} `json:"map"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	entries := make([]ledgerapp.MapEntry, len(req.Map))
	for i, e := range req.Map {
		entries[i] = ledgerapp.MapEntry{Purpose: e.Purpose, AccountID: e.AccountID}
	}
	result, err := h.Ledger.AccountMapPut(r.Context(), rest.ID, entries)
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(result))
	for i, e := range result {
		out[i] = map[string]any{"purpose": e.Purpose, "account_id": e.AccountID, "account_code": e.AccountCode}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) listJournals(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	q := r.URL.Query()
	var account *uuid.UUID
	if a := q.Get("account"); a != "" {
		id, err := uuid.Parse(a)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", "bad account id")
			return
		}
		account = &id
	}
	journals, err := h.Ledger.GetJournals(r.Context(), rest.ID, q.Get("from"), account, q.Get("source"))
	if writeAppErr(w, err) {
		return
	}
	out := make([]map[string]any, len(journals))
	for i, j := range journals {
		out[i] = map[string]any{
			"id": j.ID, "kind": j.Kind, "state": j.State,
			"accounting_date": j.AccountingDate, "recorded_at": j.RecordedAt,
			"source_kind": j.SourceKind, "source_id": j.SourceID,
			"reversal_of": j.ReversalOf, "total_cents": j.TotalCents,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// journalDocView renders a full document with its lines (account codes).
func (h *handler) journalDocView(ctx context.Context, restaurantID uuid.UUID, d ledgerdomain.JournalDocument) (map[string]any, error) {
	idx, err := h.accountIndex(ctx, restaurantID)
	if err != nil {
		return nil, err
	}
	lines := make([]map[string]any, len(d.Lines))
	for i, l := range d.Lines {
		lines[i] = map[string]any{
			"account_id": l.AccountID, "account_code": idx[l.AccountID].code,
			"side": l.Side, "amount_cents": l.AmountCents,
			"cost_center_id": l.CostCenterID, "memo": l.Memo,
		}
	}
	return map[string]any{
		"id": d.ID, "kind": d.Kind, "state": d.State,
		"accounting_date": d.AccountingDate.Format("2006-01-02"),
		"recorded_at":     d.RecordedAt.Format(time.RFC3339),
		"posted_at":       d.PostedAt, "cancelled_at": d.CancelledAt,
		"reversal_of": d.ReversalOf, "lines": lines,
	}, nil
}

func (h *handler) getJournal(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	docID, ok := pathUUID(w, r, "docID")
	if !ok {
		return
	}
	doc, err := h.Ledger.GetJournal(r.Context(), rest.ID, docID)
	if writeAppErr(w, err) {
		return
	}
	view, err := h.journalDocView(r.Context(), rest.ID, doc)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *handler) postJournal(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req struct {
		AccountingDate string `json:"accounting_date"`
		Memo           string `json:"memo"`
		Lines          []struct {
			AccountID    uuid.UUID  `json:"account_id"`
			Side         string     `json:"side"`
			AmountCents  int64      `json:"amount_cents"`
			CostCenterID *uuid.UUID `json:"cost_center_id"`
			Memo         string     `json:"memo"`
		} `json:"lines"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	date, err := time.Parse("2006-01-02", req.AccountingDate)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "accounting_date must be YYYY-MM-DD")
		return
	}
	lines := make([]ledgerapp.ManualLine, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = ledgerapp.ManualLine{
			AccountID: l.AccountID, Side: l.Side, AmountCents: l.AmountCents,
			CostCenterID: l.CostCenterID, Memo: l.Memo,
		}
	}
	doc, err := h.Ledger.ManualJournal(r.Context(), ledgerapp.ManualJournalInput{
		RestaurantID: rest.ID, CreatedBy: u.ID, AccountingDate: date, Memo: req.Memo, Lines: lines,
	}, r.URL.Query().Get("post") == "1")
	if writeAppErr(w, err) {
		return
	}
	view, err := h.journalDocView(r.Context(), rest.ID, doc)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": view})
}

func (h *handler) cancelJournal(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	docID, ok := pathUUID(w, r, "docID")
	if !ok {
		return
	}
	reversalID, err := h.Ledger.CancelJournal(r.Context(), rest.ID, docID)
	if writeAppErr(w, err) {
		return
	}
	rev, err := h.Ledger.GetJournal(r.Context(), rest.ID, reversalID)
	if writeAppErr(w, err) {
		return
	}
	revView, err := h.journalDocView(r.Context(), rest.ID, rev)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reversal": revView,
		"original": map[string]any{"id": docID, "state": ledgerdomain.StateCancelled},
	})
}

// --- helpers -----------------------------------------------------------

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return uuid.Nil(), false
	}
	return id, true
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
