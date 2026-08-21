// Package postgres implements ports.Store against a Postgres database via
// database/sql, using pgx/v5/stdlib as the driver.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// PostgresStore implements ports.Store against a Postgres database via
// database/sql, using pgx/v5/stdlib as the driver.
type PostgresStore struct {
	db *sql.DB
}

var _ ports.Store = (*PostgresStore)(nil)

// NewPostgresStore opens a connection pool for dsn and verifies it with a
// ping.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

// NewPostgresStoreFromDB wraps an already-opened handle, so one process
// can share a single pool across contexts (see cmd/aivo-server).
func NewPostgresStoreFromDB(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) RestaurantBySlug(ctx context.Context, slug string) (domain.Restaurant, error) {
	var r domain.Restaurant
	err := s.db.QueryRowContext(ctx,
		`SELECT id, slug, name, created_at FROM restaurants WHERE slug = $1`,
		slug,
	).Scan(&r.ID, &r.Slug, &r.Name, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Restaurant{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("store: restaurant by slug: %w", err)
	}
	return r, nil
}

func (s *PostgresStore) TableByToken(ctx context.Context, restaurantID uuid.UUID, token string) (domain.Table, error) {
	var t domain.Table
	err := s.db.QueryRowContext(ctx,
		`SELECT id, restaurant_id, label, token, created_at
		 FROM tables WHERE restaurant_id = $1 AND token = $2`,
		restaurantID, token,
	).Scan(&t.ID, &t.RestaurantID, &t.Label, &t.Token, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Table{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Table{}, fmt.Errorf("store: table by token: %w", err)
	}
	return t, nil
}

func (s *PostgresStore) LandingBlocks(ctx context.Context, restaurantID uuid.UUID) ([]domain.LandingBlock, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, type, position, data
		 FROM landing_blocks WHERE restaurant_id = $1 ORDER BY position ASC`,
		restaurantID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: landing blocks: %w", err)
	}
	defer rows.Close()

	blocks := []domain.LandingBlock{}
	for rows.Next() {
		var b domain.LandingBlock
		var data []byte
		if err := rows.Scan(&b.ID, &b.RestaurantID, &b.Type, &b.Position, &data); err != nil {
			return nil, fmt.Errorf("store: landing blocks: scan: %w", err)
		}
		if err := json.Unmarshal(data, &b.Data); err != nil {
			return nil, fmt.Errorf("store: landing blocks: decode data: %w", err)
		}
		blocks = append(blocks, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: landing blocks: %w", err)
	}
	return blocks, nil
}

func (s *PostgresStore) Menu(ctx context.Context, restaurantID uuid.UUID) ([]domain.Category, []domain.MenuItem, error) {
	categories, err := s.categories(ctx, restaurantID)
	if err != nil {
		return nil, nil, err
	}

	items, itemIndex, err := s.menuItems(ctx, restaurantID)
	if err != nil {
		return nil, nil, err
	}

	if err := s.attachAllergens(ctx, restaurantID, items, itemIndex); err != nil {
		return nil, nil, err
	}
	if err := s.attachOptionGroups(ctx, restaurantID, items, itemIndex); err != nil {
		return nil, nil, err
	}

	return categories, items, nil
}

func (s *PostgresStore) categories(ctx context.Context, restaurantID uuid.UUID) ([]domain.Category, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, menu_id, name, position
		 FROM categories WHERE restaurant_id = $1 ORDER BY position ASC`,
		restaurantID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: categories: %w", err)
	}
	defer rows.Close()

	categories := []domain.Category{}
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.RestaurantID, &c.MenuID, &c.Name, &c.Position); err != nil {
			return nil, fmt.Errorf("store: categories: scan: %w", err)
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: categories: %w", err)
	}
	return categories, nil
}

// menuItems returns every MenuItem for restaurantID (Allergens/OptionGroups
// left empty for the caller to populate) plus an index from item ID to its
// position in the returned slice, for attachAllergens/attachOptionGroups to
// fill in place without a second lookup pass.
func (s *PostgresStore) menuItems(ctx context.Context, restaurantID uuid.UUID) ([]domain.MenuItem, map[uuid.UUID]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, restaurant_id, category_id, name, description, price_cents, image_url, available
		 FROM menu_items WHERE restaurant_id = $1 ORDER BY name ASC`,
		restaurantID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("store: menu items: %w", err)
	}
	defer rows.Close()

	items := []domain.MenuItem{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var m domain.MenuItem
		if err := rows.Scan(&m.ID, &m.RestaurantID, &m.CategoryID, &m.Name, &m.Description, &m.PriceCents, &m.ImageURL, &m.Available); err != nil {
			return nil, nil, fmt.Errorf("store: menu items: scan: %w", err)
		}
		index[m.ID] = len(items)
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: menu items: %w", err)
	}
	return items, index, nil
}

// attachAllergens populates items[i].Allergens, joining through menu_items
// so the query stays scoped to restaurantID even though
// menu_item_allergens carries no restaurant_id column itself.
func (s *PostgresStore) attachAllergens(ctx context.Context, restaurantID uuid.UUID, items []domain.MenuItem, index map[uuid.UUID]int) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mia.menu_item_id, mia.allergen
		 FROM menu_item_allergens mia
		 JOIN menu_items mi ON mi.id = mia.menu_item_id
		 WHERE mi.restaurant_id = $1`,
		restaurantID,
	)
	if err != nil {
		return fmt.Errorf("store: allergens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID uuid.UUID
		var allergen string
		if err := rows.Scan(&itemID, &allergen); err != nil {
			return fmt.Errorf("store: allergens: scan: %w", err)
		}
		i, ok := index[itemID]
		if !ok {
			continue
		}
		items[i].Allergens = append(items[i].Allergens, domain.Allergen(allergen))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: allergens: %w", err)
	}
	return nil
}

// attachOptionGroups populates items[i].OptionGroups (each with its
// Options), joining through menu_items/option_groups so both queries stay
// scoped to restaurantID.
func (s *PostgresStore) attachOptionGroups(ctx context.Context, restaurantID uuid.UUID, items []domain.MenuItem, index map[uuid.UUID]int) error {
	groupRows, err := s.db.QueryContext(ctx,
		`SELECT og.id, og.menu_item_id, og.name, og.multi
		 FROM option_groups og
		 JOIN menu_items mi ON mi.id = og.menu_item_id
		 WHERE mi.restaurant_id = $1
		 ORDER BY og.menu_item_id, og.position ASC`,
		restaurantID,
	)
	if err != nil {
		return fmt.Errorf("store: option groups: %w", err)
	}
	defer groupRows.Close()

	// groupIndex maps option_group_id -> (item index, group index) so
	// attachOptions below can append Options in place.
	groupIndex := map[uuid.UUID][2]int{}
	for groupRows.Next() {
		var groupID, itemID uuid.UUID
		var g domain.OptionGroup
		if err := groupRows.Scan(&groupID, &itemID, &g.Name, &g.Multi); err != nil {
			return fmt.Errorf("store: option groups: scan: %w", err)
		}
		g.ID = groupID
		i, ok := index[itemID]
		if !ok {
			continue
		}
		items[i].OptionGroups = append(items[i].OptionGroups, g)
		groupIndex[groupID] = [2]int{i, len(items[i].OptionGroups) - 1}
	}
	if err := groupRows.Err(); err != nil {
		return fmt.Errorf("store: option groups: %w", err)
	}

	optRows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.option_group_id, o.label, o.price_delta_cents
		 FROM options o
		 JOIN option_groups og ON og.id = o.option_group_id
		 JOIN menu_items mi ON mi.id = og.menu_item_id
		 WHERE mi.restaurant_id = $1
		 ORDER BY o.option_group_id, o.position ASC`,
		restaurantID,
	)
	if err != nil {
		return fmt.Errorf("store: options: %w", err)
	}
	defer optRows.Close()

	for optRows.Next() {
		var groupID uuid.UUID
		var o domain.Option
		if err := optRows.Scan(&o.ID, &groupID, &o.Label, &o.PriceDeltaCents); err != nil {
			return fmt.Errorf("store: options: scan: %w", err)
		}
		pos, ok := groupIndex[groupID]
		if !ok {
			continue
		}
		itemIdx, groupIdx := pos[0], pos[1]
		items[itemIdx].OptionGroups[groupIdx].Options = append(items[itemIdx].OptionGroups[groupIdx].Options, o)
	}
	if err := optRows.Err(); err != nil {
		return fmt.Errorf("store: options: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if len(order.Lines) == 0 {
		return domain.Order{}, fmt.Errorf("store: create order: no lines")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Order{}, fmt.Errorf("store: create order: begin: %w", err)
	}
	defer tx.Rollback()

	order.ID = uuid.New()
	order.CreatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO orders (id, restaurant_id, table_id, comment, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		order.ID, order.RestaurantID, order.TableID, order.Comment, order.CreatedAt,
	)
	if err != nil {
		return domain.Order{}, fmt.Errorf("store: create order: insert order: %w", err)
	}

	for _, line := range order.Lines {
		lineID := uuid.New()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO order_lines (id, order_id, menu_item_id, name, unit_price_cents, qty)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			lineID, order.ID, line.MenuItemID, line.Name, line.UnitPriceCents, line.Qty,
		)
		if err != nil {
			return domain.Order{}, fmt.Errorf("store: create order: insert line: %w", err)
		}

		for _, opt := range line.ChosenOptions {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO order_line_options (id, order_line_id, label, price_delta_cents)
				 VALUES ($1, $2, $3, $4)`,
				uuid.New(), lineID, opt.Label, opt.PriceDeltaCents,
			)
			if err != nil {
				return domain.Order{}, fmt.Errorf("store: create order: insert line option: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Order{}, fmt.Errorf("store: create order: commit: %w", err)
	}
	return order, nil
}

func (s *PostgresStore) CreateServiceRequest(ctx context.Context, req domain.ServiceRequest) (domain.ServiceRequest, error) {
	req.ID = uuid.New()
	req.Status = domain.ServiceRequestPending
	req.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO service_requests (id, restaurant_id, table_id, kind, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		req.ID, req.RestaurantID, req.TableID, req.Kind, req.Status, req.CreatedAt,
	)
	if err != nil {
		return domain.ServiceRequest{}, fmt.Errorf("store: create service request: %w", err)
	}
	return req, nil
}

func (s *PostgresStore) HasOpenServiceRequest(ctx context.Context, tableID uuid.UUID, kind domain.ServiceRequestKind) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM service_requests
		   WHERE table_id = $1 AND kind = $2 AND status = $3
		 )`,
		tableID, kind, domain.ServiceRequestPending,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: has open service request: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) AcknowledgeServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE service_requests SET status = $1 WHERE id = $2 AND restaurant_id = $3`,
		domain.ServiceRequestAcknowledged, id, restaurantID,
	)
	if err != nil {
		return fmt.Errorf("store: acknowledge service request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: acknowledge service request: rows affected: %w", err)
	}
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) NotificationChannel(ctx context.Context, restaurantID uuid.UUID) (domain.NotificationChannel, error) {
	var ch domain.NotificationChannel
	err := s.db.QueryRowContext(ctx,
		`SELECT restaurant_id, telegram_chat_id, encrypted_bot_token, key_version
		 FROM notification_channels WHERE restaurant_id = $1`,
		restaurantID,
	).Scan(&ch.RestaurantID, &ch.TelegramChatID, &ch.EncryptedBotToken, &ch.KeyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NotificationChannel{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.NotificationChannel{}, fmt.Errorf("store: notification channel: %w", err)
	}
	return ch, nil
}

func (s *PostgresStore) SaveNotificationChannel(ctx context.Context, ch domain.NotificationChannel) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_channels (restaurant_id, telegram_chat_id, encrypted_bot_token, key_version)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (restaurant_id) DO UPDATE
		 SET telegram_chat_id = EXCLUDED.telegram_chat_id,
		     encrypted_bot_token = EXCLUDED.encrypted_bot_token,
		     key_version = EXCLUDED.key_version`,
		ch.RestaurantID, ch.TelegramChatID, ch.EncryptedBotToken, ch.KeyVersion,
	)
	if err != nil {
		return fmt.Errorf("store: save notification channel: %w", err)
	}
	return nil
}
