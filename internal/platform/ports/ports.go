// Package ports defines the boundaries the Platform app depends on but
// doesn't implement: persistence (Store), billing (BillingProvider), and
// image storage (ImageStore).
package ports

import (
	"context"
	"errors"
	"io"

	"aivo/internal/platform/domain"

	"github.com/google/uuid"
)

// ErrNotFound is returned by lookup methods when the requested row does
// not exist (or, for tenant-scoped lookups, exists under a different
// org/restaurant). Callers should treat it as "404".
var ErrNotFound = errors.New("platform: not found")

// ErrConflict is returned on unique-constraint violations the caller can
// fix (duplicate email, taken slug, taken domain).
var ErrConflict = errors.New("platform: conflict")

// Store is everything the Platform app needs from persistence. Tenant
// scoping rule: every method that reads/writes org- or restaurant-owned
// rows takes the owning orgID and MUST enforce it in the query itself.
type Store interface {
	// CreateOrgWithOwner atomically creates the org, its owner user, the
	// initial free subscription, and the first restaurant. Returns
	// ErrConflict if the email or slug is taken.
	CreateOrgWithOwner(ctx context.Context, org domain.Organization, owner domain.User, sub domain.Subscription, rest domain.Restaurant) error

	Organization(ctx context.Context, orgID uuid.UUID) (domain.Organization, error)
	UpdateOrganization(ctx context.Context, org domain.Organization) error

	UserByEmail(ctx context.Context, email string) (domain.User, error)
	UserByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	// CreateUser returns ErrConflict if the email is taken.
	CreateUser(ctx context.Context, u domain.User) error
	// StaffForRestaurant returns owners of the restaurant's org plus staff
	// scoped to the restaurant.
	StaffForRestaurant(ctx context.Context, orgID, restaurantID uuid.UUID) ([]domain.User, error)

	CreateSession(ctx context.Context, s domain.Session) error
	// SessionUser resolves an unexpired session token hash to its user.
	SessionUser(ctx context.Context, tokenHash []byte) (domain.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error

	Subscription(ctx context.Context, orgID uuid.UUID) (domain.Subscription, error)
	SaveSubscription(ctx context.Context, s domain.Subscription) error

	// CreateRestaurant returns ErrConflict if the slug is taken.
	CreateRestaurant(ctx context.Context, r domain.Restaurant) error
	Restaurants(ctx context.Context, orgID uuid.UUID) ([]domain.Restaurant, error)
	Restaurant(ctx context.Context, orgID, id uuid.UUID) (domain.Restaurant, error)
	UpdateRestaurant(ctx context.Context, r domain.Restaurant) error
	CountRestaurants(ctx context.Context, orgID uuid.UUID) (int, error)

	Theme(ctx context.Context, restaurantID uuid.UUID) (domain.Theme, error)
	SaveTheme(ctx context.Context, t domain.Theme) error

	// CustomDomainForRestaurant returns the restaurant's custom domain,
	// "" if none.
	CustomDomainForRestaurant(ctx context.Context, restaurantID uuid.UUID) (string, error)
	// SetCustomDomain claims host for the restaurant ("" removes the
	// claim). Returns ErrConflict if another restaurant holds it.
	SetCustomDomain(ctx context.Context, restaurantID uuid.UUID, host string) error

	// --- Customers (diner accounts; sessions fully separate from staff) ---

	// CreateCustomer returns ErrConflict if the email is taken.
	CreateCustomer(ctx context.Context, c domain.Customer) error
	CustomerByEmail(ctx context.Context, email string) (domain.Customer, error)
	CustomerByID(ctx context.Context, id uuid.UUID) (domain.Customer, error)
	CreateCustomerSession(ctx context.Context, s domain.Session) error
	// CustomerSession resolves an unexpired customer token hash. Staff
	// session hashes never resolve here and vice versa (separate tables).
	CustomerSession(ctx context.Context, tokenHash []byte) (domain.Customer, error)
	DeleteCustomerSession(ctx context.Context, tokenHash []byte) error
	// CustomerOrders is the customer's own cross-restaurant order
	// history, newest first.
	CustomerOrders(ctx context.Context, customerID uuid.UUID, limit int) ([]domain.CustomerOrder, error)

	// --- CRM (restaurant-scoped; guest_profiles row = visibility) ---

	// TouchGuestProfile lazily creates/updates the (restaurant, customer)
	// row, bumping last_seen.
	TouchGuestProfile(ctx context.Context, restaurantID, customerID uuid.UUID) error
	// Guests lists the restaurant's guests (only those with a profile
	// row), optionally filtered by a name/email substring.
	Guests(ctx context.Context, restaurantID uuid.UUID, query string, limit int) ([]domain.GuestSummary, error)
	// GuestProfile + the guest's orders AT THIS RESTAURANT only.
	GuestProfile(ctx context.Context, restaurantID, customerID uuid.UUID) (domain.GuestProfile, domain.GuestSummary, error)
	GuestOrders(ctx context.Context, restaurantID, customerID uuid.UUID) ([]domain.GuestOrder, error)
	UpdateGuestProfile(ctx context.Context, p domain.GuestProfile) error

	// AssistantThread returns the restaurant's chat thread ID, creating
	// it on first use.
	AssistantThread(ctx context.Context, restaurantID uuid.UUID) (uuid.UUID, error)
	CreateAssistantMessage(ctx context.Context, restaurantID uuid.UUID, m domain.AssistantMessage) error
	// AssistantMessages returns the newest `limit` messages, oldest
	// first.
	AssistantMessages(ctx context.Context, restaurantID uuid.UUID, limit int) ([]domain.AssistantMessage, error)
	AssistantMessageByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.AssistantMessage, error)
	// SetAssistantMessageStatus stamps applied/discarded on a message
	// whose status is still NULL; ErrConflict if already decided.
	SetAssistantMessageStatus(ctx context.Context, restaurantID, id uuid.UUID, status string) error

	// RestaurantByID is the org-unscoped lookup for server-side
	// composition of public pages (diner entry) — never expose it on an
	// org-authenticated path; those use Restaurant(orgID, id).
	RestaurantByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)

	// RestaurantIDByDomain resolves a verified custom domain to its
	// restaurant, for Host-header routing.
	RestaurantIDByDomain(ctx context.Context, host string) (uuid.UUID, error)
}

// ErrThemeGeneration wraps any theme-generation failure (CLI error, bad
// JSON, validation reject). Callers get this or a valid Theme, never a
// half-parsed one.
var ErrThemeGeneration = errors.New("theme generation failed")

// ThemeGenerator turns a design.md brief into a PROPOSED Theme. It never
// saves — applying stays an explicit PUT by the user (AGENTS.md: AI must
// not silently control).
type ThemeGenerator interface {
	Generate(ctx context.Context, designMD string, current domain.Theme) (domain.Theme, error)
}

// ErrAssistant wraps any assistant failure (CLI error, bad JSON).
var ErrAssistant = errors.New("assistant call failed")

// Assistant is the admin chat model behind the "Assistant" screen. It
// returns a reply plus shape-validated proposed actions; tenant-scope
// validation and applying are the caller's job — nothing here executes.
type Assistant interface {
	Chat(ctx context.Context, prompt string) (reply string, actions []domain.AssistantAction, err error)
}

// BillingProvider is the payment side of subscriptions. v1 ships only a
// fake in-memory implementation; a Stripe adapter comes later.
type BillingProvider interface {
	// Subscribe provisions (fake) billing for the org on plan and returns
	// the resulting status — the fake returns SubActive immediately; a
	// real provider may return SubTrialing until the first charge clears.
	Subscribe(ctx context.Context, orgID uuid.UUID, plan domain.Plan) (domain.SubscriptionStatus, error)
	// Cancel tears down billing for the org.
	Cancel(ctx context.Context, orgID uuid.UUID) error
}

// ImageStore stores uploaded images and returns a public URL.
type ImageStore interface {
	// Put stores r under a key derived from restaurantID/filename and
	// returns the public URL. contentType must be an image MIME type —
	// validation is the caller's job.
	Put(ctx context.Context, restaurantID uuid.UUID, filename, contentType string, r io.Reader, size int64) (url string, err error)
}
