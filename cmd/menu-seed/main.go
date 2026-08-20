// Command seed inserts one demo Restaurant (see CONTEXT.md) into Postgres,
// so the Menu app has something to browse end-to-end without a real admin
// API/UI (MVP has none — see AGENTS.md). Reads DATABASE_URL and
// TOKEN_ENCRYPTION_KEY from env; TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID are
// optional (the NotificationChannel is skipped, with a warning, if either
// is unset). Not idempotent: re-running against an already-seeded database
// fails on the restaurants.slug unique constraint.
package main

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"aivo/internal/menu/adapters/postgres"
	"aivo/internal/menu/domain"
	"aivo/pkg/crypto"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("seed: DATABASE_URL is required")
	}
	key, err := encryptionKey()
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("seed: open db: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("seed: ping db: %w", err)
	}

	ctx := context.Background()

	restaurantID := uuid.New()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO restaurants (id, slug, name) VALUES ($1, $2, $3)`,
		restaurantID, "demo-bistro", "Demo Bistro",
	); err != nil {
		return fmt.Errorf("seed: insert restaurant: %w", err)
	}

	tableToken, err := randomToken()
	if err != nil {
		return fmt.Errorf("seed: table token: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tables (id, restaurant_id, label, token) VALUES ($1, $2, $3, $4)`,
		uuid.New(), restaurantID, "Table 1", tableToken,
	); err != nil {
		return fmt.Errorf("seed: insert table: %w", err)
	}

	if err := seedMenu(ctx, db, restaurantID); err != nil {
		return err
	}
	if err := seedLanding(ctx, db, restaurantID); err != nil {
		return err
	}
	if err := seedNotificationChannel(ctx, dsn, restaurantID, key); err != nil {
		return err
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	fmt.Printf("Table link: %s/demo-bistro/t/%s\n", baseURL, tableToken)
	return nil
}

// encryptionKey reads TOKEN_ENCRYPTION_KEY as base64-encoded bytes (the
// convention the future server command should also follow — crypto.Encrypt
// itself is env-agnostic per its package doc).
func encryptionKey() ([]byte, error) {
	raw := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if raw == "" {
		return nil, fmt.Errorf("seed: TOKEN_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("seed: TOKEN_ENCRYPTION_KEY: decode base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("seed: TOKEN_ENCRYPTION_KEY: want 32 bytes (AES-256) after base64 decode, got %d", len(key))
	}
	return key, nil
}

// randomToken returns a ~128-bit random, URL-safe Table token, per
// CONTEXT.md "Table link".
func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("seed: read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// seedMenu inserts 2 Categories with 2 Menu items each. The Margherita
// Pizza carries both an OptionGroup ("Size") and Allergen tags, satisfying
// the MVP smoke-test data the seed script exists for.
func seedMenu(ctx context.Context, db *sql.DB, restaurantID uuid.UUID) error {
	mainsID, drinksID := uuid.New(), uuid.New()
	categories := []struct {
		id       uuid.UUID
		name     string
		position int
	}{
		{mainsID, "Mains", 0},
		{drinksID, "Drinks", 1},
	}
	for _, c := range categories {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO categories (id, restaurant_id, name, position) VALUES ($1, $2, $3, $4)`,
			c.id, restaurantID, c.name, c.position,
		); err != nil {
			return fmt.Errorf("seed: insert category %s: %w", c.name, err)
		}
	}

	pizzaID := uuid.New()
	items := []struct {
		id         uuid.UUID
		categoryID uuid.UUID
		name       string
		desc       string
		priceCents int
		allergens  []domain.Allergen
	}{
		{pizzaID, mainsID, "Margherita Pizza", "Tomato, mozzarella, basil.", 900,
			[]domain.Allergen{domain.AllergenCereals, domain.AllergenMilk}},
		{uuid.New(), mainsID, "Grilled Chicken Salad", "Grilled chicken, mixed greens, vinaigrette.", 1200, nil},
		{uuid.New(), drinksID, "Fresh Lemonade", "House-made, served chilled.", 350, nil},
		{uuid.New(), drinksID, "Iced Tea", "Black tea, lightly sweetened.", 300, nil},
	}
	for _, it := range items {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO menu_items (id, restaurant_id, category_id, name, description, price_cents, available)
			 VALUES ($1, $2, $3, $4, $5, $6, true)`,
			it.id, restaurantID, it.categoryID, it.name, it.desc, it.priceCents,
		); err != nil {
			return fmt.Errorf("seed: insert menu item %s: %w", it.name, err)
		}
		for _, a := range it.allergens {
			if _, err := db.ExecContext(ctx,
				`INSERT INTO menu_item_allergens (menu_item_id, allergen) VALUES ($1, $2)`,
				it.id, a,
			); err != nil {
				return fmt.Errorf("seed: insert allergen %s for %s: %w", a, it.name, err)
			}
		}
	}

	sizeGroupID := uuid.New()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO option_groups (id, menu_item_id, name, multi, position) VALUES ($1, $2, $3, false, 0)`,
		sizeGroupID, pizzaID, "Size",
	); err != nil {
		return fmt.Errorf("seed: insert option group: %w", err)
	}
	sizes := []struct {
		label string
		delta int
	}{
		{"Small", 0},
		{"Medium", 200},
		{"Large", 400},
	}
	for i, o := range sizes {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO options (id, option_group_id, label, price_delta_cents, position) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), sizeGroupID, o.label, o.delta, i,
		); err != nil {
			return fmt.Errorf("seed: insert option %s: %w", o.label, err)
		}
	}
	return nil
}

// seedLanding inserts one Landing block of each of the three most common
// types from the closed catalog (domain.LandingBlockType), per issue #19.
func seedLanding(ctx context.Context, db *sql.DB, restaurantID uuid.UUID) error {
	blocks := []struct {
		typ      domain.LandingBlockType
		position int
		data     map[string]string
	}{
		{domain.LandingBlockBanner, 0, map[string]string{
			"image_url": "https://picsum.photos/seed/demo-bistro/1200/400",
			"title":     "Welcome to Demo Bistro",
		}},
		{domain.LandingBlockFreeText, 1, map[string]string{
			"body": "Fresh, seasonal dishes made daily. Scan, browse, and order right from your table.",
		}},
		{domain.LandingBlockOpeningHours, 2, map[string]string{
			"text": "Mon-Fri 11:00-22:00, Sat-Sun 10:00-23:00",
		}},
	}
	for _, b := range blocks {
		data, err := json.Marshal(b.data)
		if err != nil {
			return fmt.Errorf("seed: encode landing block %s data: %w", b.typ, err)
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO landing_blocks (id, restaurant_id, type, position, data) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), restaurantID, b.typ, b.position, data,
		); err != nil {
			return fmt.Errorf("seed: insert landing block %s: %w", b.typ, err)
		}
	}
	return nil
}

// seedNotificationChannel encrypts TELEGRAM_BOT_TOKEN and saves it via
// PostgresStore (the one write path Store already exposes for this
// entity), skipping with a warning if Telegram env vars aren't set — the
// demo restaurant should still boot without them.
func seedNotificationChannel(ctx context.Context, dsn string, restaurantID uuid.UUID, key []byte) error {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if botToken == "" || chatID == "" {
		fmt.Println("seed: TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID not set, skipping NotificationChannel")
		return nil
	}

	encrypted, err := crypto.Encrypt([]byte(botToken), restaurantID, key)
	if err != nil {
		return fmt.Errorf("seed: encrypt telegram bot token: %w", err)
	}

	st, err := postgres.NewPostgresStore(dsn)
	if err != nil {
		return fmt.Errorf("seed: open store: %w", err)
	}
	if err := st.SaveNotificationChannel(ctx, domain.NotificationChannel{
		RestaurantID:      restaurantID,
		TelegramChatID:    chatID,
		EncryptedBotToken: encrypted,
		KeyVersion:        1,
	}); err != nil {
		return fmt.Errorf("seed: save notification channel: %w", err)
	}
	return nil
}
