package memory

import (
	"context"
	"testing"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"

	"github.com/google/uuid"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()

	restaurant := m.SeedRestaurant(domain.Restaurant{Slug: "demo-diner", Name: "Demo Diner"})
	table := m.SeedTable(domain.Table{RestaurantID: restaurant.ID, Label: "Table 5", Token: "tok-123"})
	category := m.SeedCategory(domain.Category{RestaurantID: restaurant.ID, Name: "Mains", Position: 0})
	item := m.SeedMenuItem(domain.MenuItem{
		RestaurantID: restaurant.ID,
		CategoryID:   category.ID,
		Name:         "Burger",
		PriceCents:   1200,
		Available:    true,
		Allergens:    []domain.Allergen{domain.AllergenCereals},
	})
	block := m.SeedLandingBlock(domain.LandingBlock{RestaurantID: restaurant.ID, Type: domain.LandingBlockBanner, Position: 0})

	// RestaurantBySlug
	got, err := m.RestaurantBySlug(ctx, "demo-diner")
	if err != nil || got.ID != restaurant.ID {
		t.Fatalf("RestaurantBySlug: got %+v, err %v", got, err)
	}
	if _, err := m.RestaurantBySlug(ctx, "nope"); err != ports.ErrNotFound {
		t.Fatalf("RestaurantBySlug unknown slug: want ports.ErrNotFound, got %v", err)
	}

	// TableByToken
	gotTable, err := m.TableByToken(ctx, restaurant.ID, "tok-123")
	if err != nil || gotTable.ID != table.ID {
		t.Fatalf("TableByToken: got %+v, err %v", gotTable, err)
	}
	if _, err := m.TableByToken(ctx, restaurant.ID, "wrong"); err != ports.ErrNotFound {
		t.Fatalf("TableByToken wrong token: want ports.ErrNotFound, got %v", err)
	}

	// LandingBlocks
	blocks, err := m.LandingBlocks(ctx, restaurant.ID)
	if err != nil || len(blocks) != 1 || blocks[0].ID != block.ID {
		t.Fatalf("LandingBlocks: got %+v, err %v", blocks, err)
	}
	if empty, err := m.LandingBlocks(ctx, uuid.New()); err != nil || len(empty) != 0 {
		t.Fatalf("LandingBlocks unknown restaurant: want empty slice, got %+v, err %v", empty, err)
	}

	// Menu
	cats, items, err := m.Menu(ctx, restaurant.ID)
	if err != nil || len(cats) != 1 || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("Menu: got cats=%+v items=%+v, err %v", cats, items, err)
	}

	// CreateOrder
	order, err := m.CreateOrder(ctx, domain.Order{
		RestaurantID: restaurant.ID,
		TableID:      table.ID,
		Lines: []domain.OrderLine{
			{MenuItemID: item.ID, Name: item.Name, UnitPriceCents: item.PriceCents, Qty: 2},
		},
	})
	if err != nil || order.ID == uuid.Nil || order.CreatedAt.IsZero() {
		t.Fatalf("CreateOrder: got %+v, err %v", order, err)
	}
	if _, err := m.CreateOrder(ctx, domain.Order{RestaurantID: restaurant.ID, TableID: table.ID}); err == nil {
		t.Fatal("CreateOrder with no lines: want error, got nil")
	}

	// CreateServiceRequest + HasOpenServiceRequest + AcknowledgeServiceRequest
	if open, err := m.HasOpenServiceRequest(ctx, table.ID, domain.CallWaiter); err != nil || open {
		t.Fatalf("HasOpenServiceRequest before create: got %v, err %v", open, err)
	}
	req, err := m.CreateServiceRequest(ctx, domain.ServiceRequest{
		RestaurantID: restaurant.ID,
		TableID:      table.ID,
		Kind:         domain.CallWaiter,
	})
	if err != nil || req.Status != domain.ServiceRequestPending {
		t.Fatalf("CreateServiceRequest: got %+v, err %v", req, err)
	}
	if open, err := m.HasOpenServiceRequest(ctx, table.ID, domain.CallWaiter); err != nil || !open {
		t.Fatalf("HasOpenServiceRequest after create: got %v, err %v", open, err)
	}
	if err := m.AcknowledgeServiceRequest(ctx, restaurant.ID, req.ID); err != nil {
		t.Fatalf("AcknowledgeServiceRequest: %v", err)
	}
	if open, err := m.HasOpenServiceRequest(ctx, table.ID, domain.CallWaiter); err != nil || open {
		t.Fatalf("HasOpenServiceRequest after acknowledge: got %v, err %v", open, err)
	}
	if err := m.AcknowledgeServiceRequest(ctx, restaurant.ID, req.ID); err != nil {
		t.Fatalf("AcknowledgeServiceRequest idempotent: %v", err)
	}
	if err := m.AcknowledgeServiceRequest(ctx, uuid.New(), req.ID); err != ports.ErrNotFound {
		t.Fatalf("AcknowledgeServiceRequest wrong restaurant: want ports.ErrNotFound, got %v", err)
	}

	// NotificationChannel + SaveNotificationChannel
	if _, err := m.NotificationChannel(ctx, restaurant.ID); err != ports.ErrNotFound {
		t.Fatalf("NotificationChannel before save: want ports.ErrNotFound, got %v", err)
	}
	ch := domain.NotificationChannel{
		RestaurantID:      restaurant.ID,
		TelegramChatID:    "12345",
		EncryptedBotToken: []byte("ciphertext"),
		KeyVersion:        1,
	}
	if err := m.SaveNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("SaveNotificationChannel: %v", err)
	}
	gotCh, err := m.NotificationChannel(ctx, restaurant.ID)
	if err != nil || gotCh.TelegramChatID != "12345" || gotCh.KeyVersion != 1 {
		t.Fatalf("NotificationChannel after save: got %+v, err %v", gotCh, err)
	}
}
