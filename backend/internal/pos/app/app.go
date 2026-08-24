// Package app is the POS context's use-case layer: shift lifecycle,
// per-table tickets, firing, and the service-request inbox.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/pos"
	"aivo/internal/pos/ports"

	"uuid"
)

// ErrNoOpenShift is returned when an action needs an open shift.
var ErrNoOpenShift = errors.New("no open shift")

// ErrInvalid marks caller-fixable input problems (422).
var ErrInvalid = errors.New("invalid input")

// ErrShiftNotOpen is returned when a cash op / ticket close targets a
// shift that is not open (409).
var ErrShiftNotOpen = errors.New("shift is not open")

// ErrOpenTicketsUnpaid blocks a shift close while tickets with lines are
// still open and unpaid (409).
var ErrOpenTicketsUnpaid = errors.New("open tickets are unpaid")

type App struct {
	store  ports.Store
	menu   ports.Menu
	ledger ports.Ledger
}

func New(store ports.Store, menu ports.Menu, ledger ports.Ledger) *App {
	return &App{store: store, menu: menu, ledger: ledger}
}

// State is the POS floor view: the open shift (nil if none), every
// table with its open ticket (nil if none), and pending service
// requests. Polled by the POS app every ~5s.
type State struct {
	Shift *domain.Shift
	// ShiftNumber is the open shift's 1-based ordinal (display "shift-N");
	// ShiftExpectedCents its running expected cash (opening float + all
	// ticket totals so far). Both zero when Shift is nil.
	ShiftNumber        int
	ShiftExpectedCents int
	Tables             []TableState
	Requests           []menudomain.ServiceRequest
}

type TableState struct {
	Table  menudomain.Table
	Ticket *domain.Ticket
}

func (a *App) State(ctx context.Context, restaurantID uuid.UUID) (State, error) {
	st := State{}

	// Open tickets load once (two queries total) and serve both the
	// per-table map and the running expected total.
	openTickets, err := a.store.OpenTickets(ctx, restaurantID)
	if err != nil {
		return State{}, err
	}
	openByTable := map[uuid.UUID]*domain.Ticket{}
	for i := range openTickets {
		openByTable[openTickets[i].TableID] = &openTickets[i]
	}

	shift, err := a.store.OpenShiftFor(ctx, restaurantID)
	switch {
	case err == nil:
		st.Shift = &shift
		if st.ShiftNumber, err = a.store.ShiftSequence(ctx, restaurantID, shift.ID); err != nil {
			return State{}, err
		}
		// Expected cash in the drawer (new formula): float + cash tenders
		// taken so far + pay-ins − pay-outs − drops. Card/other tenders and
		// still-open (unpaid) tickets do not count.
		cs, err := a.cashSummaryOn(ctx, a.store, restaurantID, shift.ID, shift.OpeningFloatCents)
		if err != nil {
			return State{}, err
		}
		st.ShiftExpectedCents = cs.ExpectedCents
	case errors.Is(err, ports.ErrNotFound):
		// no open shift — still show tables/requests
	default:
		return State{}, err
	}

	tables, err := a.menu.Tables(ctx, restaurantID)
	if err != nil {
		return State{}, err
	}
	for _, t := range tables {
		st.Tables = append(st.Tables, TableState{Table: t, Ticket: openByTable[t.ID]})
	}

	st.Requests, err = a.menu.PendingServiceRequests(ctx, restaurantID)
	if err != nil {
		return State{}, err
	}
	return st, nil
}

// OpenShift opens a shift; cashier is the display name denormalized
// onto the row so pos state never needs a per-poll user lookup.
func (a *App) OpenShift(ctx context.Context, restaurantID, userID uuid.UUID, cashier string, openingFloatCents int) (domain.Shift, error) {
	if openingFloatCents < 0 {
		return domain.Shift{}, fmt.Errorf("%w: opening_float_cents must be >= 0", ErrInvalid)
	}
	sh := domain.Shift{
		ID:                uuid.New(),
		RestaurantID:      restaurantID,
		OpenedBy:          userID,
		Cashier:           cashier,
		OpeningFloatCents: openingFloatCents,
	}
	if err := a.store.OpenShift(ctx, sh); err != nil {
		return domain.Shift{}, err
	}
	return a.store.ShiftByID(ctx, restaurantID, sh.ID)
}

// cashSummary aggregates a shift's cash figures for the expected-cash
// formula and the Z-report.
type cashSummary struct {
	Tenders          []ports.TenderGroupTotal
	CashOps          []domain.CashOperation
	CashTendersCents int
	PayInCents       int
	PayOutCents      int
	DropCents        int
	ExpectedCents    int // float + cash tenders + pay-in − pay-out − drop
}

func (a *App) cashSummaryOn(ctx context.Context, st ports.Store, restaurantID, shiftID uuid.UUID, floatCents int) (cashSummary, error) {
	tenders, err := st.TendersForShift(ctx, restaurantID, shiftID)
	if err != nil {
		return cashSummary{}, err
	}
	cashOps, err := st.CashOperationsForShift(ctx, restaurantID, shiftID)
	if err != nil {
		return cashSummary{}, err
	}
	cs := cashSummary{Tenders: tenders, CashOps: cashOps}
	for _, g := range tenders {
		if g.Group == domain.PaymentGroupCash {
			cs.CashTendersCents += int(g.AmountCents)
		}
	}
	for _, op := range cashOps {
		switch op.Kind {
		case domain.CashPayIn:
			cs.PayInCents += op.AmountCents
		case domain.CashPayOut:
			cs.PayOutCents += op.AmountCents
		case domain.CashDrop:
			cs.DropCents += op.AmountCents
		}
	}
	cs.ExpectedCents = floatCents + cs.CashTendersCents + cs.PayInCents - cs.PayOutCents - cs.DropCents
	return cs, nil
}

// CloseShift closes the shift atomically (debt 1): lock the shift, block
// on open unpaid tickets (auto-closing empty ones as void), compute
// expected cash by the new formula, and build the draft acceptance
// journal — all in one transaction. Returns the closed shift and the
// draft journal id. Immutable once closed.
func (a *App) CloseShift(ctx context.Context, restaurantID, shiftID, closedBy uuid.UUID, declaredCents int) (domain.Shift, uuid.UUID, error) {
	var (
		out   domain.Shift
		docID uuid.UUID
	)
	err := a.store.InTx(ctx, func(tx *sql.Tx, st ports.Store) error {
		sh, err := st.LockShift(ctx, restaurantID, shiftID)
		if err != nil {
			return err
		}
		if !sh.Open() {
			return domain.ErrShiftClosed
		}

		// Open tickets with lines must be settled first; empty ones close
		// as void.
		tickets, err := st.TicketsForShift(ctx, restaurantID, shiftID)
		if err != nil {
			return err
		}
		for _, t := range tickets {
			if t.Status == domain.TicketOpen && len(t.Lines) > 0 {
				return ErrOpenTicketsUnpaid
			}
		}
		if err := st.CloseTickets(ctx, restaurantID, shiftID); err != nil {
			return err
		}

		cs, err := a.cashSummaryOn(ctx, st, restaurantID, shiftID, sh.OpeningFloatCents)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := sh.Close(declaredCents, cs.CashTendersCents, cs.PayInCents, cs.PayOutCents, cs.DropCents, now); err != nil {
			if errors.Is(err, domain.ErrNegativeAmount) {
				return fmt.Errorf("%w: %v", ErrInvalid, err)
			}
			return err
		}
		if err := st.CloseShift(ctx, sh); err != nil {
			return err
		}

		draft := ports.ShiftJournalDraft{
			ShiftID:        shiftID,
			CreatedBy:      closedBy,
			AccountingDate: businessDate(now),
			Tenders:        cs.Tenders,
			VarianceCents:  int64(*sh.VarianceCents),
		}
		docID, err = a.ledger.BuildDraftShiftJournal(ctx, tx, restaurantID, draft)
		if err != nil {
			return err
		}
		out = sh
		return nil
	})
	return out, docID, err
}

// businessDate is the accounting date of a shift close: the UTC calendar
// date of the close, deterministic (refuted §15.4 — not the timestamp of
// the last ticket/payment).
func businessDate(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// CloseTicket records the tenders for a ticket and closes it. Σ tenders
// must equal the ticket total (unless a void closure). Requires an open
// shift.
func (a *App) CloseTicket(ctx context.Context, restaurantID, ticketID, closedBy uuid.UUID, tenders []domain.Tender) (domain.Ticket, error) {
	if _, err := a.store.OpenShiftFor(ctx, restaurantID); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return domain.Ticket{}, ErrShiftNotOpen
		}
		return domain.Ticket{}, err
	}
	t, err := a.store.TicketByID(ctx, restaurantID, ticketID)
	if err != nil {
		return domain.Ticket{}, err
	}
	// Resolve each tender's payment group from its method (snapshot).
	methods, err := a.store.PaymentMethods(ctx, restaurantID)
	if err != nil {
		return domain.Ticket{}, err
	}
	groupByMethod := map[uuid.UUID]string{}
	for _, m := range methods {
		groupByMethod[m.ID] = m.PaymentGroup
	}
	for i := range tenders {
		g, ok := groupByMethod[tenders[i].MethodID]
		if !ok {
			return domain.Ticket{}, fmt.Errorf("%w: unknown payment method", ErrInvalid)
		}
		tenders[i].PaymentGroup = g
	}
	if err := t.Close(tenders, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, domain.ErrTicketClosed):
			return domain.Ticket{}, err // 409 via ErrConflict mapping below
		case errors.Is(err, domain.ErrTendersMismatch), errors.Is(err, domain.ErrNegativeAmount):
			return domain.Ticket{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		default:
			return domain.Ticket{}, err
		}
	}
	if err := a.store.CloseTicketWithPayments(ctx, restaurantID, t, tenders, closedBy); err != nil {
		return domain.Ticket{}, err
	}
	return a.store.TicketByID(ctx, restaurantID, ticketID)
}

// RecordCashOperation records an in-shift pay-in/out/drop.
func (a *App) RecordCashOperation(ctx context.Context, restaurantID, shiftID, recordedBy uuid.UUID, kind string, amountCents int, reason string) (domain.CashOperation, error) {
	if amountCents <= 0 {
		return domain.CashOperation{}, fmt.Errorf("%w: amount_cents must be > 0", ErrInvalid)
	}
	if kind != domain.CashPayIn && kind != domain.CashPayOut && kind != domain.CashDrop {
		return domain.CashOperation{}, fmt.Errorf("%w: unknown kind %q", ErrInvalid, kind)
	}
	sh, err := a.store.ShiftByID(ctx, restaurantID, shiftID)
	if err != nil {
		return domain.CashOperation{}, err
	}
	if !sh.Open() {
		return domain.CashOperation{}, ErrShiftNotOpen
	}
	op := domain.CashOperation{
		ID: uuid.New(), ShiftID: shiftID, RestaurantID: restaurantID,
		Kind: kind, AmountCents: amountCents, Reason: reason, RecordedBy: recordedBy,
	}
	if err := a.store.RecordCashOperation(ctx, op); err != nil {
		return domain.CashOperation{}, err
	}
	return op, nil
}

// AcceptShift posts the shift's draft journal to the GL and marks the
// shift accepted, atomically (debt 1 / one live journal per shift). docID
// is the shift's live draft, resolved by the caller from the ledger.
func (a *App) AcceptShift(ctx context.Context, restaurantID, shiftID, docID, acceptedBy uuid.UUID) (domain.Shift, error) {
	var out domain.Shift
	err := a.store.InTx(ctx, func(tx *sql.Tx, st ports.Store) error {
		sh, err := st.LockShift(ctx, restaurantID, shiftID)
		if err != nil {
			return err
		}
		if err := sh.Accept(time.Now().UTC(), acceptedBy, docID); err != nil {
			return err // ErrShiftNotClosed / ErrAlreadyAccepted → mapped
		}
		if err := a.ledger.PostJournal(ctx, tx, restaurantID, docID); err != nil {
			return err
		}
		if err := st.AcceptShift(ctx, sh); err != nil {
			return err
		}
		out = sh
		return nil
	})
	return out, err
}

// ZReport is the cashier's shift breakdown (display).
type ZReport struct {
	OpeningFloatCents int
	Tenders           []ports.TenderGroupTotal
	CashOps           []domain.CashOperation
	ExpectedCashCents int
	DeclaredCents     int
	VarianceCents     int
	State             string
}

// ZReport builds the shift breakdown. For an open shift declared/variance
// are 0 (not yet counted); expected is the running figure.
func (a *App) ZReport(ctx context.Context, restaurantID, shiftID uuid.UUID) (ZReport, error) {
	sh, err := a.store.ShiftByID(ctx, restaurantID, shiftID)
	if err != nil {
		return ZReport{}, err
	}
	cs, err := a.cashSummaryOn(ctx, a.store, restaurantID, shiftID, sh.OpeningFloatCents)
	if err != nil {
		return ZReport{}, err
	}
	z := ZReport{
		OpeningFloatCents: sh.OpeningFloatCents,
		Tenders:           cs.Tenders,
		CashOps:           cs.CashOps,
		ExpectedCashCents: cs.ExpectedCents,
		State:             sh.State(),
	}
	if sh.DeclaredCents != nil {
		z.DeclaredCents = *sh.DeclaredCents
	}
	if sh.VarianceCents != nil {
		z.VarianceCents = *sh.VarianceCents
	}
	return z, nil
}

// ShiftNumber is the shift's display ordinal ("shift-N").
func (a *App) ShiftNumber(ctx context.Context, restaurantID, shiftID uuid.UUID) (int, error) {
	return a.store.ShiftSequence(ctx, restaurantID, shiftID)
}

// Shift returns one shift, scoped to the restaurant.
func (a *App) Shift(ctx context.Context, restaurantID, shiftID uuid.UUID) (domain.Shift, error) {
	return a.store.ShiftByID(ctx, restaurantID, shiftID)
}

// ShiftsByState lists closed or accepted shifts (acceptance queue).
func (a *App) ShiftsByState(ctx context.Context, restaurantID uuid.UUID, state string) ([]domain.Shift, error) {
	if state != domain.ShiftClosed && state != domain.ShiftAccepted {
		return nil, fmt.Errorf("%w: state must be closed or accepted", ErrInvalid)
	}
	return a.store.ShiftsByState(ctx, restaurantID, state)
}

// LineInput is one line a waiter adds: item + chosen options + qty.
// Options may arrive as IDs or as labels (the POS client sends labels);
// labels resolve against the item's option groups.
type LineInput struct {
	MenuItemID   uuid.UUID
	OptionIDs    []uuid.UUID
	OptionLabels []string
	Qty          int
}

// AddLines appends snapshot lines to the table's open ticket, creating
// the ticket under the open shift if the table has none. Snapshots reuse
// the menu context's NewOrderLine validation (qty, availability, option
// ownership). A non-empty note (cart handoff) is appended to the
// ticket's note.
func (a *App) AddLines(ctx context.Context, restaurantID, tableID uuid.UUID, inputs []LineInput, note string) (domain.Ticket, error) {
	if len(inputs) == 0 {
		return domain.Ticket{}, fmt.Errorf("%w: at least one line is required", ErrInvalid)
	}

	// Table must exist under this restaurant (tenant scope).
	if _, err := a.menu.TableByID(ctx, restaurantID, tableID); err != nil {
		return domain.Ticket{}, err
	}

	shift, err := a.store.OpenShiftFor(ctx, restaurantID)
	if errors.Is(err, ports.ErrNotFound) {
		return domain.Ticket{}, ErrNoOpenShift
	}
	if err != nil {
		return domain.Ticket{}, err
	}

	lines := make([]domain.TicketLine, 0, len(inputs))
	for _, in := range inputs {
		item, err := a.menu.MenuItemByID(ctx, restaurantID, in.MenuItemID)
		if err != nil {
			return domain.Ticket{}, err
		}
		optionIDs := in.OptionIDs
		for _, label := range in.OptionLabels {
			id, ok := optionIDByLabel(item, label)
			if !ok {
				return domain.Ticket{}, fmt.Errorf("%w: unknown option %q", ErrInvalid, label)
			}
			optionIDs = append(optionIDs, id)
		}
		snap, err := menudomain.NewOrderLine(item, optionIDs, in.Qty)
		if err != nil {
			return domain.Ticket{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		opts := make([]domain.LineOption, len(snap.ChosenOptions))
		for i, o := range snap.ChosenOptions {
			opts[i] = domain.LineOption{Label: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		lines = append(lines, domain.TicketLine{
			ID:             uuid.New(),
			MenuItemID:     snap.MenuItemID,
			Name:           snap.Name,
			UnitPriceCents: snap.UnitPriceCents,
			Qty:            snap.Qty,
			Options:        opts,
		})
	}

	ticket, err := a.store.OpenTicketForTable(ctx, restaurantID, tableID)
	if errors.Is(err, ports.ErrNotFound) {
		ticket = domain.Ticket{ID: uuid.New(), RestaurantID: restaurantID, ShiftID: shift.ID, TableID: tableID}
		if err := a.store.CreateTicket(ctx, ticket); err != nil {
			return domain.Ticket{}, err
		}
	} else if err != nil {
		return domain.Ticket{}, err
	}

	if err := a.store.AddLines(ctx, ticket.ID, lines); err != nil {
		return domain.Ticket{}, err
	}
	if note != "" {
		if err := a.store.AppendTicketNote(ctx, restaurantID, ticket.ID, note); err != nil {
			return domain.Ticket{}, err
		}
	}
	return a.store.TicketByID(ctx, restaurantID, ticket.ID)
}

func optionIDByLabel(item menudomain.MenuItem, label string) (uuid.UUID, bool) {
	for _, g := range item.OptionGroups {
		for _, o := range g.Options {
			if o.Label == label {
				return o.ID, true
			}
		}
	}
	return uuid.Nil(), false
}

// Fire stamps fired_at on every unfired line of the ticket. Idempotent.
func (a *App) Fire(ctx context.Context, restaurantID, ticketID uuid.UUID) (domain.Ticket, error) {
	if _, err := a.store.TicketByID(ctx, restaurantID, ticketID); err != nil {
		return domain.Ticket{}, err
	}
	if err := a.store.FireTicket(ctx, restaurantID, ticketID); err != nil {
		return domain.Ticket{}, err
	}
	return a.store.TicketByID(ctx, restaurantID, ticketID)
}

// LinkTicketCustomer records the handoff customer on the ticket (first
// link wins) so CRM spend can include handoff sales.
func (a *App) LinkTicketCustomer(ctx context.Context, restaurantID, ticketID, customerID uuid.UUID) error {
	return a.store.LinkTicketCustomer(ctx, restaurantID, ticketID, customerID)
}

func (a *App) AckRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return a.menu.AckServiceRequest(ctx, restaurantID, id)
}

func (a *App) DismissRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return a.menu.DismissServiceRequest(ctx, restaurantID, id)
}
