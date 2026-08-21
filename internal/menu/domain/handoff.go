package domain

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// HandoffTTL is how long a pickup code stays valid.
const HandoffTTL = 15 * time.Minute

// HandoffCodeAlphabet omits 0/O/1/I so a diner can read the code out or
// the waiter can type it without ambiguity.
const HandoffCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const handoffCodeLen = 6

// NewHandoffCode returns a random 6-char pickup code from the unambiguous
// alphabet (~1e9 combinations; uniqueness among active codes is enforced
// by the store's partial unique index, callers retry on conflict).
func NewHandoffCode() (string, error) {
	b := make([]byte, handoffCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("handoff code: %w", err)
	}
	for i := range b {
		b[i] = HandoffCodeAlphabet[int(b[i])%len(HandoffCodeAlphabet)]
	}
	return string(b), nil
}

// HandoffLine is one line of a handed-off cart: the display snapshot
// (name/price/options, validated at creation) plus the source ids so
// accepting re-runs the normal POS add-lines path.
type HandoffLine struct {
	MenuItemID     uuid.UUID         `json:"menu_item_id"`
	OptionIDs      []uuid.UUID       `json:"option_ids,omitempty"`
	Qty            int               `json:"qty"`
	Name           string            `json:"name"`
	UnitPriceCents int               `json:"unit_price_cents"`
	Options        []OrderLineOption `json:"options,omitempty"`
}

// Handoff is a cart stored server-side under a short pickup code, shown
// by the diner to the waiter who pulls it into the table ticket.
// Single-use (UsedAt), TTL-bound, at most one active per Table.
type Handoff struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	TableID      uuid.UUID
	CustomerID   *uuid.UUID
	Code         string
	Lines        []HandoffLine
	Note         string
	ExpiresAt    time.Time
	UsedAt       *time.Time
	CreatedAt    time.Time
}

// Active reports whether the code can still be accepted at now.
func (h Handoff) Active(now time.Time) bool {
	return h.UsedAt == nil && now.Before(h.ExpiresAt)
}

// TotalCents is the handed-off cart total (unit prices include no option
// deltas — they're separate — so sum both).
func (h Handoff) TotalCents() int {
	total := 0
	for _, l := range h.Lines {
		unit := l.UnitPriceCents
		for _, o := range l.Options {
			unit += o.PriceDeltaCents
		}
		total += unit * l.Qty
	}
	return total
}
