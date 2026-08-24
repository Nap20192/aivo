// Package postgres implements pos ports.Store against Postgres.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aivo/internal/domain/pos"
	"aivo/internal/pos/ports"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"uuid"
)

// dbtx is the query surface shared by *sql.DB and *sql.Tx, so store
// methods run unchanged against a pool or a transaction.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	pool *sql.DB // for BeginTx/InTx; nil on a tx-bound store
	q    dbtx    // active querier: pool or a *sql.Tx
}

var _ ports.Store = (*Store)(nil)

func NewStore(db *sql.DB) *Store { return &Store{pool: db, q: db} }

// withTx returns a Store whose queries run on tx.
func (s *Store) withTx(tx *sql.Tx) *Store { return &Store{pool: s.pool, q: tx} }

// InTx runs fn in one transaction, exposing the raw *sql.Tx (for the
// ledger port) and a tx-bound Store.
func (s *Store) InTx(ctx context.Context, fn func(tx *sql.Tx, st ports.Store) error) error {
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pos store: begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx, s.withTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const shiftCols = `id, restaurant_id, opened_by, cashier, opened_at, opening_float_cents, closed_at, declared_cents, expected_cents, variance_cents`

func scanShift(row interface{ Scan(...any) error }) (domain.Shift, error) {
	var s domain.Shift
	err := row.Scan(&s.ID, &s.RestaurantID, &s.OpenedBy, &s.Cashier, &s.OpenedAt, &s.OpeningFloatCents,
		&s.ClosedAt, &s.DeclaredCents, &s.ExpectedCents, &s.VarianceCents)
	return s, err
}

func (s *Store) OpenShift(ctx context.Context, sh domain.Shift) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO shifts (id, restaurant_id, opened_by, cashier, opening_float_cents) VALUES ($1, $2, $3, $4, $5)`,
		sh.ID, sh.RestaurantID, sh.OpenedBy, sh.Cashier, sh.OpeningFloatCents)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("shift already open: %w", ports.ErrConflict)
		}
		return fmt.Errorf("pos store: open shift: %w", err)
	}
	return nil
}

func (s *Store) OpenShiftFor(ctx context.Context, restaurantID uuid.UUID) (domain.Shift, error) {
	sh, err := scanShift(s.q.QueryRowContext(ctx,
		`SELECT `+shiftCols+` FROM shifts WHERE restaurant_id = $1 AND closed_at IS NULL`, restaurantID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Shift{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Shift{}, fmt.Errorf("pos store: open shift for: %w", err)
	}
	return sh, nil
}

func (s *Store) ShiftByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Shift, error) {
	sh, err := scanShift(s.q.QueryRowContext(ctx,
		`SELECT `+shiftCols+` FROM shifts WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Shift{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Shift{}, fmt.Errorf("pos store: shift by id: %w", err)
	}
	return sh, nil
}

// CloseShift posts the closing figures. The WHERE closed_at IS NULL
// guard is what makes a posted close immutable — a second close (or a
// concurrent one) matches zero rows and returns ErrConflict.
func (s *Store) CloseShift(ctx context.Context, sh domain.Shift) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE shifts SET closed_at = $1, declared_cents = $2, expected_cents = $3, variance_cents = $4
		 WHERE restaurant_id = $5 AND id = $6 AND closed_at IS NULL`,
		sh.ClosedAt, sh.DeclaredCents, sh.ExpectedCents, sh.VarianceCents, sh.RestaurantID, sh.ID)
	if err != nil {
		return fmt.Errorf("pos store: close shift: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("shift already closed or not found: %w", ports.ErrConflict)
	}
	return nil
}

func (s *Store) ShiftSequence(ctx context.Context, restaurantID, shiftID uuid.UUID) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx,
		`SELECT count(*) FROM shifts
		 WHERE restaurant_id = $1
		   AND opened_at <= (SELECT opened_at FROM shifts WHERE restaurant_id = $1 AND id = $2)`,
		restaurantID, shiftID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("pos store: shift sequence: %w", err)
	}
	if n == 0 {
		return 0, ports.ErrNotFound
	}
	return n, nil
}

func (s *Store) OpenTicketForTable(ctx context.Context, restaurantID, tableID uuid.UUID) (domain.Ticket, error) {
	var t domain.Ticket
	err := s.q.QueryRowContext(ctx,
		`SELECT id, restaurant_id, shift_id, table_id, customer_id, status, note, created_at
		 FROM tickets WHERE restaurant_id = $1 AND table_id = $2 AND status = $3`,
		restaurantID, tableID, domain.TicketOpen,
	).Scan(&t.ID, &t.RestaurantID, &t.ShiftID, &t.TableID, &t.CustomerID, &t.Status, &t.Note, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ticket{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("pos store: open ticket for table: %w", err)
	}
	if err := s.attachLines(ctx, []*domain.Ticket{&t}); err != nil {
		return domain.Ticket{}, err
	}
	return t, nil
}

func (s *Store) CreateTicket(ctx context.Context, t domain.Ticket) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO tickets (id, restaurant_id, shift_id, table_id, status) VALUES ($1, $2, $3, $4, $5)`,
		t.ID, t.RestaurantID, t.ShiftID, t.TableID, domain.TicketOpen)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("table already has an open ticket: %w", ports.ErrConflict)
		}
		return fmt.Errorf("pos store: create ticket: %w", err)
	}
	return nil
}

func (s *Store) AddLines(ctx context.Context, ticketID uuid.UUID, lines []domain.TicketLine) error {
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pos store: add lines: begin: %w", err)
	}
	defer tx.Rollback()

	for _, l := range lines {
		opts, err := json.Marshal(l.Options)
		if err != nil {
			return fmt.Errorf("pos store: add lines: encode options: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ticket_lines (id, ticket_id, menu_item_id, name, unit_price_cents, qty, options)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			l.ID, ticketID, l.MenuItemID, l.Name, l.UnitPriceCents, l.Qty, opts); err != nil {
			return fmt.Errorf("pos store: add lines: %w", err)
		}
	}
	return tx.Commit()
}

// AppendTicketNote adds note to the ticket's note field (newline-joined
// when one already exists).
func (s *Store) AppendTicketNote(ctx context.Context, restaurantID, id uuid.UUID, note string) error {
	if err := s.requireOpenTicket(ctx, restaurantID, id); err != nil {
		return err
	}
	res, err := s.q.ExecContext(ctx,
		`UPDATE tickets SET note = CASE WHEN note = '' THEN $1 ELSE note || E'\n' || $1 END
		 WHERE restaurant_id = $2 AND id = $3 AND status = 'open'`, note, restaurantID, id)
	if err != nil {
		return fmt.Errorf("pos store: append note: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *Store) TicketByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Ticket, error) {
	var t domain.Ticket
	err := s.q.QueryRowContext(ctx,
		`SELECT id, restaurant_id, shift_id, table_id, customer_id, status, note, created_at, closed_at
		 FROM tickets WHERE restaurant_id = $1 AND id = $2`, restaurantID, id,
	).Scan(&t.ID, &t.RestaurantID, &t.ShiftID, &t.TableID, &t.CustomerID, &t.Status, &t.Note, &t.CreatedAt, &t.ClosedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ticket{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("pos store: ticket by id: %w", err)
	}
	if err := s.attachLines(ctx, []*domain.Ticket{&t}); err != nil {
		return domain.Ticket{}, err
	}
	return t, nil
}

// OpenTickets returns every open ticket of the restaurant with lines,
// in TWO queries total (the 5s POS poll's hot path): tickets, then
// ticket_lines WHERE ticket_id = ANY(...).
func (s *Store) OpenTickets(ctx context.Context, restaurantID uuid.UUID) ([]domain.Ticket, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, restaurant_id, shift_id, table_id, customer_id, status, note, created_at
		 FROM tickets WHERE restaurant_id = $1 AND status = $2 ORDER BY created_at ASC`,
		restaurantID, domain.TicketOpen)
	if err != nil {
		return nil, fmt.Errorf("pos store: open tickets: %w", err)
	}
	defer rows.Close()

	tickets := []*domain.Ticket{}
	for rows.Next() {
		var t domain.Ticket
		if err := rows.Scan(&t.ID, &t.RestaurantID, &t.ShiftID, &t.TableID, &t.CustomerID, &t.Status, &t.Note, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("pos store: open tickets: scan: %w", err)
		}
		tickets = append(tickets, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachLinesBatch(ctx, tickets); err != nil {
		return nil, err
	}
	out := make([]domain.Ticket, len(tickets))
	for i, t := range tickets {
		out[i] = *t
	}
	return out, nil
}

// attachLinesBatch fills Lines for all tickets in one query.
func (s *Store) attachLinesBatch(ctx context.Context, tickets []*domain.Ticket) error {
	if len(tickets) == 0 {
		return nil
	}
	index := map[uuid.UUID]*domain.Ticket{}
	ids := make([]uuid.UUID, len(tickets))
	for i, t := range tickets {
		t.Lines = []domain.TicketLine{}
		index[t.ID] = t
		ids[i] = t.ID
	}
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, ticket_id, menu_item_id, name, unit_price_cents, qty, options, fired_at, created_at
		 FROM ticket_lines WHERE ticket_id = ANY($1) ORDER BY created_at ASC`, ids)
	if err != nil {
		return fmt.Errorf("pos store: lines batch: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var l domain.TicketLine
		var opts []byte
		if err := rows.Scan(&l.ID, &l.TicketID, &l.MenuItemID, &l.Name, &l.UnitPriceCents, &l.Qty, &opts, &l.FiredAt, &l.CreatedAt); err != nil {
			return fmt.Errorf("pos store: lines batch: scan: %w", err)
		}
		if err := json.Unmarshal(opts, &l.Options); err != nil {
			return fmt.Errorf("pos store: lines batch: decode options: %w", err)
		}
		if t, ok := index[l.TicketID]; ok {
			t.Lines = append(t.Lines, l)
		}
	}
	return rows.Err()
}

// LinkTicketCustomer sets the ticket's customer if none is linked yet
// (first accepted handoff wins). Guarded to open tickets (debt 3 —
// immutability): a closed ticket is never mutated.
func (s *Store) LinkTicketCustomer(ctx context.Context, restaurantID, id, customerID uuid.UUID) error {
	if err := s.requireOpenTicket(ctx, restaurantID, id); err != nil {
		return err
	}
	if _, err := s.q.ExecContext(ctx,
		`UPDATE tickets SET customer_id = $1
		 WHERE restaurant_id = $2 AND id = $3 AND status = 'open' AND customer_id IS NULL`,
		customerID, restaurantID, id); err != nil {
		return fmt.Errorf("pos store: link customer: %w", err)
	}
	return nil
}

// FireTicket stamps fired_at on the ticket's unfired lines. Guarded to
// open tickets (debt 3): firing a closed ticket is rejected.
func (s *Store) FireTicket(ctx context.Context, restaurantID, id uuid.UUID) error {
	if err := s.requireOpenTicket(ctx, restaurantID, id); err != nil {
		return err
	}
	res, err := s.q.ExecContext(ctx,
		`UPDATE ticket_lines SET fired_at = now()
		 WHERE fired_at IS NULL AND ticket_id IN (
		   SELECT id FROM tickets WHERE restaurant_id = $1 AND id = $2 AND status = 'open'
		 )`, restaurantID, id)
	if err != nil {
		return fmt.Errorf("pos store: fire ticket: %w", err)
	}
	_ = res // zero unfired lines is fine (idempotent fire)
	return nil
}

// requireOpenTicket returns ErrNotFound if the ticket is missing (wrong
// tenant) and ErrConflict if it is closed — the shared immutability guard
// every mutating ticket path routes through (debt 3).
func (s *Store) requireOpenTicket(ctx context.Context, restaurantID, id uuid.UUID) error {
	var status string
	err := s.q.QueryRowContext(ctx,
		`SELECT status FROM tickets WHERE restaurant_id = $1 AND id = $2`, restaurantID, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("pos store: ticket status: %w", err)
	}
	if status != domain.TicketOpen {
		return fmt.Errorf("ticket already closed: %w", ports.ErrConflict)
	}
	return nil
}

func (s *Store) TicketsForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]domain.Ticket, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, restaurant_id, shift_id, table_id, customer_id, status, note, created_at
		 FROM tickets WHERE restaurant_id = $1 AND shift_id = $2 ORDER BY created_at ASC`,
		restaurantID, shiftID)
	if err != nil {
		return nil, fmt.Errorf("pos store: tickets for shift: %w", err)
	}
	defer rows.Close()

	tickets := []*domain.Ticket{}
	for rows.Next() {
		var t domain.Ticket
		if err := rows.Scan(&t.ID, &t.RestaurantID, &t.ShiftID, &t.TableID, &t.CustomerID, &t.Status, &t.Note, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("pos store: tickets for shift: scan: %w", err)
		}
		tickets = append(tickets, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachLines(ctx, tickets); err != nil {
		return nil, err
	}
	out := make([]domain.Ticket, len(tickets))
	for i, t := range tickets {
		out[i] = *t
	}
	return out, nil
}

func (s *Store) CloseTickets(ctx context.Context, restaurantID, shiftID uuid.UUID) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE tickets SET status = $1, closed_at = now()
		 WHERE restaurant_id = $2 AND shift_id = $3 AND status = $4`,
		domain.TicketClosed, restaurantID, shiftID, domain.TicketOpen)
	if err != nil {
		return fmt.Errorf("pos store: close tickets: %w", err)
	}
	return nil
}

// attachLines fills Lines for each ticket.
// ponytail: one query per ticket — a shift has at most a few dozen
// tickets; batch with a uuid[] param if this ever shows up in profiles.
func (s *Store) attachLines(ctx context.Context, tickets []*domain.Ticket) error {
	for _, t := range tickets {
		t.Lines = []domain.TicketLine{}
		rows, err := s.q.QueryContext(ctx,
			`SELECT id, ticket_id, menu_item_id, name, unit_price_cents, qty, options, fired_at, created_at
			 FROM ticket_lines WHERE ticket_id = $1 ORDER BY created_at ASC`, t.ID)
		if err != nil {
			return fmt.Errorf("pos store: lines: %w", err)
		}
		for rows.Next() {
			var l domain.TicketLine
			var opts []byte
			if err := rows.Scan(&l.ID, &l.TicketID, &l.MenuItemID, &l.Name, &l.UnitPriceCents, &l.Qty, &opts, &l.FiredAt, &l.CreatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("pos store: lines: scan: %w", err)
			}
			if err := json.Unmarshal(opts, &l.Options); err != nil {
				rows.Close()
				return fmt.Errorf("pos store: lines: decode options: %w", err)
			}
			t.Lines = append(t.Lines, l)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

// --- payments / cash movements / acceptance (increment-1) --------------

// LockShift loads the shift FOR UPDATE, serializing CloseShift/Accept.
func (s *Store) LockShift(ctx context.Context, restaurantID, id uuid.UUID) (domain.Shift, error) {
	sh, err := scanShift(s.q.QueryRowContext(ctx,
		`SELECT `+shiftCols+` FROM shifts WHERE restaurant_id = $1 AND id = $2 FOR UPDATE`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Shift{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Shift{}, fmt.Errorf("pos store: lock shift: %w", err)
	}
	return sh, nil
}

func (s *Store) PaymentMethods(ctx context.Context, restaurantID uuid.UUID) ([]domain.PaymentMethod, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, restaurant_id, code, name, payment_group, active
		 FROM payment_methods WHERE restaurant_id = $1 ORDER BY code`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("pos store: payment methods: %w", err)
	}
	defer rows.Close()
	out := []domain.PaymentMethod{}
	for rows.Next() {
		var m domain.PaymentMethod
		if err := rows.Scan(&m.ID, &m.RestaurantID, &m.Code, &m.Name, &m.PaymentGroup, &m.Active); err != nil {
			return nil, fmt.Errorf("pos store: payment methods: scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CloseTicketWithPayments inserts the tenders and stamps closed_at, only
// while status='open' (debt 3 immutability). ErrConflict if already
// closed. The domain Ticket.Close having validated the tenders, this is
// the persistence half — both in one transaction (the caller's tx).
func (s *Store) CloseTicketWithPayments(ctx context.Context, restaurantID uuid.UUID, t domain.Ticket, tenders []domain.Tender, recordedBy uuid.UUID) error {
	run := func(st *Store) error {
		res, err := st.q.ExecContext(ctx,
			`UPDATE tickets SET status = 'closed', closed_at = $3
			 WHERE restaurant_id = $1 AND id = $2 AND status = 'open'`,
			restaurantID, t.ID, t.ClosedAt)
		if err != nil {
			return fmt.Errorf("pos store: close ticket: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("ticket already closed: %w", ports.ErrConflict)
		}
		for _, td := range tenders {
			if _, err := st.q.ExecContext(ctx,
				`INSERT INTO ticket_payments
				   (id, ticket_id, restaurant_id, method_id, payment_group, amount_cents, tip_cents, recorded_by)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
				uuid.New(), t.ID, restaurantID, td.MethodID, td.PaymentGroup, td.AmountCents, td.TipCents, recordedBy); err != nil {
				return fmt.Errorf("pos store: insert tender: %w", err)
			}
		}
		return nil
	}
	if s.pool == nil { // already in a tx
		return run(s)
	}
	return s.InTx(ctx, func(_ *sql.Tx, st ports.Store) error { return run(st.(*Store)) })
}

func (s *Store) RecordCashOperation(ctx context.Context, op domain.CashOperation) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO cash_operations (id, shift_id, restaurant_id, kind, amount_cents, reason, recorded_by, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		op.ID, op.ShiftID, op.RestaurantID, op.Kind, op.AmountCents, op.Reason, op.RecordedBy, op.RecordedAt)
	if err != nil {
		return fmt.Errorf("pos store: record cash op: %w", err)
	}
	return nil
}

func (s *Store) TendersForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]ports.TenderGroupTotal, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT tp.payment_group,
		        COALESCE(sum(tp.amount_cents), 0),
		        COALESCE(sum(tp.tip_cents), 0)
		 FROM ticket_payments tp
		 JOIN tickets t ON t.id = tp.ticket_id
		 WHERE tp.restaurant_id = $1 AND t.shift_id = $2
		 GROUP BY tp.payment_group ORDER BY tp.payment_group`, restaurantID, shiftID)
	if err != nil {
		return nil, fmt.Errorf("pos store: tenders for shift: %w", err)
	}
	defer rows.Close()
	out := []ports.TenderGroupTotal{}
	for rows.Next() {
		var g ports.TenderGroupTotal
		if err := rows.Scan(&g.Group, &g.AmountCents, &g.TipCents); err != nil {
			return nil, fmt.Errorf("pos store: tenders for shift: scan: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CashOperationsForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]domain.CashOperation, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, shift_id, restaurant_id, kind, amount_cents, reason, recorded_by, recorded_at
		 FROM cash_operations WHERE restaurant_id = $1 AND shift_id = $2 ORDER BY recorded_at`, restaurantID, shiftID)
	if err != nil {
		return nil, fmt.Errorf("pos store: cash ops for shift: %w", err)
	}
	defer rows.Close()
	out := []domain.CashOperation{}
	for rows.Next() {
		var op domain.CashOperation
		if err := rows.Scan(&op.ID, &op.ShiftID, &op.RestaurantID, &op.Kind, &op.AmountCents, &op.Reason, &op.RecordedBy, &op.RecordedAt); err != nil {
			return nil, fmt.Errorf("pos store: cash ops for shift: scan: %w", err)
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

// AcceptShift stamps acceptance, only while closed and not yet accepted.
// The WHERE guard makes a double accept a no-op → ErrConflict (409).
func (s *Store) AcceptShift(ctx context.Context, sh domain.Shift) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE shifts SET accepted_at = $3, accepted_by = $4, journal_document_id = $5
		 WHERE restaurant_id = $1 AND id = $2 AND closed_at IS NOT NULL AND accepted_at IS NULL`,
		sh.RestaurantID, sh.ID, sh.AcceptedAt, sh.AcceptedBy, sh.JournalDocumentID)
	if err != nil {
		return fmt.Errorf("pos store: accept shift: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("shift not closed or already accepted: %w", ports.ErrConflict)
	}
	return nil
}

// ShiftsByState lists shifts filtered by acceptance state.
func (s *Store) ShiftsByState(ctx context.Context, restaurantID uuid.UUID, state string) ([]domain.Shift, error) {
	cond := "closed_at IS NOT NULL AND accepted_at IS NULL" // "closed"
	if state == domain.ShiftAccepted {
		cond = "accepted_at IS NOT NULL"
	}
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+shiftCols+`, accepted_at, accepted_by, journal_document_id
		 FROM shifts WHERE restaurant_id = $1 AND `+cond+` ORDER BY opened_at DESC`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("pos store: shifts by state: %w", err)
	}
	defer rows.Close()
	out := []domain.Shift{}
	for rows.Next() {
		var sh domain.Shift
		if err := rows.Scan(&sh.ID, &sh.RestaurantID, &sh.OpenedBy, &sh.Cashier, &sh.OpenedAt, &sh.OpeningFloatCents,
			&sh.ClosedAt, &sh.DeclaredCents, &sh.ExpectedCents, &sh.VarianceCents,
			&sh.AcceptedAt, &sh.AcceptedBy, &sh.JournalDocumentID); err != nil {
			return nil, fmt.Errorf("pos store: shifts by state: scan: %w", err)
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// SeedDefaultPaymentMethods inserts the default tender methods (cash, card)
// for a restaurant on the given transaction — used by live restaurant
// provisioning so a self-registered restaurant can take tenders.
func SeedDefaultPaymentMethods(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID) error {
	for _, m := range []struct{ code, name, group string }{
		{"cash", "Cash", "cash"},
		{"card", "Card", "card"},
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO payment_methods (id, restaurant_id, code, name, payment_group) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), restaurantID, m.code, m.name, m.group); err != nil {
			return fmt.Errorf("pos store: seed payment method %s: %w", m.code, err)
		}
	}
	return nil
}
