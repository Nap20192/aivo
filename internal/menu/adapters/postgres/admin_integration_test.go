package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"

	"github.com/google/uuid"
)

// Integration test for the item↔category tenant-scoping guard. Runs only
// with TEST_DATABASE_URL set (a migrated database, e.g. the compose dev
// one); skipped otherwise so the suite stays green without infra.
func TestItemCategoryTenantScoping(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	store, err := NewPostgresStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	mkRestaurant := func(name string) (uuid.UUID, uuid.UUID) {
		rid := uuid.New()
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO restaurants (id, slug, name) VALUES ($1, $2, $3)`,
			rid, "t-"+uuid.NewString()[:8], name); err != nil {
			t.Fatal(err)
		}
		mid := uuid.New()
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO menus (id, restaurant_id, slug, name, is_default) VALUES ($1, $2, 'menu', 'Menu', true)`,
			mid, rid); err != nil {
			t.Fatal(err)
		}
		catID := uuid.New()
		if err := store.CreateCategory(ctx, domain.Category{ID: catID, RestaurantID: rid, MenuID: mid, Name: "Cat"}); err != nil {
			t.Fatal(err)
		}
		return rid, catID
	}
	restA, catA := mkRestaurant("A")
	_, catB := mkRestaurant("B")
	t.Cleanup(func() {
		store.db.ExecContext(ctx, `DELETE FROM restaurants WHERE name IN ('A','B')`)
	})

	// Create under a foreign category must fail tenant scoping.
	err = store.CreateMenuItem(ctx, domain.MenuItem{
		ID: uuid.New(), RestaurantID: restA, CategoryID: catB, Name: "Sneaky", PriceCents: 100, Available: true,
	})
	if !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("create with foreign category: got %v, want ErrNotFound", err)
	}

	// Legit create, then update to a foreign category must fail too.
	it := domain.MenuItem{ID: uuid.New(), RestaurantID: restA, CategoryID: catA, Name: "Ok", PriceCents: 100, Available: true}
	if err := store.CreateMenuItem(ctx, it); err != nil {
		t.Fatalf("legit create: %v", err)
	}
	it.CategoryID = catB
	if err := store.UpdateMenuItem(ctx, it); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("update to foreign category: got %v, want ErrNotFound", err)
	}
	// And a legit update still works.
	it.CategoryID = catA
	it.Name = "Renamed"
	if err := store.UpdateMenuItem(ctx, it); err != nil {
		t.Errorf("legit update: %v", err)
	}
}
