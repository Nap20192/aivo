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

type Store struct {
	db *sql.DB
}

var _ ports.Store = (*Store)(nil)

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

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
	_, err := s.db.ExecContext(ctx,
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
	sh, err := scanShift(s.db.QueryRowContext(ctx,
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
	sh, err := scanShift(s.db.QueryRowContext(ctx,
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
	res, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
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
	tx, err := s.db.BeginTx(ctx, nil)
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
	res, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET note = CASE WHEN note = '' THEN $1 ELSE note || E'\n' || $1 END
		 WHERE restaurant_id = $2 AND id = $3`, note, restaurantID, id)
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
	err := s.db.QueryRowContext(ctx,
		`SELECT id, restaurant_id, shift_id, table_id, customer_id, status, note, created_at
		 FROM tickets WHERE restaurant_id = $1 AND id = $2`, restaurantID, id,
	).Scan(&t.ID, &t.RestaurantID, &t.ShiftID, &t.TableID, &t.CustomerID, &t.Status, &t.Note, &t.CreatedAt)
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
	rows, err := s.db.QueryContext(ctx,
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
	rows, err := s.db.QueryContext(ctx,
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

// ShiftClosedSalesCents sums the shift's CLOSED tickets (option deltas
// included) in one aggregate — the open ones come from OpenTickets.
func (s *Store) ShiftClosedSalesCents(ctx context.Context, restaurantID, shiftID uuid.UUID) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(sum((tl.unit_price_cents + COALESCE(d.delta, 0)) * tl.qty), 0)
		 FROM tickets t
		 JOIN ticket_lines tl ON tl.ticket_id = t.id
		 LEFT JOIN LATERAL (
		     SELECT sum((o->>'price_delta_cents')::int) AS delta
		     FROM jsonb_array_elements(tl.options) o
		 ) d ON true
		 WHERE t.restaurant_id = $1 AND t.shift_id = $2 AND t.status = $3`,
		restaurantID, shiftID, domain.TicketClosed).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("pos store: closed sales: %w", err)
	}
	return total, nil
}

// LinkTicketCustomer sets the ticket's customer if none is linked yet
// (first accepted handoff wins).
func (s *Store) LinkTicketCustomer(ctx context.Context, restaurantID, id, customerID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET customer_id = $1
		 WHERE restaurant_id = $2 AND id = $3 AND customer_id IS NULL`,
		customerID, restaurantID, id); err != nil {
		return fmt.Errorf("pos store: link customer: %w", err)
	}
	return nil
}

func (s *Store) FireTicket(ctx context.Context, restaurantID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE ticket_lines SET fired_at = now()
		 WHERE fired_at IS NULL AND ticket_id IN (
		   SELECT id FROM tickets WHERE restaurant_id = $1 AND id = $2
		 )`, restaurantID, id)
	if err != nil {
		return fmt.Errorf("pos store: fire ticket: %w", err)
	}
	_ = res // zero unfired lines is fine (idempotent fire)
	return nil
}

func (s *Store) TicketsForShift(ctx context.Context, restaurantID, shiftID uuid.UUID) ([]domain.Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET status = $1 WHERE restaurant_id = $2 AND shift_id = $3 AND status = $4`,
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
		rows, err := s.db.QueryContext(ctx,
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
