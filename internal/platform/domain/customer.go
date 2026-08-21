package domain

import (
	"time"

	"github.com/google/uuid"
)

// Customer is a platform-global diner account (optional — the anonymous
// diner flow stays). Entirely separate from staff Users: different
// table, different session store, different cookie.
type Customer struct {
	ID           uuid.UUID
	Email        string
	PasswordHash []byte
	Name         string
	Phone        *string
	CreatedAt    time.Time
}

// CustomerOrder is one line of a customer's own order history
// (customer/me view — spans restaurants).
type CustomerOrder struct {
	RestaurantName string
	CreatedAt      time.Time
	TotalCents     int
	Lines          []CustomerOrderLine
}

type CustomerOrderLine struct {
	Name           string
	Qty            int
	UnitPriceCents int // base price; TotalCents includes option deltas
	TotalCents     int
	Options        []string
}

// GuestProfile is the CRM row: what one restaurant knows about one
// customer. Created lazily on the first linked order/handoff; the row's
// existence IS the privacy boundary — no row, no visibility.
type GuestProfile struct {
	RestaurantID uuid.UUID
	CustomerID   uuid.UUID
	Notes        string
	Tags         []string
	FirstSeen    time.Time
	LastSeen     time.Time
}

// GuestSummary is one CRM list entry (visits/spend computed from the
// restaurant's own orders only).
type GuestSummary struct {
	Customer        Customer
	Visits          int
	TotalSpentCents int
	LastSeen        time.Time
	Tags            []string
}

// GuestOrder is one order in the CRM guest detail (this restaurant only).
type GuestOrder struct {
	ID         uuid.UUID
	CreatedAt  time.Time
	TableLabel string
	TotalCents int
	Lines      []CustomerOrderLine
}
