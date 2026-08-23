// Package ports defines the boundaries the Menu app depends on but
// doesn't implement: persistence (Store) and outbound alerting
// (Notifier). Implementations live under adapters/; this package only
// specifies behavior.
package ports

import (
	"context"
	"errors"

	"aivo/internal/domain/menu"

	"github.com/google/uuid"
)

// ErrNotFound is returned by lookup methods when the requested row does
// not exist (or, for tenant-scoped lookups, exists under a different
// Restaurant). Callers should treat it as "404", not a system error.
var ErrNotFound = errors.New("store: not found")

// Store is everything the Menu app needs from persistence. All methods
// are context.Context-first and tenant-scoped by RestaurantID where the
// entity itself doesn't already imply the Restaurant (e.g. via Slug or
// Token lookup) — implementations MUST enforce that scoping in the query
// itself (e.g. "WHERE restaurant_id = $1 AND ..."), never in application
// code after an unscoped fetch.
type Store interface {
	// RestaurantBySlug returns the Restaurant identified by its slug.
	// Returns ErrNotFound if no Restaurant has that slug.
	RestaurantBySlug(ctx context.Context, slug string) (domain.Restaurant, error)

	// TableByToken returns the Table whose Token matches, scoped to the
	// given Restaurant (a token is only meaningful within its owning
	// Restaurant's slug in the table link). Returns ErrNotFound if no
	// Table under that Restaurant has that Token.
	TableByToken(ctx context.Context, restaurantID uuid.UUID, token string) (domain.Table, error)

	// LandingBlocks returns every LandingBlock for the Restaurant, ordered
	// by Position ascending. Returns an empty slice (not an error) if none
	// are configured.
	LandingBlocks(ctx context.Context, restaurantID uuid.UUID) ([]domain.LandingBlock, error)

	// Menu returns every Category for the Restaurant, ordered by Position
	// ascending, and every MenuItem for the Restaurant (each with its
	// OptionGroups, Options, and Allergens populated), grouped by
	// CategoryID by the caller via MenuItem.CategoryID. Returns empty
	// slices (not an error) if the Restaurant has no Categories/Items yet.
	Menu(ctx context.Context, restaurantID uuid.UUID) ([]domain.Category, []domain.MenuItem, error)

	// CreateOrder persists a new Order. The caller populates every field
	// except ID and CreatedAt, which the store generates. Order.Lines must
	// be non-empty (an Order with no lines is a caller bug, not a store
	// concern to silently accept) — implementations MUST return a non-nil
	// error rather than persist a zero-line Order. Returns the persisted
	// Order (with ID/CreatedAt set) on success.
	CreateOrder(ctx context.Context, order domain.Order) (domain.Order, error)

	// CreateServiceRequest persists a new ServiceRequest with Status set
	// to domain.ServiceRequestPending. The caller populates
	// RestaurantID/TableID/Kind; ID, Status, and CreatedAt are set by the
	// store. Callers MUST check HasOpenServiceRequest first — this method
	// does not itself dedupe.
	CreateServiceRequest(ctx context.Context, req domain.ServiceRequest) (domain.ServiceRequest, error)

	// HasOpenServiceRequest reports whether the given Table already has a
	// ServiceRequest of the given Kind in "pending" status. Used to
	// enforce "at most one open request of a given kind per Table" before
	// calling CreateServiceRequest. Restaurant scoping is implied by
	// TableID (a Table belongs to exactly one Restaurant).
	HasOpenServiceRequest(ctx context.Context, tableID uuid.UUID, kind domain.ServiceRequestKind) (bool, error)

	// AcknowledgeServiceRequest transitions the ServiceRequest with the
	// given ID to domain.ServiceRequestAcknowledged. Scoped to
	// restaurantID for tenant isolation: acknowledging an ID that exists
	// but belongs to a different Restaurant returns ErrNotFound, same as
	// a nonexistent ID. Acknowledging an already-acknowledged request is
	// idempotent (no error).
	AcknowledgeServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error

	// NotificationChannel returns the Restaurant's configured
	// NotificationChannel. Returns ErrNotFound if the Restaurant has not
	// configured one yet (a Restaurant has at most one channel for v1).
	NotificationChannel(ctx context.Context, restaurantID uuid.UUID) (domain.NotificationChannel, error)

	// SaveNotificationChannel creates or replaces (upsert, keyed on
	// RestaurantID) the Restaurant's NotificationChannel.
	// EncryptedBotToken must already be ciphertext (see docs/adr/0003) —
	// this method does not encrypt, it only persists.
	SaveNotificationChannel(ctx context.Context, ch domain.NotificationChannel) error
}
