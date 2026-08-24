// Package billing holds BillingProvider implementations. v1 ships only
// Fake — no real payment processing (see docs/PLATFORM.md); a Stripe
// adapter slots in behind the same port later.
package billing

import (
	"context"
	"log"
	"sync"

	"aivo/internal/domain/platform"
	"aivo/internal/platform/ports"

	"uuid"
)

// Fake is an in-memory BillingProvider that approves everything
// immediately. It remembers what each org subscribed to only so logs and
// tests can inspect it.
type Fake struct {
	mu   sync.Mutex
	subs map[uuid.UUID]domain.Plan
}

var _ ports.BillingProvider = (*Fake)(nil)

func NewFake() *Fake { return &Fake{subs: map[uuid.UUID]domain.Plan{}} }

func (f *Fake) Subscribe(_ context.Context, orgID uuid.UUID, plan domain.Plan) (domain.SubscriptionStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[orgID] = plan
	log.Printf("billing(fake): org %s subscribed to %s", orgID, plan)
	return domain.SubActive, nil
}

func (f *Fake) Cancel(_ context.Context, orgID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.subs, orgID)
	log.Printf("billing(fake): org %s canceled", orgID)
	return nil
}
