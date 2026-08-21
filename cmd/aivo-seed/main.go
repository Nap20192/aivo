// Command aivo-seed inserts the demo tenant "Ember & Bone" (see
// docs/prototypes/aivo-menu-prototype.dc.html for the fixture source):
// org + owner + free subscription + restaurant ember-and-bone, a waiter
// account, the prototype menu (Starters / From the grill / Sides / Wine
// with Size/Doneness option groups and sauce add-ons), tables
// 03/04/07/09/12/15, and the prototype theme. Reads DATABASE_URL. Not
// idempotent: re-running fails on the owner email unique constraint.
//
// Demo credentials (dev only, never real secrets):
//
//	owner@ember.test  / embertest1
//	waiter@ember.test / embertest1
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

	menupg "aivo/internal/menu/adapters/postgres"
	menudomain "aivo/internal/menu/domain"
	"aivo/internal/platform/adapters/billing"
	platformpg "aivo/internal/platform/adapters/postgres"
	platformapp "aivo/internal/platform/app"
	platformdomain "aivo/internal/platform/domain"
	pospg "aivo/internal/pos/adapters/postgres"
	"aivo/pkg/migrate"

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
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("seed: open db: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("seed: ping db: %w", err)
	}

	ctx := context.Background()

	// Same migration set as the server, so the seed works on a fresh DB.
	err = migrate.Apply(ctx, db, []migrate.Source{
		{Name: "menu", FS: menupg.MigrationsFS, Dir: "migrations"},
		{Name: "platform", FS: platformpg.MigrationsFS, Dir: "migrations"},
		{Name: "pos", FS: pospg.MigrationsFS, Dir: "migrations"},
	})
	if err != nil {
		return err
	}

	menuStore := menupg.NewPostgresStoreFromDB(db)
	platform := platformapp.New(platformpg.NewStore(db), billing.NewFake(), nil)

	owner, _, err := platform.Register(ctx, "Ember & Bone", "Ember & Bone", "owner@ember.test", "embertest1")
	if err != nil {
		return fmt.Errorf("seed: register: %w", err)
	}
	rests, err := platform.Restaurants(ctx, owner.OrgID)
	if err != nil || len(rests) != 1 {
		return fmt.Errorf("seed: restaurants: %v", err)
	}
	rest := rests[0]

	if _, err := platform.AddStaff(ctx, owner.OrgID, rest.ID, "waiter@ember.test", "embertest1", platformdomain.RoleWaiter); err != nil {
		return fmt.Errorf("seed: waiter: %w", err)
	}

	// Settings matching the admin prototype fixtures.
	address := "14 Rue des Bouchers"
	phone := "02 512 33 74"
	instagram := "@emberandbone"
	hours := []platformdomain.HoursRow{
		{Label: "Kitchen", Open: "17:00", Close: "22:30"},
		{Label: "Bar", Open: "17:00", Close: "00:00"},
	}
	if _, err := platform.UpdateRestaurant(ctx, owner.OrgID, rest.ID, platformapp.RestaurantPatch{
		Address: &address, Phone: &phone, Instagram: &instagram, Hours: &hours,
	}); err != nil {
		return fmt.Errorf("seed: restaurant settings: %w", err)
	}

	// Theme: the prototype's restaurant config (accent drives themeVars
	// in the menu app).
	theme, _ := json.Marshal(map[string]any{
		"brand_name": "Ember & Bone",
		"accent":     "Blood red",
		"bold":       false,
	})
	if _, err := platform.SaveTheme(ctx, platformdomain.Theme{RestaurantID: rest.ID, ThemeJSON: theme}); err != nil {
		return fmt.Errorf("seed: theme: %w", err)
	}

	if err := seedMenu(ctx, menuStore, rest.ID); err != nil {
		return err
	}
	if err := seedCustomer(ctx, db, menuStore, platform, rest.ID); err != nil {
		return err
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	tables, err := menuStore.Tables(ctx, rest.ID)
	if err != nil {
		return err
	}
	fmt.Println("Seeded Ember & Bone (owner@ember.test / embertest1, waiter@ember.test / embertest1)")
	for _, t := range tables {
		fmt.Printf("Table %s: %s/%s/t/%s\n", t.Label, baseURL, rest.Slug, t.Token)
	}
	return nil
}

type opt struct {
	label string
	delta int
}

type group struct {
	name  string
	multi bool
	opts  []opt
}

type item struct {
	name      string
	desc      string
	cents     int
	allergens []menudomain.Allergen
	available bool
	groups    []group
}

func seedMenu(ctx context.Context, store *menupg.PostgresStore, restaurantID uuid.UUID) error {
	doneness := group{name: "Doneness", opts: []opt{
		{"Rare", 0}, {"Medium rare", 0}, {"Medium", 0}, {"Well done", 0},
	}}
	sauces := group{name: "Sauces", multi: true, opts: []opt{
		{"Béarnaise", 300}, {"Bone marrow butter", 400}, {"Peppercorn sauce", 300},
	}}

	// Default menu is auto-provisioned on registration; Wine moves to a
	// second "Bar" menu (exercises the multi-menu contract).
	menus, err := store.Menus(ctx, restaurantID)
	if err != nil || len(menus) == 0 {
		return fmt.Errorf("seed: menus: %v", err)
	}
	defaultMenuID := menus[0].ID // default first per store ordering
	barMenu := menudomain.Menu{ID: uuid.New(), RestaurantID: restaurantID, Slug: "bar", Name: "Bar", Position: 1}
	if err := store.CreateMenu(ctx, barMenu); err != nil {
		return fmt.Errorf("seed: bar menu: %w", err)
	}
	menuFor := func(category string) uuid.UUID {
		if category == "Wine" {
			return barMenu.ID
		}
		return defaultMenuID
	}

	// Fixtures from docs/prototypes/aivo-menu-prototype.dc.html (prices
	// there are dollars; integer cents here).
	menu := []struct {
		category string
		items    []item
	}{
		{"Starters", []item{
			{"Bone marrow, sourdough", "Roasted, parsley and caper salt, grilled sourdough.", 1400, []menudomain.Allergen{menudomain.AllergenCereals}, true, nil},
			{"Beef tartare, cured yolk", "Hand-cut sirloin, mustard, shallot, rye crisps.", 1800, []menudomain.Allergen{menudomain.AllergenEggs, menudomain.AllergenMustard}, true, nil},
			{"Charred leeks, olive oil", "Vadouvan, hazelnut, sheep's curd.", 1200, nil, false, nil},
			{"Grilled flatbread, beef fat", "Wood oven, smoked salt, rosemary.", 800, nil, true, nil},
		}},
		{"From the grill", []item{
			{"Dry-aged ribeye", "45 days on the bone, over vine wood. Rested 10 minutes, carved to order.", 4600,
				[]menudomain.Allergen{menudomain.AllergenMilk, menudomain.AllergenMustard}, true,
				[]group{{name: "Size", opts: []opt{{"300 g", 0}, {"400 g · centre cut", 1200}, {"600 g · to share", 2600}}}, doneness, sauces}},
			{"Bavette, chimichurri", "Onglet's louder cousin, cooked over embers.", 3400, nil, true, []group{doneness, sauces}},
			{"Lamb shoulder, 6 hours", "Slow-roasted over embers, harissa, yoghurt, flatbread. Enough for two.", 4600, nil, false, nil},
			{"Half chicken, brined", "Lemon, thyme, pan drippings.", 2800, nil, true, nil},
		}},
		{"Sides", []item{
			{"Triple-cooked chips", "Beef fat, rosemary salt.", 900, nil, true, nil},
			{"Hispi cabbage", "Charred, anchovy cream.", 800, []menudomain.Allergen{menudomain.AllergenFish, menudomain.AllergenMilk}, true, nil},
			{"Green salad", "Soft herbs, mustard dressing.", 700, []menudomain.Allergen{menudomain.AllergenMustard}, true, nil},
		}},
		{"Wine", []item{
			{"Malbec, glass", "Mendoza. Dark fruit, holds up to the grill.", 1400, nil, true, nil},
			{"Gamay, Beaujolais", "Chilled, light, made for chips.", 1200, nil, true, nil},
			{"Ribolla, 2021", "Orange. Skin contact, apricot, grip.", 1300, nil, true, nil},
		}},
	}

	for pos, c := range menu {
		cat := menudomain.Category{ID: uuid.New(), RestaurantID: restaurantID, MenuID: menuFor(c.category), Name: c.category, Position: pos}
		if err := store.CreateCategory(ctx, cat); err != nil {
			return fmt.Errorf("seed: category %s: %w", c.category, err)
		}
		for _, it := range c.items {
			groups := make([]menudomain.OptionGroup, len(it.groups))
			for gi, g := range it.groups {
				opts := make([]menudomain.Option, len(g.opts))
				for oi, o := range g.opts {
					opts[oi] = menudomain.Option{ID: uuid.New(), Label: o.label, PriceDeltaCents: o.delta}
				}
				groups[gi] = menudomain.OptionGroup{ID: uuid.New(), Name: g.name, Multi: g.multi, Options: opts}
			}
			mi := menudomain.MenuItem{
				ID: uuid.New(), RestaurantID: restaurantID, CategoryID: cat.ID,
				Name: it.name, Description: it.desc, PriceCents: it.cents,
				Allergens: it.allergens, Available: it.available, OptionGroups: groups,
			}
			if err := store.CreateMenuItem(ctx, mi); err != nil {
				return fmt.Errorf("seed: item %s: %w", it.name, err)
			}
		}
	}

	for _, label := range []string{"03", "04", "07", "09", "12", "15"} {
		token, err := randomToken()
		if err != nil {
			return err
		}
		t := menudomain.Table{ID: uuid.New(), RestaurantID: restaurantID, Label: label, Token: token}
		if err := store.CreateTable(ctx, t); err != nil {
			return fmt.Errorf("seed: table %s: %w", label, err)
		}
	}
	return nil
}

// seedCustomer creates the demo diner account (guest@ember.test /
// embertest1) with two historical linked orders at Ember & Bone, so the
// admin Guests screen and customer/me history have data.
func seedCustomer(ctx context.Context, db *sql.DB, store *menupg.PostgresStore, platform *platformapp.App, restaurantID uuid.UUID) error {
	customer, _, err := platform.RegisterCustomer(ctx, "guest@ember.test", "embertest1", "Nora Guest")
	if err != nil {
		return fmt.Errorf("seed: customer: %w", err)
	}

	_, items, err := store.Menu(ctx, restaurantID)
	if err != nil {
		return fmt.Errorf("seed: customer orders: menu: %w", err)
	}
	byName := map[string]menudomain.MenuItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	tables, err := store.Tables(ctx, restaurantID)
	if err != nil || len(tables) == 0 {
		return fmt.Errorf("seed: customer orders: tables: %v", err)
	}

	orderFixtures := []struct {
		daysAgo int
		names   []string
	}{
		{21, []string{"Bavette, chimichurri", "Triple-cooked chips", "Malbec, glass"}},
		{6, []string{"Beef tartare, cured yolk", "Half chicken, brined", "Gamay, Beaujolais"}},
	}
	for _, of := range orderFixtures {
		lines := []menudomain.OrderLine{}
		for _, name := range of.names {
			it, ok := byName[name]
			if !ok {
				return fmt.Errorf("seed: customer orders: item %q missing", name)
			}
			line, err := menudomain.NewOrderLine(it, nil, 1)
			if err != nil {
				return fmt.Errorf("seed: customer orders: %w", err)
			}
			lines = append(lines, line)
		}
		order, err := store.CreateOrder(ctx, menudomain.Order{
			RestaurantID: restaurantID,
			TableID:      tables[0].ID,
			CustomerID:   &customer.ID,
			Lines:        lines,
		})
		if err != nil {
			return fmt.Errorf("seed: customer orders: create: %w", err)
		}
		// Backdate for a believable history.
		if _, err := db.ExecContext(ctx,
			`UPDATE orders SET created_at = now() - $1::int * interval '1 day' WHERE id = $2`,
			of.daysAgo, order.ID); err != nil {
			return fmt.Errorf("seed: customer orders: backdate: %w", err)
		}
	}

	if err := platform.TouchGuest(ctx, restaurantID, customer.ID); err != nil {
		return fmt.Errorf("seed: guest profile: %w", err)
	}
	fmt.Println("Demo customer: guest@ember.test / embertest1 (2 historical orders)")
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("seed: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
