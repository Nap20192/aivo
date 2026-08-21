// Package app is the POS context's use-case layer: shift lifecycle,
// per-table tickets, firing, and the service-request inbox.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	menudomain "aivo/internal/menu/domain"
	"aivo/internal/pos/domain"
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
	Shift    *domain.Shift
	Tables   []TableState
	Requests []menudomain.ServiceRequest
}

type TableState struct {
	Table  menudomain.Table
	Ticket *domain.Ticket
}

func (a *App) State(ctx context.Context, restaurantID uuid.UUID) (State, error) {
	st := State{}

	shift, err := a.store.OpenShiftFor(ctx, restaurantID)
	switch {
	case err == nil:
		st.Shift = &shift
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
		ts := TableState{Table: t}
		ticket, err := a.store.OpenTicketForTable(ctx, restaurantID, t.ID)
		switch {
		case err == nil:
			ts.Ticket = &ticket
		case errors.Is(err, ports.ErrNotFound):
		default:
			return State{}, err
		}
		st.Tables = append(st.Tables, ts)
	}

	st.Requests, err = a.menu.PendingServiceRequests(ctx, restaurantID)
	if err != nil {
		return State{}, err
	}
	return st, nil
}

func (a *App) OpenShift(ctx context.Context, restaurantID, userID uuid.UUID, openingFloatCents int) (domain.Shift, error) {
	if openingFloatCents < 0 {
		return domain.Shift{}, fmt.Errorf("%w: opening_float_cents must be >= 0", ErrInvalid)
	}
	sh := domain.Shift{
		ID:                uuid.New(),
		RestaurantID:      restaurantID,
		OpenedBy:          userID,
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

// LineInput is one line a waiter adds: item + chosen option IDs + qty.
type LineInput struct {
	MenuItemID uuid.UUID
	OptionIDs  []uuid.UUID
	Qty        int
}

// AddLines appends snapshot lines to the table's open ticket, creating
// the ticket under the open shift if the table has none. Snapshots reuse
// the menu context's NewOrderLine validation (qty, availability, option
// ownership).
func (a *App) AddLines(ctx context.Context, restaurantID, tableID uuid.UUID, inputs []LineInput) (domain.Ticket, error) {
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
		snap, err := menudomain.NewOrderLine(item, in.OptionIDs, in.Qty)
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
	return a.store.TicketByID(ctx, restaurantID, ticket.ID)
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

func (a *App) AckRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return a.menu.AckServiceRequest(ctx, restaurantID, id)
}

func (a *App) DismissRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return a.menu.DismissServiceRequest(ctx, restaurantID, id)
}
