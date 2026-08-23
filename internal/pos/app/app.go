// Package app is the POS context's use-case layer: shift lifecycle,
// per-table tickets, firing, and the service-request inbox.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/pos"
	"aivo/internal/pos/ports"

	"github.com/google/uuid"
)

// ErrNoOpenShift is returned when an action needs an open shift.
var ErrNoOpenShift = errors.New("no open shift")

// ErrInvalid marks caller-fixable input problems (422).
var ErrInvalid = errors.New("invalid input")

type App struct {
	store ports.Store
	menu  ports.Menu
}

func New(store ports.Store, menu ports.Menu) *App {
	return &App{store: store, menu: menu}
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
		closedSales, err := a.store.ShiftClosedSalesCents(ctx, restaurantID, shift.ID)
		if err != nil {
			return State{}, err
		}
		st.ShiftExpectedCents = shift.OpeningFloatCents + closedSales
		for _, t := range openTickets {
			if t.ShiftID == shift.ID {
				st.ShiftExpectedCents += t.TotalCents()
			}
		}
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

// CloseShift posts the closing figures: expected cash = opening float +
// the shift's ticket totals (v1 treats all sales as cash — no payment
// methods yet), variance = declared - expected. Closing also closes any
// still-open tickets of the shift. Immutable once posted.
func (a *App) CloseShift(ctx context.Context, restaurantID, shiftID uuid.UUID, declaredCents int) (domain.Shift, error) {
	sh, err := a.store.ShiftByID(ctx, restaurantID, shiftID)
	if err != nil {
		return domain.Shift{}, err
	}

	tickets, err := a.store.TicketsForShift(ctx, restaurantID, shiftID)
	if err != nil {
		return domain.Shift{}, err
	}
	sales := 0
	for _, t := range tickets {
		sales += t.TotalCents()
	}

	if err := sh.Close(declaredCents, sales, time.Now().UTC()); err != nil {
		if errors.Is(err, domain.ErrNegativeAmount) {
			return domain.Shift{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return domain.Shift{}, err
	}
	if err := a.store.CloseShift(ctx, sh); err != nil {
		return domain.Shift{}, err
	}
	if err := a.store.CloseTickets(ctx, restaurantID, shiftID); err != nil {
		return domain.Shift{}, err
	}
	return sh, nil
}

// ShiftNumber is the shift's display ordinal ("shift-N").
func (a *App) ShiftNumber(ctx context.Context, restaurantID, shiftID uuid.UUID) (int, error) {
	return a.store.ShiftSequence(ctx, restaurantID, shiftID)
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
	return uuid.Nil, false
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
