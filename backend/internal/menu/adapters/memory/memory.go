// Package memory implements ports.Store in-memory, used as a test double
// for app-layer and adapters/http tests where a live Postgres isn't
// available.
package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"

	"uuid"
)

// MemoryStore is an in-memory ports.Store implementation used as a test
// double for app-layer and adapters/http tests where a live Postgres
// isn't available.
// ponytail: linear scans over small maps under one mutex — fine for test
// fixtures, add real indexes only if this stops being test-only.
var _ ports.Store = (*MemoryStore)(nil)

type MemoryStore struct {
	mu sync.Mutex

	restaurants          map[uuid.UUID]domain.Restaurant
	tables               map[uuid.UUID]domain.Table
	categories           map[uuid.UUID]domain.Category
	menuItems            map[uuid.UUID]domain.MenuItem
	orders               map[uuid.UUID]domain.Order
	serviceRequests      map[uuid.UUID]domain.ServiceRequest
	landingBlocks        map[uuid.UUID]domain.LandingBlock
	notificationChannels map[uuid.UUID]domain.NotificationChannel // keyed by RestaurantID
}

// NewMemoryStore returns an empty MemoryStore ready to be pre-seeded via
// its Seed* helpers and/or the normal Store write methods.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		restaurants:          make(map[uuid.UUID]domain.Restaurant),
		tables:               make(map[uuid.UUID]domain.Table),
		categories:           make(map[uuid.UUID]domain.Category),
		menuItems:            make(map[uuid.UUID]domain.MenuItem),
		orders:               make(map[uuid.UUID]domain.Order),
		serviceRequests:      make(map[uuid.UUID]domain.ServiceRequest),
		landingBlocks:        make(map[uuid.UUID]domain.LandingBlock),
		notificationChannels: make(map[uuid.UUID]domain.NotificationChannel),
	}
}

// --- Seed helpers -----------------------------------------------------
//
// The Store interface has no Create methods for Restaurant/Table/
// Category/MenuItem/LandingBlock (there's no admin API in the MVP — a
// seed script populates Postgres directly). These helpers are the
// MemoryStore equivalent for tests. Each fills in a random ID if the
// caller left it zero, and returns the stored value.

func (m *MemoryStore) SeedRestaurant(r domain.Restaurant) domain.Restaurant {
	if r.ID == uuid.Nil() {
		r.ID = uuid.New()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restaurants[r.ID] = r
	return r
}

func (m *MemoryStore) SeedTable(t domain.Table) domain.Table {
	if t.ID == uuid.Nil() {
		t.ID = uuid.New()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tables[t.ID] = t
	return t
}

func (m *MemoryStore) SeedCategory(c domain.Category) domain.Category {
	if c.ID == uuid.Nil() {
		c.ID = uuid.New()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.categories[c.ID] = c
	return c
}

func (m *MemoryStore) SeedMenuItem(item domain.MenuItem) domain.MenuItem {
	if item.ID == uuid.Nil() {
		item.ID = uuid.New()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.menuItems[item.ID] = item
	return item
}

func (m *MemoryStore) SeedLandingBlock(b domain.LandingBlock) domain.LandingBlock {
	if b.ID == uuid.Nil() {
		b.ID = uuid.New()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.landingBlocks[b.ID] = b
	return b
}

// --- Store implementation ----------------------------------------------

func (m *MemoryStore) RestaurantBySlug(ctx context.Context, slug string) (domain.Restaurant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.restaurants {
		if r.Slug == slug {
			return r, nil
		}
	}
	return domain.Restaurant{}, ports.ErrNotFound
}

func (m *MemoryStore) TableByToken(ctx context.Context, restaurantID uuid.UUID, token string) (domain.Table, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tables {
		if t.RestaurantID == restaurantID && t.Token == token {
			return t, nil
		}
	}
	return domain.Table{}, ports.ErrNotFound
}

func (m *MemoryStore) LandingBlocks(ctx context.Context, restaurantID uuid.UUID) ([]domain.LandingBlock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	blocks := make([]domain.LandingBlock, 0)
	for _, b := range m.landingBlocks {
		if b.RestaurantID == restaurantID {
			blocks = append(blocks, b)
		}
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Position < blocks[j].Position })
	return blocks, nil
}

func (m *MemoryStore) Menu(ctx context.Context, restaurantID uuid.UUID) ([]domain.Category, []domain.MenuItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cats := make([]domain.Category, 0)
	for _, c := range m.categories {
		if c.RestaurantID == restaurantID {
			cats = append(cats, c)
		}
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].Position < cats[j].Position })

	items := make([]domain.MenuItem, 0)
	for _, it := range m.menuItems {
		if it.RestaurantID == restaurantID {
			items = append(items, it)
		}
	}
	return cats, items, nil
}

// Order returns the persisted Order by ID, for tests that need to verify
// what was actually stored (e.g. that an OrderLine snapshot didn't drift
// after the source MenuItem changed). Not part of the Store interface —
// the MVP has no order-read endpoint (see CONTEXT.md, no admin API yet).
func (m *MemoryStore) Order(id uuid.UUID) (domain.Order, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	return o, ok
}

func (m *MemoryStore) CreateOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if len(order.Lines) == 0 {
		return domain.Order{}, errors.New("store: order must have at least one line")
	}
	order.ID = uuid.New()
	order.CreatedAt = time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[order.ID] = order
	return order, nil
}

func (m *MemoryStore) CreateServiceRequest(ctx context.Context, req domain.ServiceRequest) (domain.ServiceRequest, error) {
	req.ID = uuid.New()
	req.Status = domain.ServiceRequestPending
	req.CreatedAt = time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceRequests[req.ID] = req
	return req, nil
}

func (m *MemoryStore) HasOpenServiceRequest(ctx context.Context, tableID uuid.UUID, kind domain.ServiceRequestKind) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.serviceRequests {
		if r.TableID == tableID && r.Kind == kind && r.Status == domain.ServiceRequestPending {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryStore) AcknowledgeServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.serviceRequests[id]
	if !ok || r.RestaurantID != restaurantID {
		return ports.ErrNotFound
	}
	r.Status = domain.ServiceRequestAcknowledged
	m.serviceRequests[id] = r
	return nil
}

func (m *MemoryStore) NotificationChannel(ctx context.Context, restaurantID uuid.UUID) (domain.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.notificationChannels[restaurantID]
	if !ok {
		return domain.NotificationChannel{}, ports.ErrNotFound
	}
	return ch, nil
}

func (m *MemoryStore) SaveNotificationChannel(ctx context.Context, ch domain.NotificationChannel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notificationChannels[ch.RestaurantID] = ch
	return nil
}
