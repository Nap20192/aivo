// Package postgres implements platform ports.Store against Postgres via
// database/sql with the pgx stdlib driver, same as the menu adapter.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

type Store struct {
	db *sql.DB
}

var _ ports.Store = (*Store)(nil)

// NewStore wraps an already-opened database handle (shared with the
// other contexts' stores — one pool per process).
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Store) CreateOrgWithOwner(ctx context.Context, org domain.Organization, owner domain.User, sub domain.Subscription, rest domain.Restaurant) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: register: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO organizations (id, name) VALUES ($1, $2)`, org.ID, org.Name); err != nil {
		return fmt.Errorf("store: register: org: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, $4)`,
		rest.ID, org.ID, rest.Slug, rest.Name); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("slug taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: register: restaurant: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, org_id, email, password_hash, role, restaurant_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		owner.ID, org.ID, owner.Email, owner.PasswordHash, owner.Role, owner.RestaurantID); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("email taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: register: owner: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO subscriptions (org_id, plan, status) VALUES ($1, $2, $3)`,
		org.ID, sub.Plan, sub.Status); err != nil {
		return fmt.Errorf("store: register: subscription: %w", err)
	}
	return tx.Commit()
}

func (s *Store) Organization(ctx context.Context, orgID uuid.UUID) (domain.Organization, error) {
	var o domain.Organization
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM organizations WHERE id = $1`, orgID,
	).Scan(&o.ID, &o.Name, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Organization{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Organization{}, fmt.Errorf("store: org: %w", err)
	}
	return o, nil
}

func (s *Store) UpdateOrganization(ctx context.Context, org domain.Organization) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE organizations SET name = $1 WHERE id = $2`, org.Name, org.ID)
	if err != nil {
		return fmt.Errorf("store: update org: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

const userCols = `id, org_id, email, password_hash, role, restaurant_id, created_at`

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.Role, &u.RestaurantID, &u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE email = $1`, email))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("store: user by email: %w", err)
	}
	return u, nil
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("store: user by id: %w", err)
	}
	return u, nil
}

func (s *Store) CreateUser(ctx context.Context, u domain.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, org_id, email, password_hash, role, restaurant_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, u.OrgID, u.Email, u.PasswordHash, u.Role, u.RestaurantID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("email taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: create user: %w", err)
	}
	return nil
}

func (s *Store) StaffForRestaurant(ctx context.Context, orgID, restaurantID uuid.UUID) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userCols+` FROM users
		 WHERE org_id = $1 AND (restaurant_id = $2 OR restaurant_id IS NULL)
		 ORDER BY created_at ASC`, orgID, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("store: staff: %w", err)
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("store: staff: scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) CreateSession(ctx context.Context, sess domain.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		sess.TokenHash, sess.UserID, sess.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT u.id, u.org_id, u.email, u.password_hash, u.role, u.restaurant_id, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("store: session user: %w", err)
	}
	return u, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

func (s *Store) Subscription(ctx context.Context, orgID uuid.UUID) (domain.Subscription, error) {
	var sub domain.Subscription
	err := s.db.QueryRowContext(ctx,
		`SELECT org_id, plan, status, updated_at FROM subscriptions WHERE org_id = $1`, orgID,
	).Scan(&sub.OrgID, &sub.Plan, &sub.Status, &sub.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Subscription{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: subscription: %w", err)
	}
	return sub, nil
}

func (s *Store) SaveSubscription(ctx context.Context, sub domain.Subscription) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (org_id, plan, status, updated_at) VALUES ($1, $2, $3, now())
		 ON CONFLICT (org_id) DO UPDATE SET plan = EXCLUDED.plan, status = EXCLUDED.status, updated_at = now()`,
		sub.OrgID, sub.Plan, sub.Status)
	if err != nil {
		return fmt.Errorf("store: save subscription: %w", err)
	}
	return nil
}

const restCols = `id, org_id, slug, name, address, hours, contacts, created_at`

func scanRestaurant(row interface{ Scan(...any) error }) (domain.Restaurant, error) {
	var r domain.Restaurant
	var contacts []byte
	err := row.Scan(&r.ID, &r.OrgID, &r.Slug, &r.Name, &r.Address, &r.Hours, &contacts, &r.CreatedAt)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(contacts, &r.Contacts); err != nil {
		return r, fmt.Errorf("decode contacts: %w", err)
	}
	return r, nil
}

func (s *Store) CreateRestaurant(ctx context.Context, r domain.Restaurant) error {
	contacts, err := json.Marshal(orEmpty(r.Contacts))
	if err != nil {
		return fmt.Errorf("store: create restaurant: encode contacts: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO restaurants (id, org_id, slug, name, address, hours, contacts)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, r.OrgID, r.Slug, r.Name, r.Address, r.Hours, contacts)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("slug taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: create restaurant: %w", err)
	}
	return nil
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func (s *Store) Restaurants(ctx context.Context, orgID uuid.UUID) ([]domain.Restaurant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+restCols+` FROM restaurants WHERE org_id = $1 ORDER BY created_at ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: restaurants: %w", err)
	}
	defer rows.Close()

	out := []domain.Restaurant{}
	for rows.Next() {
		r, err := scanRestaurant(rows)
		if err != nil {
			return nil, fmt.Errorf("store: restaurants: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Restaurant(ctx context.Context, orgID, id uuid.UUID) (domain.Restaurant, error) {
	r, err := scanRestaurant(s.db.QueryRowContext(ctx,
		`SELECT `+restCols+` FROM restaurants WHERE org_id = $1 AND id = $2`, orgID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Restaurant{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("store: restaurant: %w", err)
	}
	return r, nil
}

func (s *Store) UpdateRestaurant(ctx context.Context, r domain.Restaurant) error {
	contacts, err := json.Marshal(orEmpty(r.Contacts))
	if err != nil {
		return fmt.Errorf("store: update restaurant: encode contacts: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE restaurants SET slug = $1, name = $2, address = $3, hours = $4, contacts = $5
		 WHERE org_id = $6 AND id = $7`,
		r.Slug, r.Name, r.Address, r.Hours, contacts, r.OrgID, r.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("slug taken: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: update restaurant: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *Store) CountRestaurants(ctx context.Context, orgID uuid.UUID) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM restaurants WHERE org_id = $1`, orgID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count restaurants: %w", err)
	}
	return n, nil
}

func (s *Store) Theme(ctx context.Context, restaurantID uuid.UUID) (domain.Theme, error) {
	var t domain.Theme
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT restaurant_id, theme, design_md, updated_at FROM restaurant_themes WHERE restaurant_id = $1`,
		restaurantID,
	).Scan(&t.RestaurantID, &raw, &t.DesignMD, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// No row yet = empty theme, not an error: every restaurant has a
		// theme conceptually, we just store it lazily.
		return domain.Theme{RestaurantID: restaurantID, ThemeJSON: json.RawMessage(`{}`), UpdatedAt: time.Time{}}, nil
	}
	if err != nil {
		return domain.Theme{}, fmt.Errorf("store: theme: %w", err)
	}
	t.ThemeJSON = raw
	return t, nil
}

func (s *Store) SaveTheme(ctx context.Context, t domain.Theme) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO restaurant_themes (restaurant_id, theme, design_md, updated_at) VALUES ($1, $2, $3, now())
		 ON CONFLICT (restaurant_id) DO UPDATE SET theme = EXCLUDED.theme, design_md = EXCLUDED.design_md, updated_at = now()`,
		t.RestaurantID, []byte(t.ThemeJSON), t.DesignMD)
	if err != nil {
		return fmt.Errorf("store: save theme: %w", err)
	}
	return nil
}

func (s *Store) RestaurantIDByDomain(ctx context.Context, host string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`SELECT restaurant_id FROM custom_domains WHERE domain = $1 AND verified_at IS NOT NULL`, host,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ports.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: domain: %w", err)
	}
	return id, nil
}
