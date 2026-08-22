// Package ports defines the boundaries the POS app depends on:
// persistence (Store) and the in-process bridge to the Menu context
// (Menu) — Go interfaces, not gRPC, per ADR 0001.
package ports

import (
	"context"
	"errors"

	menudomain "aivo/internal/menu/domain"
	"aivo/internal/pos/domain"

	"github.com/google/uuid"
)

// ErrNotFound mirrors the other contexts' not-found sentinel.
var ErrNotFound = errors.New("pos: not found")

// ErrConflict is returned when a uniqueness rule blocks a write (a shift
// is already open, a table already has an open ticket under another
// shift).
var ErrConflict = errors.New("pos: conflict")

// Store is POS persistence. Every method is scoped by restaurantID in
// the query itself.
type Store interface {
	// OpenShift creates a new open shift. Returns ErrConflict if the
	// restaurant already has one open.
	OpenShift(ctx context.Context, s domain.Shift) error
	// OpenShiftFor returns the restaurant's open shift, ErrNotFound if none.
	OpenShiftFor(ctx context.Context, restaurantID uuid.UUID) (domain.Shift, error)
	// ShiftByID returns the shift, scoped to restaurantID.
	ShiftByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Shift, error)
	// CloseShift persists the closing figures of an already-Close()d
	// shift. Only rows with closed_at IS NULL are updated (immutability);
	// returns ErrConflict if the row was already closed.
	CloseShift(ctx context.Context, s domain.Shift) error
	// ShiftSequence returns the shift's 1-based ordinal within its
	// restaurant (by opened_at), for display numbers like "shift-121".
	ShiftSequence(ctx context.Context, restaurantID, shiftID uuid.UUID) (int, error)

	// OpenTicketForTable returns the table's open ticket with lines,
	// ErrNotFound if none.
	OpenTicketForTable(ctx context.Context, restaurantID, tableID uuid.UUID) (domain.Ticket, error)
	// CreateTicket creates an open ticket. Returns ErrConflict if the
	// table already has one open.
	CreateTicket(ctx context.Context, t domain.Ticket) error
	// AddLines appends lines to an open ticket.
	AddLines(ctx context.Context, ticketID uuid.UUID, lines []domain.TicketLine) error
	// AppendTicketNote adds a diner note to the ticket (newline-joined).
	AppendTicketNote(ctx context.Context, restaurantID, id uuid.UUID, note string) error
	// TicketByID returns the ticket with lines, scoped to restaurantID.
	TicketByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Ticket, error)
	// FireTicket stamps fired_at=now on every unfired line of the ticket.
	FireTicket(ctx context.Context, restaurantID, id uuid.UUID) error
	// TicketsForShift returns every ticket (with lines) of the shift.
	TicketsForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]domain.Ticket, error)
	// CloseTickets closes every open ticket of the shift (shift close).
	CloseTickets(ctx context.Context, restaurantID, shiftID uuid.UUID) error
}

// Menu is the in-process bridge to the Menu context: item lookups for
// line snapshots, tables for the floor view, and the service-request
// inbox.
type Menu interface {
	// MenuItemByID returns the item (with option groups), scoped to
	// restaurantID.
	MenuItemByID(ctx context.Context, restaurantID, id uuid.UUID) (menudomain.MenuItem, error)
	// Tables returns every table of the restaurant.
	Tables(ctx context.Context, restaurantID uuid.UUID) ([]menudomain.Table, error)
	// TableByID returns one table, scoped to restaurantID.
	TableByID(ctx context.Context, restaurantID, id uuid.UUID) (menudomain.Table, error)
	// PendingServiceRequests returns the restaurant's pending requests,
	// oldest first.
	PendingServiceRequests(ctx context.Context, restaurantID uuid.UUID) ([]menudomain.ServiceRequest, error)
	// AckServiceRequest / DismissServiceRequest transition a request,
	// scoped to restaurantID (ErrNotFound on wrong tenant).
	AckServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error
	DismissServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error
}
