package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const menuCols = `id, restaurant_id, slug, name, position, is_default`

func scanMenu(row interface{ Scan(...any) error }) (domain.Menu, error) {
	var m domain.Menu
	err := row.Scan(&m.ID, &m.RestaurantID, &m.Slug, &m.Name, &m.Position, &m.IsDefault)
	return m, err
}

func (s *PostgresStore) Menus(ctx context.Context, restaurantID uuid.UUID) ([]domain.Menu, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+menuCols+` FROM menus WHERE restaurant_id = $1
		 ORDER BY is_default DESC, position ASC, name ASC`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("store: menus: %w", err)
	}
	defer rows.Close()

	menus := []domain.Menu{}
	for rows.Next() {
		m, err := scanMenu(rows)
		if err != nil {
			return nil, fmt.Errorf("store: menus: scan: %w", err)
		}
		menus = append(menus, m)
	}
	return menus, rows.Err()
}

func (s *PostgresStore) MenuBySlug(ctx context.Context, restaurantID uuid.UUID, slug string) (domain.Menu, error) {
	m, err := scanMenu(s.db.QueryRowContext(ctx,
		`SELECT `+menuCols+` FROM menus WHERE restaurant_id = $1 AND slug = $2`, restaurantID, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Menu{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Menu{}, fmt.Errorf("store: menu by slug: %w", err)
	}
	return m, nil
}

func (s *PostgresStore) menuByID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, restaurantID, id uuid.UUID) (domain.Menu, error) {
	m, err := scanMenu(q.QueryRowContext(ctx,
		`SELECT `+menuCols+` FROM menus WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Menu{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Menu{}, fmt.Errorf("store: menu by id: %w", err)
	}
	return m, nil
}

func (s *PostgresStore) CreateMenu(ctx context.Context, m domain.Menu) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO menus (id, restaurant_id, slug, name, position, is_default)
		 VALUES ($1, $2, $3, $4, $5, false)`,
		m.ID, m.RestaurantID, m.Slug, m.Name, m.Position)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("menu slug taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: create menu: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateMenu(ctx context.Context, m domain.Menu) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: update menu: begin: %w", err)
	}
	defer tx.Rollback()

	current, err := s.menuByID(ctx, tx, m.RestaurantID, m.ID)
	if err != nil {
		return err
	}
	// The default flag never comes off by editing the default itself —
	// promote another menu instead (keeps "exactly one default" true).
	if current.IsDefault && !m.IsDefault {
		return fmt.Errorf("pick a new default menu instead of clearing this one: %w", ports.ErrConflict)
	}
	if m.IsDefault && !current.IsDefault {
		if _, err := tx.ExecContext(ctx,
			`UPDATE menus SET is_default = false WHERE restaurant_id = $1 AND is_default`, m.RestaurantID); err != nil {
			return fmt.Errorf("store: update menu: clear default: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE menus SET slug = $1, name = $2, position = $3, is_default = $4
		 WHERE restaurant_id = $5 AND id = $6`,
		m.Slug, m.Name, m.Position, m.IsDefault, m.RestaurantID, m.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("menu slug taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: update menu: %w", err)
	}
	return tx.Commit()
}

func (s *PostgresStore) DeleteMenu(ctx context.Context, restaurantID, id uuid.UUID, force bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete menu: begin: %w", err)
	}
	defer tx.Rollback()

	m, err := s.menuByID(ctx, tx, restaurantID, id)
	if err != nil {
		return err
	}
	var totalMenus, categoryCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM menus WHERE restaurant_id = $1`, restaurantID).Scan(&totalMenus); err != nil {
		return fmt.Errorf("store: delete menu: count menus: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM categories WHERE menu_id = $1`, id).Scan(&categoryCount); err != nil {
		return fmt.Errorf("store: delete menu: count categories: %w", err)
	}
	if err := domain.CanDeleteMenu(m, totalMenus, categoryCount, force); err != nil {
		return err
	}
	// Categories (and their items) cascade via FK. Items referenced by
	// historical order/ticket lines block the cascade (RESTRICT), same
	// as DeleteMenuItem.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM menus WHERE restaurant_id = $1 AND id = $2`, restaurantID, id); err != nil {
		if isFKViolation(err) {
			return ports.ErrItemReferenced
		}
		return fmt.Errorf("store: delete menu: %w", err)
	}
	return tx.Commit()
}
