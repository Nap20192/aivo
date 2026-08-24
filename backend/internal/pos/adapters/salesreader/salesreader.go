// Package salesreader implements inventory ports.SalesReader over the pos
// tables (tickets / ticket_lines) — the food-cost report's revenue and
// sold-quantity source. It lives in pos (which owns those tables); the
// dependency direction is pos → inventory (legal, §2). Read-only.
package salesreader

import (
	"context"
	"database/sql"
	"fmt"

	"aivo/internal/inventory/ports"

	"uuid"
)

type Reader struct {
	db *sql.DB
}

var _ ports.SalesReader = (*Reader)(nil)

func New(db *sql.DB) *Reader { return &Reader{db: db} }

// SoldDishes aggregates closed-ticket sales by menu item over [from,to]
// (by close date): item count (×1000 as milli) and revenue (unit price +
// option deltas). Name is the snapshot line name.
func (r *Reader) SoldDishes(ctx context.Context, restaurantID uuid.UUID, from, to string) ([]ports.SaleQty, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tl.menu_item_id, min(tl.name),
		        COALESCE(sum(tl.qty), 0) * 1000 AS qty_milli,
		        COALESCE(sum((tl.unit_price_cents + COALESCE(d.delta, 0)) * tl.qty), 0) AS revenue
		 FROM tickets t
		 JOIN ticket_lines tl ON tl.ticket_id = t.id
		 LEFT JOIN LATERAL (
		     SELECT sum((o->>'price_delta_cents')::int) AS delta
		     FROM jsonb_array_elements(tl.options) o
		 ) d ON true
		 WHERE t.restaurant_id = $1 AND t.status = 'closed'
		   AND t.closed_at::date >= $2::date AND t.closed_at::date <= $3::date
		 GROUP BY tl.menu_item_id`,
		restaurantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("salesreader: sold dishes: %w", err)
	}
	defer rows.Close()
	out := []ports.SaleQty{}
	for rows.Next() {
		var s ports.SaleQty
		if err := rows.Scan(&s.MenuItemID, &s.Name, &s.QtyMilli, &s.RevenueCents); err != nil {
			return nil, fmt.Errorf("salesreader: scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
