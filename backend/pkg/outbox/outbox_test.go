package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"aivo/internal/sharedkernel"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

func TestBackoffFor(t *testing.T) {
	base := 2 * time.Second
	max := 60 * time.Second
	cases := []struct {
		name     string
		failures int
		want     time.Duration
	}{
		{"zero failures returns base", 0, base},
		{"negative failures returns base", -1, base},
		{"first failure returns base", 1, base},
		{"second failure doubles", 2, 4 * time.Second},
		{"third failure doubles again", 3, 8 * time.Second},
		{"caps at max", 20, max},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := backoffFor(tc.failures, base, max)
			if got != tc.want {
				t.Fatalf("backoffFor(%d) = %v, want %v", tc.failures, got, tc.want)
			}
		})
	}
}

func TestPublish_RequiresTx(t *testing.T) {
	err := Publish(context.Background(), nil, Event{Name: "X"})
	if err == nil {
		t.Fatal("expected error for nil tx, got nil")
	}
}

func TestPublish_RequiresName(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	err = Publish(context.Background(), tx, Event{})
	if err == nil {
		t.Fatal("expected error for empty Name, got nil")
	}
}

// fakeDeliverer records every delivery attempt and lets the test decide
// whether each one succeeds.
type fakeDeliverer struct {
	shouldFail func(ev PendingEvent) bool
	attempts   []PendingEvent
}

func (f *fakeDeliverer) Deliver(_ context.Context, ev PendingEvent) error {
	f.attempts = append(f.attempts, ev)
	if f.shouldFail != nil && f.shouldFail(ev) {
		return errors.New("delivery refused")
	}
	return nil
}

func TestPublishAndPoll_Integration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	t.Run("commit then poll delivers and marks published", func(t *testing.T) {
		resetEvents(t, db)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		ev := Event{
			Name:          "TicketClosed",
			AggregateType: "ticket",
			AggregateID:   sharedkernel.NewID(),
			Payload:       json.RawMessage(`{"total_cents":1234}`),
		}
		if err := Publish(ctx, tx, ev); err != nil {
			t.Fatalf("publish: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}

		deliverer := &fakeDeliverer{}
		p := &Poller{DB: db, Deliver: deliverer, Interval: time.Millisecond}
		p.pollOnce(ctx)

		if len(deliverer.attempts) != 1 {
			t.Fatalf("expected 1 delivery attempt, got %d", len(deliverer.attempts))
		}
		if deliverer.attempts[0].AggregateID != ev.AggregateID {
			t.Fatalf("delivered wrong event: got %v want %v", deliverer.attempts[0].AggregateID, ev.AggregateID)
		}

		var publishedAt sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT published_at FROM events WHERE aggregate_id = $1`, ev.AggregateID).Scan(&publishedAt); err != nil {
			t.Fatalf("query published_at: %v", err)
		}
		if !publishedAt.Valid {
			t.Fatal("expected published_at to be set after a successful delivery")
		}

		// A second poll must not redeliver an already-published event.
		p.pollOnce(ctx)
		if len(deliverer.attempts) != 1 {
			t.Fatalf("expected no redelivery of a published event, got %d total attempts", len(deliverer.attempts))
		}
	})

	t.Run("rollback publishes nothing", func(t *testing.T) {
		resetEvents(t, db)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		id := sharedkernel.NewID()
		if err := Publish(ctx, tx, Event{Name: "X", AggregateType: "t", AggregateID: id}); err != nil {
			t.Fatalf("publish: %v", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback: %v", err)
		}

		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE aggregate_id = $1`, id).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 event rows after rollback, got %d", count)
		}
	})

	t.Run("failed delivery is retried, not marked published", func(t *testing.T) {
		resetEvents(t, db)
		tx, _ := db.BeginTx(ctx, nil)
		id := sharedkernel.NewID()
		if err := Publish(ctx, tx, Event{Name: "X", AggregateType: "t", AggregateID: id}); err != nil {
			t.Fatalf("publish: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}

		failFirst := true
		deliverer := &fakeDeliverer{shouldFail: func(ev PendingEvent) bool {
			if failFirst {
				failFirst = false
				return true
			}
			return false
		}}
		// Zero backoff base so the second pollOnce (after the induced
		// failure) is immediately eligible for retry.
		p := &Poller{DB: db, Deliver: deliverer, Interval: time.Nanosecond}
		p.pollOnce(ctx) // fails once, schedules a retry
		var publishedAt sql.NullTime
		db.QueryRowContext(ctx, `SELECT published_at FROM events WHERE aggregate_id = $1`, id).Scan(&publishedAt)
		if publishedAt.Valid {
			t.Fatal("must not be published after a failed delivery")
		}

		time.Sleep(3 * time.Millisecond) // clear the in-memory backoff window
		p.pollOnce(ctx)                  // succeeds this time
		db.QueryRowContext(ctx, `SELECT published_at FROM events WHERE aggregate_id = $1`, id).Scan(&publishedAt)
		if !publishedAt.Valid {
			t.Fatal("expected published_at to be set after the retried delivery succeeds")
		}
		if len(deliverer.attempts) != 2 {
			t.Fatalf("expected exactly 2 delivery attempts, got %d", len(deliverer.attempts))
		}
	})
}

func TestPollOnce_FetchPendingErrorDoesNotPanic(t *testing.T) {
	db := openTestDB(t)
	db.Close() // force every query against it to fail

	deliverer := &fakeDeliverer{}
	p := &Poller{DB: db, Deliver: deliverer, Interval: time.Millisecond}
	p.pollOnce(context.Background()) // must not panic

	if len(deliverer.attempts) != 0 {
		t.Fatalf("expected no delivery attempts when fetch fails, got %d", len(deliverer.attempts))
	}
}

func TestPollOnce_MarkPublishedErrorDoesNotPanic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	resetEvents(t, db)

	tx, _ := db.BeginTx(ctx, nil)
	id := sharedkernel.NewID()
	if err := Publish(ctx, tx, Event{Name: "X", AggregateType: "t", AggregateID: id}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Deliverer "succeeds" but closes the DB first, so the poller's
	// follow-up markPublished call fails — must be handled, not panic.
	closingDeliverer := &fakeDeliverer{}
	p := &Poller{DB: db, Deliver: closeThenSucceed{db: db, inner: closingDeliverer}, Interval: time.Millisecond}
	p.pollOnce(ctx) // must not panic despite markPublished failing
}

// closeThenSucceed wraps a Deliverer, closing db before reporting
// success, so the caller's next DB call (markPublished) observes a
// closed connection and returns an error.
type closeThenSucceed struct {
	db    *sql.DB
	inner *fakeDeliverer
}

func (c closeThenSucceed) Deliver(ctx context.Context, ev PendingEvent) error {
	_ = c.inner.Deliver(ctx, ev)
	return c.db.Close()
}

func TestRun_DeliversThenStopsOnCancel(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	resetEvents(t, db)

	tx, _ := db.BeginTx(ctx, nil)
	id := sharedkernel.NewID()
	if err := Publish(ctx, tx, Event{Name: "X", AggregateType: "t", AggregateID: id}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	deliverer := &fakeDeliverer{}
	p := &Poller{DB: db, Deliver: deliverer, Interval: time.Millisecond}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		p.Run(runCtx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for len(deliverer.attempts) == 0 {
		select {
		case <-deadline:
			cancel()
			t.Fatal("Run never delivered the pending event")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Pin to a single physical connection and isolate in a dedicated
	// schema so this test never touches the real (possibly shared dev)
	// events table that lives in the default schema.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS outbox_pkg_test`); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO outbox_pkg_test`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id             uuid PRIMARY KEY,
			name           text NOT NULL,
			aggregate_type text NOT NULL,
			aggregate_id   uuid NOT NULL,
			restaurant_id  uuid,
			payload        jsonb NOT NULL DEFAULT '{}'::jsonb,
			occurred_at    timestamptz NOT NULL DEFAULT now(),
			published_at   timestamptz
		)
	`); err != nil {
		t.Fatalf("create events table: %v", err)
	}
	return db
}

func resetEvents(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM events`); err != nil {
		t.Fatalf("reset events table: %v", err)
	}
}
