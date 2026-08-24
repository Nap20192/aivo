// Package ports defines the boundaries the POS app depends on:
// persistence (Store) and the in-process bridge to the Menu context
// (Menu) — Go interfaces, not gRPC, per ADR 0001.
package ports

import (
	"context"
	"database/sql"
	"errors"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/pos"

	"uuid"
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
	// OpenTickets returns every open ticket (with lines) of the
	// restaurant in two queries — the pos state hot path.
	OpenTickets(ctx context.Context, restaurantID uuid.UUID) ([]domain.Ticket, error)
	// LinkTicketCustomer sets the ticket's customer when none is linked.
	LinkTicketCustomer(ctx context.Context, restaurantID, id, customerID uuid.UUID) error
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

	// --- payments / cash movements / acceptance (increment-1) ---

	// InTx runs fn in a single transaction, passing the raw *sql.Tx (for
	// the ledger port) and a Store bound to that transaction.
	InTx(ctx context.Context, fn func(tx *sql.Tx, s Store) error) error
	// LockShift loads the shift FOR UPDATE (CloseShift serialization).
	LockShift(ctx context.Context, restaurantID, shiftID uuid.UUID) (domain.Shift, error)
	// PaymentMethods returns the restaurant's tender methods.
	PaymentMethods(ctx context.Context, restaurantID uuid.UUID) ([]domain.PaymentMethod, error)
	// RecordTicketPayments inserts the tenders taken at ticket close and
	// stamps closed_at — only while status='open' (immutability guard,
	// debt 3); ErrConflict if already closed.
	CloseTicketWithPayments(ctx context.Context, restaurantID uuid.UUID, t domain.Ticket, tenders []domain.Tender, recordedBy uuid.UUID) error
	// RecordCashOperation inserts a pay-in/out/drop; requires an open shift.
	RecordCashOperation(ctx context.Context, op domain.CashOperation) error
	// TendersForShift aggregates tenders by payment group.
	TendersForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]TenderGroupTotal, error)
	// CashOperationsForShift returns the shift's cash movements.
	CashOperationsForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]domain.CashOperation, error)
	// AcceptShift stamps accepted_at/by + journal_document_id, only while
	// closed and not yet accepted; ErrConflict otherwise (double accept).
	AcceptShift(ctx context.Context, s domain.Shift) error
	// ShiftsByState lists shifts in the "closed" (closed, not accepted) or
	// "accepted" state, newest first (acceptance queue).
	ShiftsByState(ctx context.Context, restaurantID uuid.UUID, state string) ([]domain.Shift, error)
}

// TenderGroupTotal is a shift's tender total for one payment group.
type TenderGroupTotal struct {
	Group       string
	AmountCents int64
	TipCents    int64
}

// ShiftJournalDraft is what pos hands the ledger to build the acceptance
// draft (contract §3). Tips are recorded on ticket_payments but not posted
// in increment-1; the journal uses tender amounts only.
type ShiftJournalDraft struct {
	ShiftID        uuid.UUID
	CreatedBy      uuid.UUID
	AccountingDate time.Time
	Tenders        []TenderGroupTotal
	VarianceCents  int64
}

// Ledger is the synchronous pos→ledger bridge. Build/Post run inside the
// pos transaction (shared *sql.Tx). Account-map reads and reversals are
// ledger back-office operations handled directly by the ledger app, not
// through this bridge.
type Ledger interface {
	// BuildDraftShiftJournal creates the acceptance draft in tx.
	BuildDraftShiftJournal(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID, draft ShiftJournalDraft) (uuid.UUID, error)
	// PostJournal posts a draft document (draft→posted) in tx.
	PostJournal(ctx context.Context, tx *sql.Tx, restaurantID, docID uuid.UUID) error
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
