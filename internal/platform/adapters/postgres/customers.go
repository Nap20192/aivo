package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
)

// --- Customers ---------------------------------------------------------

const customerCols = `id, email, password_hash, name, phone, created_at`

func scanCustomer(row interface{ Scan(...any) error }) (domain.Customer, error) {
	var c domain.Customer
	err := row.Scan(&c.ID, &c.Email, &c.PasswordHash, &c.Name, &c.Phone, &c.CreatedAt)
	return c, err
}

func (s *Store) CreateCustomer(ctx context.Context, c domain.Customer) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO customers (id, email, password_hash, name, phone) VALUES ($1, $2, $3, $4, $5)`,
		c.ID, c.Email, c.PasswordHash, c.Name, c.Phone)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("email taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: create customer: %w", err)
	}
	return nil
}

func (s *Store) CustomerByEmail(ctx context.Context, email string) (domain.Customer, error) {
	c, err := scanCustomer(s.db.QueryRowContext(ctx,
		`SELECT `+customerCols+` FROM customers WHERE email = $1`, email))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Customer{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Customer{}, fmt.Errorf("store: customer by email: %w", err)
	}
	return c, nil
}

func (s *Store) CustomerByID(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	c, err := scanCustomer(s.db.QueryRowContext(ctx,
		`SELECT `+customerCols+` FROM customers WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Customer{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Customer{}, fmt.Errorf("store: customer by id: %w", err)
	}
	return c, nil
}

func (s *Store) CreateCustomerSession(ctx context.Context, sess domain.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO customer_sessions (token_hash, customer_id, expires_at) VALUES ($1, $2, $3)`,
		sess.TokenHash, sess.UserID, sess.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: create customer session: %w", err)
	}
	return nil
}

func (s *Store) CustomerSession(ctx context.Context, tokenHash []byte) (domain.Customer, error) {
	c, err := scanCustomer(s.db.QueryRowContext(ctx,
		`SELECT c.id, c.email, c.password_hash, c.name, c.phone, c.created_at
		 FROM customer_sessions s JOIN customers c ON c.id = s.customer_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Customer{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Customer{}, fmt.Errorf("store: customer session: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteCustomerSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM customer_sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("store: delete customer session: %w", err)
	}
	return nil
}

// orderLinesFor loads display lines for a set of order IDs, one query
// per order (order history pages are small).
func (s *Store) orderLinesFor(ctx context.Context, orderID uuid.UUID) ([]domain.CustomerOrderLine, int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ol.name, ol.qty, ol.unit_price_cents,
		        COALESCE((SELECT json_agg(olo.label) FROM order_line_options olo WHERE olo.order_line_id = ol.id), '[]'),
		        COALESCE((SELECT sum(olo.price_delta_cents) FROM order_line_options olo WHERE olo.order_line_id = ol.id), 0)
		 FROM order_lines ol WHERE ol.order_id = $1`, orderID)
	if err != nil {
		return nil, 0, fmt.Errorf("store: order lines: %w", err)
	}
	defer rows.Close()

	lines := []domain.CustomerOrderLine{}
	total := 0
	for rows.Next() {
		var l domain.CustomerOrderLine
		var optsJSON []byte
		var deltaSum int
		if err := rows.Scan(&l.Name, &l.Qty, &l.UnitPriceCents, &optsJSON, &deltaSum); err != nil {
			return nil, 0, fmt.Errorf("store: order lines: scan: %w", err)
		}
		if err := json.Unmarshal(optsJSON, &l.Options); err != nil {
			return nil, 0, fmt.Errorf("store: order lines: options: %w", err)
		}
		l.TotalCents = (l.UnitPriceCents + deltaSum) * l.Qty
		total += l.TotalCents
		lines = append(lines, l)
	}
	return lines, total, rows.Err()
}

func (s *Store) CustomerOrders(ctx context.Context, customerID uuid.UUID, limit int) ([]domain.CustomerOrder, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, r.name, o.created_at
		 FROM orders o JOIN restaurants r ON r.id = o.restaurant_id
		 WHERE o.customer_id = $1
		 ORDER BY o.created_at DESC LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: customer orders: %w", err)
	}
	defer rows.Close()

	type head struct {
		id   uuid.UUID
		name string
		at   sql.NullTime
	}
	heads := []head{}
	for rows.Next() {
		var h head
		if err := rows.Scan(&h.id, &h.name, &h.at); err != nil {
			return nil, fmt.Errorf("store: customer orders: scan: %w", err)
		}
		heads = append(heads, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	orders := []domain.CustomerOrder{}
	for _, h := range heads {
		lines, total, err := s.orderLinesFor(ctx, h.id)
		if err != nil {
			return nil, err
		}
		orders = append(orders, domain.CustomerOrder{
			RestaurantName: h.name, CreatedAt: h.at.Time, TotalCents: total, Lines: lines,
		})
	}
	return orders, nil
}

// --- CRM ---------------------------------------------------------------

func (s *Store) TouchGuestProfile(ctx context.Context, restaurantID, customerID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guest_profiles (restaurant_id, customer_id) VALUES ($1, $2)
		 ON CONFLICT (restaurant_id, customer_id) DO UPDATE SET last_seen = now()`,
		restaurantID, customerID)
	if err != nil {
		return fmt.Errorf("store: touch guest profile: %w", err)
	}
	return nil
}

// guestTotalsSQL aggregates visits/spend per guest for THIS restaurant
// in one grouped query: linked menu orders UNION accepted-handoff ticket
// sales (tickets carry customer_id since a customer's handoff was
// accepted — a handoff-only regular must not read 0 visits / $0).
const guestTotalsSQL = `
	SELECT customer_id, count(DISTINCT src_id) AS visits, COALESCE(sum(amt), 0) AS spent
	FROM (
	    SELECT o.customer_id, o.id AS src_id,
	           (ol.unit_price_cents + COALESCE(d.delta, 0)) * ol.qty AS amt
	    FROM orders o
	    JOIN order_lines ol ON ol.order_id = o.id
	    LEFT JOIN LATERAL (
	        SELECT sum(olo.price_delta_cents) AS delta
	        FROM order_line_options olo WHERE olo.order_line_id = ol.id
	    ) d ON true
	    WHERE o.restaurant_id = $1 AND o.customer_id = ANY($2)
	  UNION ALL
	    SELECT t.customer_id, t.id,
	           (tl.unit_price_cents + COALESCE(td.delta, 0)) * tl.qty
	    FROM tickets t
	    JOIN ticket_lines tl ON tl.ticket_id = t.id
	    LEFT JOIN LATERAL (
	        SELECT sum((o2->>'price_delta_cents')::int) AS delta
	        FROM jsonb_array_elements(tl.options) o2
	    ) td ON true
	    WHERE t.restaurant_id = $1 AND t.customer_id = ANY($2)
	) src GROUP BY customer_id`

type guestTotals struct{ visits, spent int }

// guestTotalsFor runs the grouped aggregate once for the whole id set
// (kills the per-guest N+1). IDs absent from the result have no
// activity yet ({0, 0}).
func (s *Store) guestTotalsFor(ctx context.Context, restaurantID uuid.UUID, customerIDs []uuid.UUID) (map[uuid.UUID]guestTotals, error) {
	out := map[uuid.UUID]guestTotals{}
	if len(customerIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, guestTotalsSQL, restaurantID, customerIDs)
	if err != nil {
		return nil, fmt.Errorf("store: guest totals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var t guestTotals
		if err := rows.Scan(&id, &t.visits, &t.spent); err != nil {
			return nil, fmt.Errorf("store: guest totals: scan: %w", err)
		}
		out[id] = t
	}
	return out, rows.Err()
}

func (s *Store) Guests(ctx context.Context, restaurantID uuid.UUID, query string, limit int) ([]domain.GuestSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.email, c.password_hash, c.name, c.phone, c.created_at, g.tags, g.last_seen
		 FROM guest_profiles g JOIN customers c ON c.id = g.customer_id
		 WHERE g.restaurant_id = $1
		   AND ($2 = '' OR c.name ILIKE '%' || $2 || '%' OR c.email ILIKE '%' || $2 || '%')
		 ORDER BY g.last_seen DESC LIMIT $3`, restaurantID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("store: guests: %w", err)
	}
	defer rows.Close()

	type row struct {
		c        domain.Customer
		tags     []string
		lastSeen sql.NullTime
	}
	found := []row{}
	for rows.Next() {
		var r row
		var tagsJSON []byte
		if err := rows.Scan(&r.c.ID, &r.c.Email, &r.c.PasswordHash, &r.c.Name, &r.c.Phone, &r.c.CreatedAt, &tagsJSON, &r.lastSeen); err != nil {
			return nil, fmt.Errorf("store: guests: scan: %w", err)
		}
		if err := json.Unmarshal(tagsJSON, &r.tags); err != nil {
			return nil, fmt.Errorf("store: guests: tags: %w", err)
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(found))
	for i, r := range found {
		ids[i] = r.c.ID
	}
	totals, err := s.guestTotalsFor(ctx, restaurantID, ids)
	if err != nil {
		return nil, err
	}
	out := []domain.GuestSummary{}
	for _, r := range found {
		tags := r.tags
		if tags == nil {
			tags = []string{}
		}
		t := totals[r.c.ID]
		out = append(out, domain.GuestSummary{
			Customer: r.c, Visits: t.visits, TotalSpentCents: t.spent,
			LastSeen: r.lastSeen.Time, Tags: tags,
		})
	}
	return out, nil
}

func (s *Store) GuestProfile(ctx context.Context, restaurantID, customerID uuid.UUID) (domain.GuestProfile, domain.GuestSummary, error) {
	var p domain.GuestProfile
	var tagsJSON []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT restaurant_id, customer_id, notes, tags, first_seen, last_seen
		 FROM guest_profiles WHERE restaurant_id = $1 AND customer_id = $2`,
		restaurantID, customerID,
	).Scan(&p.RestaurantID, &p.CustomerID, &p.Notes, &tagsJSON, &p.FirstSeen, &p.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GuestProfile{}, domain.GuestSummary{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, fmt.Errorf("store: guest profile: %w", err)
	}
	if err := json.Unmarshal(tagsJSON, &p.Tags); err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, fmt.Errorf("store: guest profile: tags: %w", err)
	}

	c, err := s.CustomerByID(ctx, customerID)
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, err
	}
	totals, err := s.guestTotalsFor(ctx, restaurantID, []uuid.UUID{customerID})
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, err
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	t := totals[customerID]
	sum := domain.GuestSummary{Customer: c, Visits: t.visits, TotalSpentCents: t.spent, LastSeen: p.LastSeen, Tags: tags}
	return p, sum, nil
}

func (s *Store) GuestOrders(ctx context.Context, restaurantID, customerID uuid.UUID) ([]domain.GuestOrder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.created_at, COALESCE(t.label, '')
		 FROM orders o LEFT JOIN tables t ON t.id = o.table_id
		 WHERE o.restaurant_id = $1 AND o.customer_id = $2
		 ORDER BY o.created_at DESC LIMIT 100`, restaurantID, customerID)
	if err != nil {
		return nil, fmt.Errorf("store: guest orders: %w", err)
	}
	defer rows.Close()

	type head struct {
		id    uuid.UUID
		at    sql.NullTime
		label string
	}
	heads := []head{}
	for rows.Next() {
		var h head
		if err := rows.Scan(&h.id, &h.at, &h.label); err != nil {
			return nil, fmt.Errorf("store: guest orders: scan: %w", err)
		}
		heads = append(heads, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	orders := []domain.GuestOrder{}
	for _, h := range heads {
		lines, total, err := s.orderLinesFor(ctx, h.id)
		if err != nil {
			return nil, err
		}
		orders = append(orders, domain.GuestOrder{
			ID: h.id, CreatedAt: h.at.Time, TableLabel: h.label, TotalCents: total, Lines: lines,
		})
	}
	return orders, nil
}

func (s *Store) UpdateGuestProfile(ctx context.Context, p domain.GuestProfile) error {
	if p.Tags == nil {
		p.Tags = []string{}
	}
	tags, err := json.Marshal(p.Tags)
	if err != nil {
		return fmt.Errorf("store: update guest profile: encode tags: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE guest_profiles SET notes = $1, tags = $2 WHERE restaurant_id = $3 AND customer_id = $4`,
		p.Notes, tags, p.RestaurantID, p.CustomerID)
	if err != nil {
		return fmt.Errorf("store: update guest profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}
