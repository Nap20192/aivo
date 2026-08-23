// Package domain holds the core types for the Platform context:
// organizations, users, sessions, subscriptions, restaurant provisioning,
// themes, and custom domains. See docs/PLATFORM.md for the contract.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"aivo/internal/sharedkernel"
)

// Organization is the billing/auth boundary. One Organization owns 1..N
// Restaurants (operational tenants).
type Organization struct {
	ID        sharedkernel.ID
	Name      string
	CreatedAt time.Time
}

// Role is a platform user's role. Owners are org-wide; managers and
// waiters are scoped to a single Restaurant.
type Role string

const (
	RoleOwner   Role = "owner"
	RoleManager Role = "manager"
	RoleWaiter  Role = "waiter"
)

// ValidRole reports whether r is one of the three known roles.
func ValidRole(r Role) bool {
	return r == RoleOwner || r == RoleManager || r == RoleWaiter
}

// User is a platform account (owner/manager/waiter). RestaurantID is nil
// for org-wide users (owners) and set for restaurant-scoped staff
// (managers, waiters).
type User struct {
	ID           sharedkernel.ID
	OrgID        sharedkernel.ID
	Email        string
	PasswordHash []byte
	Role         Role
	RestaurantID *sharedkernel.ID
	CreatedAt    time.Time
}

// CanAccessRestaurant reports whether u may act on the given Restaurant.
// Org scoping (the restaurant belongs to u.OrgID) is checked by the
// store lookup; this only adds the per-restaurant staff restriction.
func (u User) CanAccessRestaurant(restaurantID sharedkernel.ID) bool {
	if u.RestaurantID == nil {
		return true // org-wide (owner)
	}
	return *u.RestaurantID == restaurantID
}

// CanManage reports whether u may use admin (content/settings) endpoints.
func (u User) CanManage() bool { return u.Role == RoleOwner || u.Role == RoleManager }

// Session is a server-side login session. TokenHash is the SHA-256 of the
// random cookie token — the raw token is never stored.
type Session struct {
	TokenHash []byte
	UserID    sharedkernel.ID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Plan is a subscription plan.
type Plan string

const (
	PlanFree     Plan = "free"
	PlanPro      Plan = "pro"
	PlanBusiness Plan = "business"
)

// ValidPlan reports whether p is a known plan.
func ValidPlan(p Plan) bool { return p == PlanFree || p == PlanPro || p == PlanBusiness }

// Plan limits, enforced in the app layer.
const (
	FreeMaxRestaurants = 1
	FreeMaxMenuItems   = 30
)

// MaxRestaurants returns how many Restaurants a plan allows (0 = unlimited).
func (p Plan) MaxRestaurants() int {
	if p == PlanBusiness {
		return 0
	}
	return 1 // free and pro: single restaurant; business: multi
}

// MaxMenuItems returns how many menu items a plan allows per restaurant
// (0 = unlimited).
func (p Plan) MaxMenuItems() int {
	if p == PlanFree {
		return FreeMaxMenuItems
	}
	return 0
}

// SubscriptionStatus is one state of the subscription state machine:
// trialing → active → past_due → canceled.
type SubscriptionStatus string

const (
	SubTrialing SubscriptionStatus = "trialing"
	SubActive   SubscriptionStatus = "active"
	SubPastDue  SubscriptionStatus = "past_due"
	SubCanceled SubscriptionStatus = "canceled"
)

// ErrInvalidTransition is returned by Subscription.Transition for a
// disallowed state change.
var ErrInvalidTransition = errors.New("invalid subscription transition")

// allowed transitions of the state machine. canceled is terminal.
var subTransitions = map[SubscriptionStatus][]SubscriptionStatus{
	SubTrialing: {SubActive, SubPastDue, SubCanceled},
	SubActive:   {SubPastDue, SubCanceled},
	SubPastDue:  {SubActive, SubCanceled},
	SubCanceled: {},
}

// Subscription is an Organization's plan + billing state. One row per org.
type Subscription struct {
	OrgID     sharedkernel.ID
	Plan      Plan
	Status    SubscriptionStatus
	UpdatedAt time.Time
}

// Transition moves the Subscription to next, enforcing the state machine.
func (s *Subscription) Transition(next SubscriptionStatus) error {
	for _, ok := range subTransitions[s.Status] {
		if ok == next {
			s.Status = next
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, s.Status, next)
}

// HoursRow is one opening-hours line (e.g. Kitchen 17:00–22:30). Shape
// matches the admin client (web/admin/src/api/types.ts).
type HoursRow struct {
	Label string `json:"label"`
	Open  string `json:"open"`
	Close string `json:"close"`
}

// Restaurant is the platform view of an operational tenant: provisioning
// and settings. The Menu context holds its own narrower view of the same
// row (id, slug, name).
type Restaurant struct {
	ID        sharedkernel.ID
	OrgID     sharedkernel.ID
	Slug      string
	Name      string
	Address   string
	Hours     []HoursRow
	Contacts  map[string]string // e.g. "phone", "instagram", "map_url"
	CreatedAt time.Time
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSlug reports whether s is a usable restaurant slug: lowercase
// letters/digits with single hyphens, 1-64 chars, and not a reserved
// top-level path segment.
func ValidSlug(s string) bool {
	if len(s) < 1 || len(s) > 64 || !slugRe.MatchString(s) {
		return false
	}
	switch s {
	case "api", "admin", "pos", "assets", "static":
		return false
	}
	return true
}

// Theme is a Restaurant's menu customization: structured theme JSON
// (accent, bold flag, banner, fonts, CSS vars — applied as CSS custom
// properties by the menu app) plus the free-text design.md source.
type Theme struct {
	RestaurantID sharedkernel.ID
	ThemeJSON    json.RawMessage
	DesignMD     string
	UpdatedAt    time.Time
}

// CustomDomain maps an external hostname to a Restaurant. VerifiedAt nil
// means the domain is claimed but not yet serving (cert automation is out
// of scope for v1 — see docs/PLATFORM.md).
type CustomDomain struct {
	Domain       string
	RestaurantID sharedkernel.ID
	VerifiedAt   *time.Time
	CreatedAt    time.Time
}
