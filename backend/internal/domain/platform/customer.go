package domain

import (
	"time"

	"aivo/internal/sharedkernel"
)

// Customer is a platform-global diner account (optional — the anonymous
// diner flow stays). Entirely separate from staff Users: different
// table, different session store, different cookie.
type Customer struct {
	ID           sharedkernel.ID
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
	RestaurantID sharedkernel.ID
	CustomerID   sharedkernel.ID
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
	ID         sharedkernel.ID
	CreatedAt  time.Time
	TableLabel string
	TotalCents int
	Lines      []CustomerOrderLine
}
