// Package domain holds the core types for the Menu context: the
// diner-facing digital menu, ordering, and landing page for a single
// restaurant. See internal/menu/CONTEXT.md for the authoritative glossary.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Restaurant is the tenant-owning entity that self-registers and runs a
// Menu. Every other tenant-scoped record carries a RestaurantID back to
// this type for isolation.
type Restaurant struct {
	ID        uuid.UUID
	Slug      string // readable, non-sensitive identifier used in table links
	Name      string
	CreatedAt time.Time
}

// Table is a physical dining table at a Restaurant, identified by its
// table link: {restaurant_slug}/t/{table_token}. Token is a ~128-bit
// random, unguessable value; regenerating it invalidates the old link
// immediately (e.g. after a lost/compromised QR).
type Table struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	Label        string // staff-facing identifier, e.g. "Table 5", for printing/mapping QR codes
	Token        string
	CreatedAt    time.Time
}

// Category is a named grouping of Menu items within a Restaurant's Menu.
type Category struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	Name         string
	Position     int // display order within the Menu
}

// Allergen is a tag on a Menu item drawn from the fixed EU 14 allergen
// categories, not free text.
type Allergen string

const (
	AllergenCereals     Allergen = "cereals_gluten"
	AllergenCrustaceans Allergen = "crustaceans"
	AllergenEggs        Allergen = "eggs"
	AllergenFish        Allergen = "fish"
	AllergenPeanuts     Allergen = "peanuts"
	AllergenSoybeans    Allergen = "soybeans"
	AllergenMilk        Allergen = "milk"
	AllergenNuts        Allergen = "nuts"
	AllergenCelery      Allergen = "celery"
	AllergenMustard     Allergen = "mustard"
	AllergenSesame      Allergen = "sesame"
	AllergenSulphites   Allergen = "sulphur_dioxide_sulphites"
	AllergenLupin       Allergen = "lupin"
	AllergenMolluscs    Allergen = "molluscs"
)

// Option is one choice within an OptionGroup, with a label and a price
// delta applied on top of the Menu item's base price.
type Option struct {
	ID              uuid.UUID
	Label           string
	PriceDeltaCents int
}

// OptionGroup is a named set of choices on a Menu item (e.g. "Size",
// "Add-ons"). Multi selects whether a diner may choose more than one
// Option (true) or exactly one (false).
type OptionGroup struct {
	ID      uuid.UUID
	Name    string
	Multi   bool
	Options []Option
}

// MenuItem is a single purchasable dish or drink.
type MenuItem struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	CategoryID   uuid.UUID
	Name         string
	Description  string
	PriceCents   int
	ImageURL     string
	Allergens    []Allergen
	Available    bool // false when 86'd
	OptionGroups []OptionGroup
}

// OrderLineOption is a snapshot of a chosen Option at the moment an Order
// was submitted, so later edits to the source Option never retroactively
// alter an existing Order line.
type OrderLineOption struct {
	Label           string
	PriceDeltaCents int
}

// OrderLine is a snapshot of a Menu item (name, price, chosen Options)
// captured at Order submission, plus a reference to the source MenuItem.
type OrderLine struct {
	MenuItemID     uuid.UUID
	Name           string
	UnitPriceCents int
	Qty            int
	ChosenOptions  []OrderLineOption
}

// Sentinel errors returned by NewOrderLine. Callers (internal/menu/app) map
// these to HTTP 400 — they're all diner-fixable input problems, never a
// system error.
var (
	ErrInvalidQty      = errors.New("line qty must be at least 1")
	ErrItemUnavailable = errors.New("unavailable")
	ErrUnknownOption   = errors.New("unknown option_id")
)

// NewOrderLine builds an OrderLine snapshot from item as it exists right
// now, plus the diner's chosen Option IDs and quantity — validating qty,
// item availability, and that every Option ID actually belongs to item.
// This is the one place an OrderLine gets constructed, which is what makes
// it a true snapshot: a later edit to item can never retroactively change
// an OrderLine built before that edit (see CONTEXT.md "Order line").
func NewOrderLine(item MenuItem, optionIDs []uuid.UUID, qty int) (OrderLine, error) {
	if qty < 1 {
		return OrderLine{}, ErrInvalidQty
	}
	if !item.Available {
		return OrderLine{}, fmt.Errorf("menu item %q is %w", item.Name, ErrItemUnavailable)
	}

	optionsByID := make(map[uuid.UUID]Option)
	for _, g := range item.OptionGroups {
		for _, o := range g.Options {
			optionsByID[o.ID] = o
		}
	}
	chosen := make([]OrderLineOption, 0, len(optionIDs))
	for _, oid := range optionIDs {
		opt, ok := optionsByID[oid]
		if !ok {
			return OrderLine{}, ErrUnknownOption
		}
		chosen = append(chosen, OrderLineOption{Label: opt.Label, PriceDeltaCents: opt.PriceDeltaCents})
	}

	return OrderLine{
		MenuItemID:     item.ID,
		Name:           item.Name,
		UnitPriceCents: item.PriceCents,
		Qty:            qty,
		ChosenOptions:  chosen,
	}, nil
}

// Order is a diner's submitted Cart, persisted as Order lines plus one
// optional free-text comment. Carries no payment fields — payment happens
// in person, outside the Menu context (see docs/adr/0002).
type Order struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	TableID      uuid.UUID
	Lines        []OrderLine
	Comment      string
	CreatedAt    time.Time
}

// ServiceRequestKind identifies what a diner is asking staff for.
type ServiceRequestKind string

const (
	CallWaiter  ServiceRequestKind = "call_waiter"
	RequestBill ServiceRequestKind = "request_bill"
)

// Service request lifecycle states.
const (
	ServiceRequestPending      = "pending"
	ServiceRequestAcknowledged = "acknowledged"
)

// ServiceRequest is a diner-initiated action with no items, e.g. "call
// waiter" or "request bill". Deduped per Table (not per diner session):
// at most one open (pending) request of a given Kind per Table.
type ServiceRequest struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	TableID      uuid.UUID
	Kind         ServiceRequestKind
	Status       string // "pending" or "acknowledged", see ServiceRequestPending/Acknowledged
	CreatedAt    time.Time
}

// LandingBlockType identifies which section of the closed Landing block
// catalog a LandingBlock renders.
type LandingBlockType string

const (
	LandingBlockBanner       LandingBlockType = "banner"
	LandingBlockFreeText     LandingBlockType = "free_text"
	LandingBlockOpeningHours LandingBlockType = "opening_hours"
	LandingBlockLocation     LandingBlockType = "location"
	LandingBlockSocialLinks  LandingBlockType = "social_links"
	LandingBlockContact      LandingBlockType = "contact"
)

// LandingBlock is one placeable unit on a Restaurant's Landing page. Data
// is a flexible key/value payload whose keys depend on Type:
//
//   - banner:         "image_url", "title"
//   - free_text:      "body"
//   - opening_hours:  "text"
//   - location:       "address", "map_url"
//   - social_links:   one key per platform, e.g. "instagram", "facebook"
//   - contact:        "phone"
//
// Banner and free_text are repeatable (a Restaurant may place several);
// the rest are single-instance facts about the Restaurant.
type LandingBlock struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	Type         LandingBlockType
	Position     int
	Data         map[string]string
}

// NotificationChannel is a Restaurant's configured way to receive alerts
// about new Orders and Service requests. Telegram is the only channel
// type for v1 (see docs/adr/0001); EncryptedBotToken is AES-256-GCM
// ciphertext (see docs/adr/0003) — never held as a plaintext string.
// KeyVersion identifies which master key version encrypted the token, so
// a future key rotation can re-wrap rows without a schema migration.
type NotificationChannel struct {
	RestaurantID      uuid.UUID
	TelegramChatID    string
	EncryptedBotToken []byte
	KeyVersion        int
}
