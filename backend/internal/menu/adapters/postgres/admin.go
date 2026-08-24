package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"

	"github.com/jackc/pgx/v5/pgconn"
	"uuid"
)

var _ ports.AdminStore = (*PostgresStore)(nil)

// isFKViolation reports whether err is a Postgres foreign-key violation
// (SQLSTATE 23503).
func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (s *PostgresStore) RestaurantByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error) {
	var r domain.Restaurant
	err := s.db.QueryRowContext(ctx,
		`SELECT id, slug, name, created_at FROM restaurants WHERE id = $1`, id,
	).Scan(&r.ID, &r.Slug, &r.Name, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Restaurant{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("store: restaurant by id: %w", err)
	}
	return r, nil
}

func (s *PostgresStore) TableByTokenGlobal(ctx context.Context, token string) (domain.Table, error) {
	var t domain.Table
	err := s.db.QueryRowContext(ctx,
		`SELECT id, restaurant_id, label, token, created_at FROM tables WHERE token = $1`, token,
	).Scan(&t.ID, &t.RestaurantID, &t.Label, &t.Token, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Table{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Table{}, fmt.Errorf("store: table by token: %w", err)
	}
	return t, nil
}

func (s *PostgresStore) Tables(ctx context.Context, restaurantID uuid.UUID) ([]domain.Table, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, label, token, created_at
		 FROM tables WHERE restaurant_id = $1 ORDER BY label ASC`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("store: tables: %w", err)
	}
	defer rows.Close()

	tables := []domain.Table{}
	for rows.Next() {
		var t domain.Table
		if err := rows.Scan(&t.ID, &t.RestaurantID, &t.Label, &t.Token, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: tables: scan: %w", err)
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (s *PostgresStore) TableByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Table, error) {
	var t domain.Table
	err := s.db.QueryRowContext(ctx,
		`SELECT id, restaurant_id, label, token, created_at
		 FROM tables WHERE restaurant_id = $1 AND id = $2`, restaurantID, id,
	).Scan(&t.ID, &t.RestaurantID, &t.Label, &t.Token, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Table{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Table{}, fmt.Errorf("store: table by id: %w", err)
	}
	return t, nil
}

func (s *PostgresStore) CreateTable(ctx context.Context, t domain.Table) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tables (id, restaurant_id, label, token) VALUES ($1, $2, $3, $4)`,
		t.ID, t.RestaurantID, t.Label, t.Token)
	if err != nil {
		return fmt.Errorf("store: create table: %w", err)
	}
	return nil
}

func (s *PostgresStore) RegenerateTableToken(ctx context.Context, restaurantID, id uuid.UUID, newToken string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tables SET token = $1 WHERE restaurant_id = $2 AND id = $3`,
		newToken, restaurantID, id)
	if err != nil {
		return fmt.Errorf("store: regenerate token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) CreateCategory(ctx context.Context, c domain.Category) error {
	// menu_id is written via a scoped subquery so a category can never
	// land under another restaurant's menu.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO categories (id, restaurant_id, menu_id, name, position)
		 SELECT $1, $2, m.id, $4, $5 FROM menus m WHERE m.id = $3 AND m.restaurant_id = $2`,
		c.ID, c.RestaurantID, c.MenuID, c.Name, c.Position)
	if err != nil {
		return fmt.Errorf("store: create category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound // menu_id not under this restaurant
	}
	return nil
}

func (s *PostgresStore) UpdateCategory(ctx context.Context, c domain.Category) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE categories SET name = $1, position = $2 WHERE restaurant_id = $3 AND id = $4`,
		c.Name, c.Position, c.RestaurantID, c.ID)
	if err != nil {
		return fmt.Errorf("store: update category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteCategory(ctx context.Context, restaurantID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM categories WHERE restaurant_id = $1 AND id = $2`, restaurantID, id)
	if err != nil {
		if isFKViolation(err) {
			return ports.ErrItemReferenced
		}
		return fmt.Errorf("store: delete category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) CreateMenuItem(ctx context.Context, it domain.MenuItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create item: begin: %w", err)
	}
	defer tx.Rollback()

	// category_id is written via a scoped subquery so an item can never
	// land under another restaurant's category (tenant isolation — same
	// pattern as CreateCategory's menu check).
	res, err := tx.ExecContext(ctx,
		`INSERT INTO menu_items (id, restaurant_id, category_id, name, description, price_cents, image_url, available)
		 SELECT $1, $2, c.id, $4, $5, $6, $7, $8
		 FROM categories c WHERE c.id = $3 AND c.restaurant_id = $2`,
		it.ID, it.RestaurantID, it.CategoryID, it.Name, it.Description, it.PriceCents, it.ImageURL, it.Available)
	if err != nil {
		return fmt.Errorf("store: create item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound // category not under this restaurant
	}
	if err := insertItemCollections(ctx, tx, it); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) UpdateMenuItem(ctx context.Context, it domain.MenuItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: update item: begin: %w", err)
	}
	defer tx.Rollback()

	// Same tenant scoping as CreateMenuItem: the target category must
	// belong to this restaurant or the update matches zero rows.
	res, err := tx.ExecContext(ctx,
		`UPDATE menu_items SET category_id = $1, name = $2, description = $3,
		        price_cents = $4, image_url = $5, available = $6
		 WHERE restaurant_id = $7 AND id = $8
		   AND EXISTS (SELECT 1 FROM categories c WHERE c.id = $1 AND c.restaurant_id = $7)`,
		it.CategoryID, it.Name, it.Description, it.PriceCents, it.ImageURL, it.Available,
		it.RestaurantID, it.ID)
	if err != nil {
		return fmt.Errorf("store: update item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}

	// Replace collections wholesale. Order/ticket lines snapshot labels
	// and prices, so dropping options never rewrites history.
	if _, err := tx.ExecContext(ctx, `DELETE FROM menu_item_allergens WHERE menu_item_id = $1`, it.ID); err != nil {
		return fmt.Errorf("store: update item: clear allergens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM option_groups WHERE menu_item_id = $1`, it.ID); err != nil {
		return fmt.Errorf("store: update item: clear option groups: %w", err)
	}
	if err := insertItemCollections(ctx, tx, it); err != nil {
		return err
	}
	return tx.Commit()
}

func insertItemCollections(ctx context.Context, tx *sql.Tx, it domain.MenuItem) error {
	for _, a := range it.Allergens {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO menu_item_allergens (menu_item_id, allergen) VALUES ($1, $2)`, it.ID, a); err != nil {
			return fmt.Errorf("store: item allergen: %w", err)
		}
	}
	for gi, g := range it.OptionGroups {
		if g.ID == uuid.Nil() {
			g.ID = uuid.New()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO option_groups (id, menu_item_id, name, multi, position) VALUES ($1, $2, $3, $4, $5)`,
			g.ID, it.ID, g.Name, g.Multi, gi); err != nil {
			return fmt.Errorf("store: option group: %w", err)
		}
		for oi, o := range g.Options {
			if o.ID == uuid.Nil() {
				o.ID = uuid.New()
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO options (id, option_group_id, label, price_delta_cents, position) VALUES ($1, $2, $3, $4, $5)`,
				o.ID, g.ID, o.Label, o.PriceDeltaCents, oi); err != nil {
				return fmt.Errorf("store: option: %w", err)
			}
		}
	}
	return nil
}

func (s *PostgresStore) DeleteMenuItem(ctx context.Context, restaurantID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM menu_items WHERE restaurant_id = $1 AND id = $2`, restaurantID, id)
	if err != nil {
		if isFKViolation(err) {
			return ports.ErrItemReferenced
		}
		return fmt.Errorf("store: delete item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) MenuItemByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.MenuItem, error) {
	var m domain.MenuItem
	err := s.db.QueryRowContext(ctx,
		`SELECT id, restaurant_id, category_id, name, description, price_cents, image_url, available
		 FROM menu_items WHERE restaurant_id = $1 AND id = $2`, restaurantID, id,
	).Scan(&m.ID, &m.RestaurantID, &m.CategoryID, &m.Name, &m.Description, &m.PriceCents, &m.ImageURL, &m.Available)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MenuItem{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.MenuItem{}, fmt.Errorf("store: item by id: %w", err)
	}
	items := []domain.MenuItem{m}
	index := map[uuid.UUID]int{m.ID: 0}
	if err := s.attachAllergens(ctx, restaurantID, items, index); err != nil {
		return domain.MenuItem{}, err
	}
	if err := s.attachOptionGroups(ctx, restaurantID, items, index); err != nil {
		return domain.MenuItem{}, err
	}
	return items[0], nil
}

func (s *PostgresStore) CountMenuItems(ctx context.Context, restaurantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM menu_items WHERE restaurant_id = $1`, restaurantID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count items: %w", err)
	}
	return n, nil
}

func (s *PostgresStore) PendingServiceRequests(ctx context.Context, restaurantID uuid.UUID) ([]domain.ServiceRequest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, table_id, kind, status, created_at
		 FROM service_requests WHERE restaurant_id = $1 AND status = $2
		 ORDER BY created_at ASC`, restaurantID, domain.ServiceRequestPending)
	if err != nil {
		return nil, fmt.Errorf("store: pending requests: %w", err)
	}
	defer rows.Close()

	reqs := []domain.ServiceRequest{}
	for rows.Next() {
		var r domain.ServiceRequest
		if err := rows.Scan(&r.ID, &r.RestaurantID, &r.TableID, &r.Kind, &r.Status, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: pending requests: scan: %w", err)
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

func (s *PostgresStore) PendingServiceRequestsForTable(ctx context.Context, restaurantID, tableID uuid.UUID) ([]domain.ServiceRequest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, table_id, kind, status, created_at
		 FROM service_requests WHERE restaurant_id = $1 AND table_id = $2 AND status = $3
		 ORDER BY created_at ASC`, restaurantID, tableID, domain.ServiceRequestPending)
	if err != nil {
		return nil, fmt.Errorf("store: pending requests for table: %w", err)
	}
	defer rows.Close()

	reqs := []domain.ServiceRequest{}
	for rows.Next() {
		var r domain.ServiceRequest
		if err := rows.Scan(&r.ID, &r.RestaurantID, &r.TableID, &r.Kind, &r.Status, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: pending requests for table: scan: %w", err)
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

func (s *PostgresStore) SetServiceRequestStatus(ctx context.Context, restaurantID, id uuid.UUID, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE service_requests SET status = $1 WHERE id = $2 AND restaurant_id = $3`,
		status, id, restaurantID)
	if err != nil {
		return fmt.Errorf("store: set request status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}
