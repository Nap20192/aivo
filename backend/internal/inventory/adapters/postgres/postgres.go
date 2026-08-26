// Package postgres implements inventory ports.Store against Postgres via
// database/sql (pgx/v5/stdlib), hand-written like the other contexts'
// adapters. A Store carries a querier (pool or *sql.Tx) so document posting
// and the pos COGS hook can run in one transaction (WithTx / InTx).
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"uuid"
)

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	pool *sql.DB // for BeginTx/InTx; nil on a tx-bound store
	q    dbtx
}

var _ ports.Store = (*Store)(nil)

func NewStore(db *sql.DB) *Store { return &Store{pool: db, q: db} }

// WithTx returns a tx-bound store (pool nil: it never begins its own tx).
func (s *Store) WithTx(tx *sql.Tx) ports.Store { return &Store{q: tx} }

func (s *Store) InTx(ctx context.Context, fn func(tx *sql.Tx, st ports.Store) error) error {
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("inventory store: begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx, s.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func dateStr(t time.Time) string { return t.Format("2006-01-02") }

// --- products ----------------------------------------------------------

const productCols = `id, restaurant_id, sku, name, type, stock_unit, menu_item_id, min_stock, archived, created_at`

func scanProduct(row interface{ Scan(...any) error }) (inv.Product, error) {
	var p inv.Product
	err := row.Scan(&p.ID, &p.RestaurantID, &p.SKU, &p.Name, &p.Type, &p.StockUnit,
		&p.MenuItemID, &p.MinStock, &p.Archived, &p.CreatedAt)
	return p, err
}

func (s *Store) InsertProduct(ctx context.Context, p inv.Product) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO inventory_products (id, restaurant_id, sku, name, type, stock_unit, menu_item_id, min_stock)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.RestaurantID, p.SKU, p.Name, p.Type, p.StockUnit, p.MenuItemID, p.MinStock)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: insert product: %w", err)
	}
	return nil
}

func (s *Store) Products(ctx context.Context, restaurantID uuid.UUID) ([]inv.Product, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+productCols+` FROM inventory_products WHERE restaurant_id = $1 ORDER BY name`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: products: %w", err)
	}
	defer rows.Close()
	out := []inv.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory store: products: scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ProductByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Product, error) {
	p, err := scanProduct(s.q.QueryRowContext(ctx, `SELECT `+productCols+` FROM inventory_products WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.Product{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.Product{}, fmt.Errorf("inventory store: product by id: %w", err)
	}
	return p, nil
}

func (s *Store) ProductByMenuItem(ctx context.Context, restaurantID, menuItemID uuid.UUID) (inv.Product, error) {
	p, err := scanProduct(s.q.QueryRowContext(ctx, `SELECT `+productCols+` FROM inventory_products WHERE restaurant_id = $1 AND menu_item_id = $2`, restaurantID, menuItemID))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.Product{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.Product{}, fmt.Errorf("inventory store: product by menu item: %w", err)
	}
	return p, nil
}

func (s *Store) ProductHasMoves(ctx context.Context, productID uuid.UUID) (bool, error) {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT count(*) FROM stock_moves WHERE product_id = $1`, productID).Scan(&n); err != nil {
		return false, fmt.Errorf("inventory store: product has moves: %w", err)
	}
	return n > 0, nil
}

func (s *Store) UpdateProduct(ctx context.Context, p inv.Product) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE inventory_products SET name = $3, stock_unit = $4, menu_item_id = $5, min_stock = $6, archived = $7
		 WHERE restaurant_id = $1 AND id = $2`,
		p.RestaurantID, p.ID, p.Name, p.StockUnit, p.MenuItemID, p.MinStock, p.Archived)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: update product: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// --- tech cards --------------------------------------------------------

const techCardCols = `id, restaurant_id, product_id, valid_from, valid_to, consumption, yield_milli, created_by, created_at,
	format, scope_note, presentation_note, storage_note, organoleptic_note`

func scanTechCard(row interface{ Scan(...any) error }) (inv.TechCard, error) {
	var t inv.TechCard
	err := row.Scan(&t.ID, &t.RestaurantID, &t.ProductID, &t.ValidFrom, &t.ValidTo,
		&t.Consumption, &t.YieldMilli, &t.CreatedBy, &t.CreatedAt,
		&t.Format, &t.ScopeNote, &t.PresentationNote, &t.StorageNote, &t.OrganolepticNote)
	return t, err
}

func (s *Store) InsertTechCard(ctx context.Context, tc inv.TechCard) error {
	format := tc.Format
	if format == "" {
		format = inv.FormatSimple
	}
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO tech_cards (id, restaurant_id, product_id, valid_from, valid_to, consumption, yield_milli, created_by,
		                         format, scope_note, presentation_note, storage_note, organoleptic_note)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		tc.ID, tc.RestaurantID, tc.ProductID, dateStr(tc.ValidFrom), nullDate(tc.ValidTo), tc.Consumption, tc.YieldMilli, tc.CreatedBy,
		format, tc.ScopeNote, tc.PresentationNote, tc.StorageNote, tc.OrganolepticNote)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: insert tech card: %w", err)
	}
	for _, l := range tc.Lines {
		yield := l.YieldPermille
		if yield <= 0 {
			yield = inv.YieldPermilleDefault
		}
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO tech_card_lines (id, tech_card_id, ingredient_product_id, qty, seq, yield_permille) VALUES ($1,$2,$3,$4,$5,$6)`,
			l.ID, tc.ID, l.IngredientProductID, l.Qty, l.Seq, yield); err != nil {
			if isUniqueViolation(err) {
				return ports.ErrConflict
			}
			return fmt.Errorf("inventory store: insert tech card line: %w", err)
		}
	}
	return nil
}

func (s *Store) CloseTechCard(ctx context.Context, id uuid.UUID, validTo time.Time) error {
	_, err := s.q.ExecContext(ctx, `UPDATE tech_cards SET valid_to = $2 WHERE id = $1`, id, dateStr(validTo))
	if err != nil {
		return fmt.Errorf("inventory store: close tech card: %w", err)
	}
	return nil
}

func (s *Store) techCardLines(ctx context.Context, techCardID uuid.UUID) ([]inv.TechCardLine, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, tech_card_id, ingredient_product_id, qty, seq, yield_permille FROM tech_card_lines WHERE tech_card_id = $1 ORDER BY seq`, techCardID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: tech card lines: %w", err)
	}
	defer rows.Close()
	out := []inv.TechCardLine{}
	for rows.Next() {
		var l inv.TechCardLine
		if err := rows.Scan(&l.ID, &l.TechCardID, &l.IngredientProductID, &l.Qty, &l.Seq, &l.YieldPermille); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) TechCardsByProduct(ctx context.Context, restaurantID, productID uuid.UUID) ([]inv.TechCard, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+techCardCols+` FROM tech_cards WHERE restaurant_id = $1 AND product_id = $2 ORDER BY valid_from DESC`, restaurantID, productID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: tech cards: %w", err)
	}
	defer rows.Close()
	out := []inv.TechCard{}
	for rows.Next() {
		t, err := scanTechCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TechCardByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.TechCard, error) {
	t, err := scanTechCard(s.q.QueryRowContext(ctx, `SELECT `+techCardCols+` FROM tech_cards WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.TechCard{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.TechCard{}, fmt.Errorf("inventory store: tech card by id: %w", err)
	}
	if t.Lines, err = s.techCardLines(ctx, id); err != nil {
		return inv.TechCard{}, err
	}
	return t, nil
}

func (s *Store) ActiveTechCard(ctx context.Context, restaurantID, productID uuid.UUID, on time.Time) (inv.TechCard, error) {
	t, err := scanTechCard(s.q.QueryRowContext(ctx,
		`SELECT `+techCardCols+` FROM tech_cards
		 WHERE restaurant_id = $1 AND product_id = $2 AND valid_from <= $3::date AND (valid_to IS NULL OR valid_to > $3::date)`,
		restaurantID, productID, dateStr(on)))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.TechCard{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.TechCard{}, fmt.Errorf("inventory store: active tech card: %w", err)
	}
	if t.Lines, err = s.techCardLines(ctx, t.ID); err != nil {
		return inv.TechCard{}, err
	}
	return t, nil
}

func (s *Store) ActiveRecipeGraph(ctx context.Context, restaurantID uuid.UUID, on time.Time) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT tc.product_id, l.ingredient_product_id
		 FROM tech_cards tc JOIN tech_card_lines l ON l.tech_card_id = tc.id
		 WHERE tc.restaurant_id = $1 AND tc.valid_from <= $2::date AND (tc.valid_to IS NULL OR tc.valid_to > $2::date)`,
		restaurantID, dateStr(on))
	if err != nil {
		return nil, fmt.Errorf("inventory store: recipe graph: %w", err)
	}
	defer rows.Close()
	adj := map[uuid.UUID][]uuid.UUID{}
	for rows.Next() {
		var p, ing uuid.UUID
		if err := rows.Scan(&p, &ing); err != nil {
			return nil, err
		}
		adj[p] = append(adj[p], ing)
	}
	return adj, rows.Err()
}

func (s *Store) InsertCosting(ctx context.Context, c inv.RecipeCosting) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO recipe_costings (id, tech_card_id, cost_cents, method, computed_by) VALUES ($1,$2,$3,$4,$5)`,
		c.ID, c.TechCardID, c.CostCents, c.Method, c.ComputedBy)
	if err != nil {
		return fmt.Errorf("inventory store: insert costing: %w", err)
	}
	return nil
}

func (s *Store) Costings(ctx context.Context, techCardID uuid.UUID) ([]inv.RecipeCosting, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, tech_card_id, cost_cents, method, computed_at, computed_by FROM recipe_costings WHERE tech_card_id = $1 ORDER BY computed_at DESC`, techCardID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: costings: %w", err)
	}
	defer rows.Close()
	out := []inv.RecipeCosting{}
	for rows.Next() {
		var c inv.RecipeCosting
		if err := rows.Scan(&c.ID, &c.TechCardID, &c.CostCents, &c.Method, &c.ComputedAt, &c.ComputedBy); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) LatestCostCents(ctx context.Context, techCardID uuid.UUID) (int64, bool, error) {
	var cost int64
	err := s.q.QueryRowContext(ctx, `SELECT cost_cents FROM recipe_costings WHERE tech_card_id = $1 ORDER BY computed_at DESC LIMIT 1`, techCardID).Scan(&cost)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("inventory store: latest cost: %w", err)
	}
	return cost, true, nil
}

// --- stock -------------------------------------------------------------

func (s *Store) InsertStockMove(ctx context.Context, m inv.StockMove) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO stock_moves (id, restaurant_id, product_id, kind, qty, cost_cents, estimated, business_date, doc_kind, doc_id, source_event_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		m.ID, m.RestaurantID, m.ProductID, m.Kind, m.QtyMilli, m.CostCents, m.Estimated, dateStr(m.BusinessDate), m.DocKind, m.DocID, m.SourceEventID)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: insert stock move: %w", err)
	}
	return nil
}

func (s *Store) LockOnHand(ctx context.Context, restaurantID, productID uuid.UUID) (inv.OnHand, error) {
	if _, err := s.q.ExecContext(ctx,
		`INSERT INTO stock_on_hand (restaurant_id, product_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		restaurantID, productID); err != nil {
		return inv.OnHand{}, fmt.Errorf("inventory store: ensure on_hand: %w", err)
	}
	var o inv.OnHand
	err := s.q.QueryRowContext(ctx,
		`SELECT restaurant_id, product_id, qty, value_cents, last_avg_cents FROM stock_on_hand
		 WHERE restaurant_id = $1 AND product_id = $2 FOR UPDATE`, restaurantID, productID).
		Scan(&o.RestaurantID, &o.ProductID, &o.QtyMilli, &o.ValueCents, &o.LastAvgCents)
	if err != nil {
		return inv.OnHand{}, fmt.Errorf("inventory store: lock on_hand: %w", err)
	}
	return o, nil
}

func (s *Store) SaveOnHand(ctx context.Context, o inv.OnHand) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO stock_on_hand (restaurant_id, product_id, qty, value_cents, last_avg_cents, updated_at)
		 VALUES ($1,$2,$3,$4,$5, now())
		 ON CONFLICT (restaurant_id, product_id)
		 DO UPDATE SET qty = EXCLUDED.qty, value_cents = EXCLUDED.value_cents, last_avg_cents = EXCLUDED.last_avg_cents, updated_at = now()`,
		o.RestaurantID, o.ProductID, o.QtyMilli, o.ValueCents, o.LastAvgCents)
	if err != nil {
		return fmt.Errorf("inventory store: save on_hand: %w", err)
	}
	return nil
}

func (s *Store) OnHand(ctx context.Context, restaurantID, productID uuid.UUID) (inv.OnHand, error) {
	o := inv.OnHand{RestaurantID: restaurantID, ProductID: productID}
	err := s.q.QueryRowContext(ctx,
		`SELECT qty, value_cents, last_avg_cents FROM stock_on_hand WHERE restaurant_id = $1 AND product_id = $2`,
		restaurantID, productID).Scan(&o.QtyMilli, &o.ValueCents, &o.LastAvgCents)
	if errors.Is(err, sql.ErrNoRows) {
		return o, nil // zero position
	}
	if err != nil {
		return inv.OnHand{}, fmt.Errorf("inventory store: on_hand: %w", err)
	}
	return o, nil
}

func (s *Store) OnHandAll(ctx context.Context, restaurantID uuid.UUID) ([]inv.OnHand, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT restaurant_id, product_id, qty, value_cents, last_avg_cents FROM stock_on_hand WHERE restaurant_id = $1`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: on_hand all: %w", err)
	}
	defer rows.Close()
	out := []inv.OnHand{}
	for rows.Next() {
		var o inv.OnHand
		if err := rows.Scan(&o.RestaurantID, &o.ProductID, &o.QtyMilli, &o.ValueCents, &o.LastAvgCents); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) MaxMoveDate(ctx context.Context, restaurantID, productID uuid.UUID) (time.Time, bool, error) {
	var d *time.Time
	err := s.q.QueryRowContext(ctx, `SELECT max(business_date)::date FROM stock_moves WHERE restaurant_id = $1 AND product_id = $2`, restaurantID, productID).Scan(&d)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("inventory store: max move date: %w", err)
	}
	if d == nil {
		return time.Time{}, false, nil
	}
	return *d, true, nil
}

const moveCols = `id, restaurant_id, product_id, kind, qty, cost_cents, estimated, business_date, recorded_at, doc_kind, doc_id, source_event_id, created_at`

func (s *Store) StockMoves(ctx context.Context, restaurantID uuid.UUID, from string, product *uuid.UUID) ([]inv.StockMove, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+moveCols+` FROM stock_moves
		 WHERE restaurant_id = $1 AND ($2 = '' OR business_date >= $2::date) AND ($3::uuid IS NULL OR product_id = $3::uuid)
		 ORDER BY business_date, recorded_at`, restaurantID, from, product)
	if err != nil {
		return nil, fmt.Errorf("inventory store: stock moves: %w", err)
	}
	defer rows.Close()
	out := []inv.StockMove{}
	for rows.Next() {
		var m inv.StockMove
		if err := rows.Scan(&m.ID, &m.RestaurantID, &m.ProductID, &m.Kind, &m.QtyMilli, &m.CostCents, &m.Estimated,
			&m.BusinessDate, &m.RecordedAt, &m.DocKind, &m.DocID, &m.SourceEventID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MovesByDoc(ctx context.Context, restaurantID uuid.UUID, docKind string, docID uuid.UUID) ([]inv.StockMove, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+moveCols+` FROM stock_moves WHERE restaurant_id = $1 AND doc_kind = $2 AND doc_id = $3 AND kind <> 'reversal'`,
		restaurantID, docKind, docID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: moves by doc: %w", err)
	}
	defer rows.Close()
	out := []inv.StockMove{}
	for rows.Next() {
		var m inv.StockMove
		if err := rows.Scan(&m.ID, &m.RestaurantID, &m.ProductID, &m.Kind, &m.QtyMilli, &m.CostCents, &m.Estimated,
			&m.BusinessDate, &m.RecordedAt, &m.DocKind, &m.DocID, &m.SourceEventID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MoveExistsBySourceEvent(ctx context.Context, sourceEventID uuid.UUID) (bool, error) {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT count(*) FROM stock_moves WHERE source_event_id = $1`, sourceEventID).Scan(&n); err != nil {
		return false, fmt.Errorf("inventory store: move exists: %w", err)
	}
	return n > 0, nil
}

func (s *Store) SaleCostByProduct(ctx context.Context, restaurantID uuid.UUID, from, to string) (map[uuid.UUID]int64, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT product_id, COALESCE(sum(abs(cost_cents)),0) FROM stock_moves
		 WHERE restaurant_id = $1 AND kind = 'sale' AND business_date >= $2::date AND business_date <= $3::date
		 GROUP BY product_id`, restaurantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("inventory store: sale cost: %w", err)
	}
	defer rows.Close()
	out := map[uuid.UUID]int64{}
	for rows.Next() {
		var p uuid.UUID
		var c int64
		if err := rows.Scan(&p, &c); err != nil {
			return nil, err
		}
		out[p] = c
	}
	return out, rows.Err()
}

func (s *Store) SaleEstimatedShare(ctx context.Context, restaurantID uuid.UUID, from, to string) (int, int, error) {
	var total, estimated int
	err := s.q.QueryRowContext(ctx,
		`SELECT count(*), count(*) FILTER (WHERE estimated) FROM stock_moves
		 WHERE restaurant_id = $1 AND kind = 'sale' AND business_date >= $2::date AND business_date <= $3::date`,
		restaurantID, from, to).Scan(&total, &estimated)
	if err != nil {
		return 0, 0, fmt.Errorf("inventory store: estimated share: %w", err)
	}
	return total, estimated, nil
}

// --- suppliers ---------------------------------------------------------

func (s *Store) InsertSupplier(ctx context.Context, sup inv.Supplier) error {
	contacts, _ := json.Marshal(orEmpty(sup.Contacts))
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO suppliers (id, restaurant_id, name, contacts, note) VALUES ($1,$2,$3,$4,$5)`,
		sup.ID, sup.RestaurantID, sup.Name, contacts, sup.Note)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: insert supplier: %w", err)
	}
	return nil
}

func scanSupplier(row interface{ Scan(...any) error }) (inv.Supplier, error) {
	var sup inv.Supplier
	var contacts []byte
	if err := row.Scan(&sup.ID, &sup.RestaurantID, &sup.Name, &contacts, &sup.Note, &sup.Archived, &sup.CreatedAt); err != nil {
		return inv.Supplier{}, err
	}
	_ = json.Unmarshal(contacts, &sup.Contacts)
	return sup, nil
}

func (s *Store) Suppliers(ctx context.Context, restaurantID uuid.UUID) ([]inv.Supplier, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, restaurant_id, name, contacts, note, archived, created_at FROM suppliers WHERE restaurant_id = $1 ORDER BY name`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("inventory store: suppliers: %w", err)
	}
	defer rows.Close()
	out := []inv.Supplier{}
	for rows.Next() {
		sup, err := scanSupplier(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sup)
	}
	return out, rows.Err()
}

func (s *Store) SupplierByID(ctx context.Context, restaurantID, id uuid.UUID) (inv.Supplier, error) {
	sup, err := scanSupplier(s.q.QueryRowContext(ctx, `SELECT id, restaurant_id, name, contacts, note, archived, created_at FROM suppliers WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return inv.Supplier{}, ports.ErrNotFound
	}
	if err != nil {
		return inv.Supplier{}, fmt.Errorf("inventory store: supplier by id: %w", err)
	}
	return sup, nil
}

func (s *Store) UpdateSupplier(ctx context.Context, sup inv.Supplier) error {
	contacts, _ := json.Marshal(orEmpty(sup.Contacts))
	res, err := s.q.ExecContext(ctx,
		`UPDATE suppliers SET name = $3, contacts = $4, note = $5, archived = $6 WHERE restaurant_id = $1 AND id = $2`,
		sup.RestaurantID, sup.ID, sup.Name, contacts, sup.Note, sup.Archived)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return fmt.Errorf("inventory store: update supplier: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// --- helpers -----------------------------------------------------------

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func nullDate(t *time.Time) any {
	if t == nil {
		return nil
	}
	return dateStr(*t)
}
