// Package outbox implements the transactional outbox pattern shared by
// every service that publishes domain events across a process boundary:
// a row is inserted in the same database transaction as the business
// write it represents, and a background Poller delivers it to a
// Deliverer at least once, retrying with backoff until delivery is
// acknowledged. See openspec/changes/split-inventory-microservice for
// the design this implements.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"aivo/internal/sharedkernel"
)

// Event is one outbox row to publish. AggregateID doubles as the
// idempotency key a consumer dedupes on — it's the existing domain
// document ID already in play (ticket ID, receipt ID, ...), not a new
// ID scheme.
type Event struct {
	ID            sharedkernel.ID
	Name          string // e.g. "TicketClosed"
	AggregateType string // e.g. "ticket"
	AggregateID   sharedkernel.ID
	RestaurantID  sharedkernel.NullID
	Payload       json.RawMessage
	OccurredAt    time.Time
}

// Publish inserts ev into tx's events table. Call it in the same
// transaction as the business write it represents: if the transaction
// rolls back, no event exists; if it commits, delivery is guaranteed
// eventually. The target table must have the shape of
// backend/migrations/platform/0004_events.up.sql (every service's own
// events table mirrors it).
func Publish(ctx context.Context, tx *sql.Tx, ev Event) error {
	if tx == nil {
		return errors.New("outbox: Publish requires a transaction")
	}
	if ev.Name == "" {
		return errors.New("outbox: Name is required")
	}
	if ev.ID == (sharedkernel.ID{}) {
		ev.ID = sharedkernel.NewID()
	}
	occurredAt := ev.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	payload := ev.Payload
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, name, aggregate_type, aggregate_id, restaurant_id, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, ev.ID, ev.Name, ev.AggregateType, ev.AggregateID, ev.RestaurantID, []byte(payload), occurredAt)
	return err
}

// PendingEvent is an unpublished row read back from the events table.
type PendingEvent struct {
	ID            sharedkernel.ID
	Name          string
	AggregateType string
	AggregateID   sharedkernel.ID
	RestaurantID  sharedkernel.NullID
	Payload       json.RawMessage
	OccurredAt    time.Time
}

// Deliverer delivers one pending event to its consumer (typically a
// gRPC call) and reports whether the consumer acknowledged it. A
// non-nil error means "not delivered yet" — the Poller retries later
// with backoff. Implementations must be safe to call more than once for
// the same event (at-least-once delivery) and should make the
// consumer's effect idempotent on PendingEvent.AggregateID, not rely on
// the Poller to dedupe.
type Deliverer interface {
	Deliver(ctx context.Context, ev PendingEvent) error
}

// fetchPending and markPublished are the two queries a Poller needs.
// Defined as package-level vars (not methods) so tests can stub them
// without a real database.
func fetchPending(ctx context.Context, db *sql.DB, limit int) ([]PendingEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, aggregate_type, aggregate_id, restaurant_id, payload, occurred_at
		FROM events
		WHERE published_at IS NULL
		ORDER BY occurred_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingEvent
	for rows.Next() {
		var ev PendingEvent
		var payload []byte
		if err := rows.Scan(&ev.ID, &ev.Name, &ev.AggregateType, &ev.AggregateID, &ev.RestaurantID, &payload, &ev.OccurredAt); err != nil {
			return nil, err
		}
		ev.Payload = payload
		out = append(out, ev)
	}
	return out, rows.Err()
}

func markPublished(ctx context.Context, db *sql.DB, id sharedkernel.ID) error {
	_, err := db.ExecContext(ctx, `UPDATE events SET published_at = now() WHERE id = $1`, id)
	return err
}

// Poller periodically delivers unpublished events from DB's events
// table via Deliver, marking published_at only once delivery is
// acknowledged. One Poller per producing service, run as a goroutine
// for the lifetime of the process (call Run once; it blocks until ctx
// is cancelled).
//
// Failed deliveries back off per-event, in memory, capped at
// MaxBackoff — not persisted, so a process restart resets backoff to
// the base Interval. That's an accepted ceiling for this pilot (see
// design.md's Risks section); upgrade path if it matters later is a
// next_attempt_at column on the events table instead of the in-memory
// map.
type Poller struct {
	DB         *sql.DB
	Deliver    Deliverer
	Interval   time.Duration // default 2s
	MaxBackoff time.Duration // default 60s
	BatchSize  int           // default 50
	Logger     *slog.Logger

	nextAttempt map[sharedkernel.ID]time.Time
	failures    map[sharedkernel.ID]int
}

func (p *Poller) Run(ctx context.Context) {
	interval := p.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		p.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pollOnce runs a single poll pass: fetch pending events, deliver the
// ones whose backoff has elapsed, mark delivered ones published. It's
// exported behavior via Run's loop but kept as its own method so tests
// can drive one pass deterministically without waiting on a ticker.
func (p *Poller) pollOnce(ctx context.Context) {
	logger := p.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxBackoff := p.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 60 * time.Second
	}
	base := p.Interval
	if base <= 0 {
		base = 2 * time.Second
	}
	batch := p.BatchSize
	if batch <= 0 {
		batch = 50
	}
	if p.nextAttempt == nil {
		p.nextAttempt = map[sharedkernel.ID]time.Time{}
	}
	if p.failures == nil {
		p.failures = map[sharedkernel.ID]int{}
	}

	pending, err := fetchPending(ctx, p.DB, batch)
	if err != nil {
		logger.Error("outbox: fetch pending failed", "error", err)
		return
	}

	now := time.Now()
	for _, ev := range pending {
		if until, ok := p.nextAttempt[ev.ID]; ok && now.Before(until) {
			continue
		}
		if err := p.Deliver.Deliver(ctx, ev); err != nil {
			p.failures[ev.ID]++
			delay := backoffFor(p.failures[ev.ID], base, maxBackoff)
			p.nextAttempt[ev.ID] = now.Add(delay)
			logger.Warn("outbox: delivery failed, will retry", "event_id", ev.ID, "name", ev.Name, "error", err, "retry_in", delay)
			continue
		}
		if err := markPublished(ctx, p.DB, ev.ID); err != nil {
			logger.Error("outbox: mark published failed", "event_id", ev.ID, "error", err)
			continue
		}
		delete(p.nextAttempt, ev.ID)
		delete(p.failures, ev.ID)
	}
}

// backoffFor returns the delay before the next attempt, doubling per
// failure from base and capped at max.
func backoffFor(failures int, base, max time.Duration) time.Duration {
	if failures <= 0 {
		return base
	}
	d := base
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	return d
}
