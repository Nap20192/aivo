// AdminStore is the staff-facing write/read surface over the Menu
// context's data: category/item/table CRUD for the admin panel, plus the
// lookups the POS bridge and the /api/v1/t/{token} diner entry need.
// Separate from Store so the diner-facing adapters (and the in-memory
// test store) don't have to implement it. Implemented by the Postgres
// adapter only.
package ports

import (
	"context"
	"errors"

	"aivo/internal/domain/menu"

	"uuid"
)

// ErrItemReferenced is returned by DeleteMenuItem when historical order
// or ticket lines still reference the item.
var ErrItemReferenced = errors.New("store: menu item referenced by past orders; mark unavailable instead")

// ErrConflict is returned on caller-fixable unique violations (taken
// menu slug).
var ErrConflict = errors.New("store: conflict")

// AdminStore methods are all tenant-scoped by restaurantID in the query
// itself, same rule as Store.
type AdminStore interface {
	RestaurantByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)
	// TableByTokenGlobal resolves a table token without a slug (tokens
	// are globally unique, see menu migration 0002).
	TableByTokenGlobal(ctx context.Context, token string) (domain.Table, error)

	Tables(ctx context.Context, restaurantID uuid.UUID) ([]domain.Table, error)
	TableByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.Table, error)
	CreateTable(ctx context.Context, t domain.Table) error
	// RegenerateTableToken swaps the table's token for newToken,
	// invalidating the old link immediately.
	RegenerateTableToken(ctx context.Context, restaurantID, id uuid.UUID, newToken string) error

	// Menus returns the restaurant's menus, default first then by
	// position. Every restaurant has at least one (the default).
	Menus(ctx context.Context, restaurantID uuid.UUID) ([]domain.Menu, error)
	MenuBySlug(ctx context.Context, restaurantID uuid.UUID, slug string) (domain.Menu, error)
	// CreateMenu returns ErrConflict-wrapping error on a taken slug.
	CreateMenu(ctx context.Context, m domain.Menu) error
	// UpdateMenu persists name/slug/position/is_default; setting
	// IsDefault=true clears the previous default atomically. Clearing the
	// default's flag directly (IsDefault=false on the default menu) is
	// rejected — pick a new default instead.
	UpdateMenu(ctx context.Context, m domain.Menu) error
	// DeleteMenu enforces domain.CanDeleteMenu (never default/last;
	// non-empty only with force — categories and items cascade).
	DeleteMenu(ctx context.Context, restaurantID, id uuid.UUID, force bool) error

	// CreateCategory persists c under c.MenuID (must belong to the same
	// restaurant).
	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, restaurantID, id uuid.UUID) error

	// CreateMenuItem / UpdateMenuItem persist the item with its
	// allergens, option groups, and options; update replaces the
	// collections wholesale (order/ticket lines snapshot, so replacing
	// options never rewrites history).
	CreateMenuItem(ctx context.Context, it domain.MenuItem) error
	UpdateMenuItem(ctx context.Context, it domain.MenuItem) error
	// DeleteMenuItem returns ErrItemReferenced if order or ticket lines
	// reference the item (mark it unavailable instead).
	DeleteMenuItem(ctx context.Context, restaurantID, id uuid.UUID) error
	MenuItemByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.MenuItem, error)
	CountMenuItems(ctx context.Context, restaurantID uuid.UUID) (int, error)

	// CreateHandoff stores a pickup code, deleting any previous active
	// handoff for the same table (no stacking). Returns ErrConflict on a
	// code collision — the caller regenerates and retries.
	CreateHandoff(ctx context.Context, h domain.Handoff) error
	// HandoffByCode returns the restaurant's ACTIVE (unused, unexpired)
	// handoff for code; ErrNotFound otherwise (unknown, expired, used, or
	// another restaurant's — all look identical).
	HandoffByCode(ctx context.Context, restaurantID uuid.UUID, code string) (domain.Handoff, error)
	// MarkHandoffUsed consumes the code (single-use); ErrNotFound if it
	// is no longer active. UnmarkHandoffUsed compensates when the accept
	// that consumed it fails afterwards.
	MarkHandoffUsed(ctx context.Context, restaurantID, id uuid.UUID) error
	UnmarkHandoffUsed(ctx context.Context, id uuid.UUID) error

	PendingServiceRequests(ctx context.Context, restaurantID uuid.UUID) ([]domain.ServiceRequest, error)
	// PendingServiceRequestsForTable narrows to one table (the diner
	// "one open request per table" state).
	PendingServiceRequestsForTable(ctx context.Context, restaurantID, tableID uuid.UUID) ([]domain.ServiceRequest, error)
	// SetServiceRequestStatus transitions a request (acknowledged or
	// dismissed), scoped to restaurantID; ErrNotFound on wrong tenant.
	SetServiceRequestStatus(ctx context.Context, restaurantID, id uuid.UUID, status string) error
}
