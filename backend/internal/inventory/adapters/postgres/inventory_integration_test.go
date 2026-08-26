package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	inv "aivo/internal/domain/inventory"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"
	"aivo/migrations"
	"aivo/pkg/migrate"
	"aivo/pkg/outbox"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// Integration tests for the inventory context against Postgres, in its
// own "inventory" schema (split-inventory-microservice, design.md D1).
// Skipped without DATABASE_URL — a database that already has
// organizations/users/restaurants (platform/menu migrations applied,
// e.g. by a prior `go run ./cmd/aivo-server`). Ledger is no longer
// in-process (design.md D2): every flow that used to post a GL journal
// synchronously now asserts the outbox event it published instead.
//
// Covers weighted-average moving cost persisted to on_hand, the fold
// invariant, receipt/write-off/stocktake posting with the right outbox
// event, calendar-versioning, cycle rejection, backdate rejection, COGS
// on sale (both the pos-facing ConsumeForSale and the gRPC-facing
// HandleTicketClosed), exact reversal, and that every outbox-publishing
// flow still commits its stock movement when the (simulated) ledger
// delivery target is unreachable.

type fixture struct {
	ctx    context.Context
	db     *sql.DB
	store  *Store
	app    *inventoryapp.App
	restID uuid.UUID
	userID uuid.UUID
}

type noSales struct{}

func (noSales) SoldDishes(context.Context, uuid.UUID, string, string) ([]ports.SaleQty, error) {
	return nil, nil
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := OpenSchemaDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Apply(ctx, db, []migrate.Source{
		{Name: "inventory", FS: migrations.FS, Dir: "inventory"},
	}); err != nil {
		t.Fatal(err)
	}

	store := NewStore(db)
	app := inventoryapp.New(store, noSales{})

	orgID, restID, userID := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'inv-org')`, orgID)
	exec(`INSERT INTO users (id, org_id, email, password_hash, role) VALUES ($1, $2, $3, $4, 'owner')`,
		userID, orgID, "u-"+uuid.New().String()[:8]+"@t", []byte("x"))
	exec(`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, 'T')`, restID, orgID, "t-"+uuid.New().String()[:8])
	t.Cleanup(func() {
		bg := context.Background()
		db.ExecContext(bg, `DELETE FROM events WHERE restaurant_id = $1`, restID)
		db.ExecContext(bg, `DELETE FROM restaurants WHERE id = $1`, restID)
		db.ExecContext(bg, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return fixture{ctx: ctx, db: db, store: store, app: app, restID: restID, userID: userID}
}

func (f fixture) product(t *testing.T, sku, name string, ptype inv.ProductType, unit string, menuItem *uuid.UUID) inv.Product {
	t.Helper()
	p, err := f.app.CreateProduct(f.ctx, f.restID, inventoryapp.ProductInput{SKU: sku, Name: name, Type: string(ptype), StockUnit: unit, MenuItemID: menuItem})
	if err != nil {
		t.Fatalf("create product %s: %v", sku, err)
	}
	return p
}

func (f fixture) postReceipt(t *testing.T, date time.Time, product uuid.UUID, qty float64, unit string, unitPrice int64) uuid.UUID {
	t.Helper()
	r, err := f.app.CreateReceipt(f.ctx, f.restID, nil, date, "", []inventoryapp.ReceiptLineInput{{ProductID: product, QtyInput: qty, Unit: unit, UnitPriceCents: unitPrice}}, f.userID)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r.ID, f.userID); err != nil {
		t.Fatalf("post receipt: %v", err)
	}
	return r.ID
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// Weighted average across two receipts, on_hand fold, and one outbox
// event per receipt.
func TestReceiptMovingAverageAndOutboxEvent(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)

	r1 := f.postReceipt(t, day(2026, 1, 1), flour.ID, 5000, inv.UnitG, 6) // 5000g @ 6c/g = 30000
	r2 := f.postReceipt(t, day(2026, 1, 2), flour.ID, 5000, inv.UnitG, 8) // 5000g @ 8c/g = 40000

	oh, err := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oh.QtyMilli != 10_000_000 || oh.ValueCents != 70000 || oh.AvgCentsPerBase() != 7 {
		t.Fatalf("on_hand qty=%d value=%d avg=%d, want 10000000/70000/7", oh.QtyMilli, oh.ValueCents, oh.AvgCentsPerBase())
	}
	assertPendingEvent(t, f, inventoryapp.EventReceiptPosted, r1, 30000)
	assertPendingEvent(t, f, inventoryapp.EventReceiptPosted, r2, 40000)
}

// Write-off issues at the average and publishes a write-off-posted event.
func TestWriteOffAtAverage(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 1000, inv.UnitG, 5) // 1000g @ 5 = 5000, avg 5

	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonSpoilage, day(2026, 1, 2), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh.QtyMilli != 800_000 || oh.ValueCents != 4000 { // 200g @ 5 = 1000 removed
		t.Errorf("after write-off qty=%d value=%d, want 800000/4000", oh.QtyMilli, oh.ValueCents)
	}
	assertPendingEvent(t, f, inventoryapp.EventWriteOffPosted, w.ID, 1000) // shrinkage debit
}

// Backdate before the last move for a product is rejected.
func TestBackdateRejected(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 10), flour.ID, 1000, inv.UnitG, 5)

	r, _ := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 5), "", []inventoryapp.ReceiptLineInput{{ProductID: flour.ID, QtyInput: 100, Unit: inv.UnitG, UnitPriceCents: 5}}, f.userID)
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r.ID, f.userID); !errors.Is(err, inventoryapp.ErrBackdated) {
		t.Errorf("backdated post: got %v, want ErrBackdated", err)
	}
}

// Reversal of a receipt rolls on_hand back by the ORIGINAL cost (not the
// current average after a later receipt), and publishes a cancel event.
func TestReceiptReversalExact(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	r1, _ := f.app.CreateReceipt(f.ctx, f.restID, nil, day(2026, 1, 1), "", []inventoryapp.ReceiptLineInput{{ProductID: flour.ID, QtyInput: 5000, Unit: inv.UnitG, UnitPriceCents: 6}}, f.userID)
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r1.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	f.postReceipt(t, day(2026, 1, 2), flour.ID, 5000, inv.UnitG, 8) // avg now 7

	if _, err := f.app.CancelReceipt(f.ctx, f.restID, r1.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	// r1 removed by its original 30000; the r2 value 40000 remains.
	if oh.QtyMilli != 5_000_000 || oh.ValueCents != 40000 {
		t.Errorf("after reversal qty=%d value=%d, want 5000000/40000", oh.QtyMilli, oh.ValueCents)
	}
	rec, _ := f.app.Receipt(f.ctx, f.restID, r1.ID)
	if rec.Status != inv.DocCancelled {
		t.Errorf("receipt status %s, want cancelled", rec.Status)
	}
	assertEventExists(t, f, inventoryapp.EventReceiptCancelled, r1.ID)
}

// Calendar versioning: a backdated version closes the previous; a cycle is
// rejected; a second version on the same day conflicts.
func TestTechCardVersioning(t *testing.T) {
	f := setup(t)
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, ptr(uuid.New()))
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)

	v1, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}}, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	// Second version on a later date closes v1 at that date.
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 5), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 250, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := f.store.TechCardByID(f.ctx, f.restID, v1.ID)
	if reloaded.ValidTo == nil || !sameDay(*reloaded.ValidTo, day(2026, 1, 5)) {
		t.Errorf("v1 valid_to = %v, want 2026-01-05", reloaded.ValidTo)
	}
	// Same-day second version conflicts.
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 5), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inventoryapp.ErrVersionExists) {
		t.Errorf("same-day version: got %v, want ErrVersionExists", err)
	}
	// Cycle: prepared A needs B, B needs A.
	pa := f.product(t, "PA", "Prep A", inv.TypePrepared, inv.UnitG, nil)
	pb := f.product(t, "PB", "Prep B", inv.TypePrepared, inv.UnitG, nil)
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, pa.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: pb.ID, QtyInput: 100, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, pb.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: pa.ID, QtyInput: 100, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inv.ErrRecipeCycle) {
		t.Errorf("cycle: got %v, want ErrRecipeCycle", err)
	}
}

func (f fixture) borschtWithFlourCard(t *testing.T) (dish inv.Product, flour inv.Product, menuItem uuid.UUID) {
	t.Helper()
	menuItem = uuid.New()
	dish = f.product(t, "BORSCHT", "Borscht", inv.TypeDish, inv.UnitPcs, &menuItem)
	flour = f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 5000, inv.UnitG, 6) // avg 6c/g
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	return dish, flour, menuItem
}

// COGS on sale via the pos-facing ConsumeForSale (shared-tx path):
// assemble depletes the ingredient and publishes a COGS-posted event; a
// re-run is idempotent (both on stock and on the outbox: no second event).
func TestCOGSOnSale(t *testing.T) {
	f := setup(t)
	_, flour, menuItem := f.borschtWithFlourCard(t)

	ticketLine := uuid.New()
	ticketID := uuid.New()
	consume := func() int64 {
		tx, _ := f.db.BeginTx(f.ctx, nil)
		cost, err := f.app.ConsumeForSale(f.ctx, tx, f.restID, f.userID, ticketID, day(2026, 1, 3),
			[]inventoryapp.SaleLine{{MenuItemID: menuItem, Qty: 1, TicketLineID: ticketLine}})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return cost
	}
	cost := consume() // 200g @ 6 = 1200
	if cost != 1200 {
		t.Errorf("cogs cost = %d, want 1200", cost)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh.QtyMilli != 4_800_000 {
		t.Errorf("flour after sale = %d, want 4800000", oh.QtyMilli)
	}
	assertPendingEvent(t, f, inventoryapp.EventCOGSPosted, ticketID, 1200)
	eventCountBefore := countEvents(t, f, inventoryapp.EventCOGSPosted, ticketID)

	// Idempotent re-run (same ticket line) consumes nothing more and
	// publishes no second event.
	if again := consume(); again != 0 {
		t.Errorf("idempotent re-run consumed %d, want 0", again)
	}
	oh2, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh2.QtyMilli != 4_800_000 {
		t.Errorf("flour after re-run = %d, want 4800000", oh2.QtyMilli)
	}
	if got := countEvents(t, f, inventoryapp.EventCOGSPosted, ticketID); got != eventCountBefore {
		t.Errorf("re-run published %d events for ticket, want unchanged at %d", got, eventCountBefore)
	}
}

// HandleTicketClosed (the gRPC-facing entry point, its own transaction —
// specs/inventory-service "Sale-triggered stock consumption is driven by
// the TicketClosed event") is idempotent: delivering the same ticket
// twice deducts stock once (service-events "Delivery is idempotent on the
// consumer side").
func TestHandleTicketClosed_Idempotent(t *testing.T) {
	f := setup(t)
	_, flour, menuItem := f.borschtWithFlourCard(t)

	ticketID := uuid.New()
	lines := []inventoryapp.SaleLine{{MenuItemID: menuItem, Qty: 1, TicketLineID: uuid.New()}}

	applied1, err := f.app.HandleTicketClosed(f.ctx, f.restID, f.userID, ticketID, day(2026, 1, 3), lines)
	if err != nil {
		t.Fatal(err)
	}
	if !applied1 {
		t.Error("first delivery: applied = false, want true")
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh.QtyMilli != 4_800_000 {
		t.Fatalf("flour after first delivery = %d, want 4800000", oh.QtyMilli)
	}

	// Redelivery of the same ticket_id: no-op.
	applied2, err := f.app.HandleTicketClosed(f.ctx, f.restID, f.userID, ticketID, day(2026, 1, 3), lines)
	if err != nil {
		t.Fatal(err)
	}
	if applied2 {
		t.Error("redelivery: applied = true, want false (no-op)")
	}
	oh2, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh2.QtyMilli != 4_800_000 {
		t.Errorf("flour after redelivery = %d, want unchanged 4800000", oh2.QtyMilli)
	}
}

// Stocktake: dry-run computes variance without saving; post books shortage
// and publishes a stocktake-posted event.
func TestStocktakeDryRunAndPost(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 1000, inv.UnitG, 5) // 1000g, avg 5

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 2), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.EnterCounts(f.ctx, f.restID, st.ID, []inventoryapp.StocktakeCountInput{{ProductID: flour.ID, QtyInput: 900, Unit: inv.UnitG}}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.app.DryRun(f.ctx, f.restID, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].VarianceQtyMilli != -100_000 || rows[0].VarianceCostCents != -500 {
		t.Fatalf("dry-run row = %+v, want variance -100000/-500", rows[0])
	}
	// Dry-run created no moves.
	moves, _ := f.store.StockMoves(f.ctx, f.restID, "", &flour.ID)
	if len(moves) != 1 { // only the receipt
		t.Fatalf("dry-run created moves: %d (want 1 receipt only)", len(moves))
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh.QtyMilli != 900_000 {
		t.Errorf("after stocktake qty=%d, want 900000", oh.QtyMilli)
	}
	assertEventExists(t, f, inventoryapp.EventStocktakePosted, st.ID)
}

// Every outbox-publishing flow (sale COGS, receipt, write-off, stocktake)
// commits its stock movement even when the ledger delivery target is
// unreachable (specs/inventory-service "Receipt posted while ledger is
// unreachable" — stock still commits; the GL entry posts once ledger
// becomes reachable). A Deliverer stub that always errors stands in for
// "ledger unreachable"; pollOnce must leave the event unpublished while
// the business row/stock move it accompanies is already durably committed
// (it committed in the same transaction as the event row, before the
// Poller ever runs).
type alwaysErrDeliverer struct{}

func (alwaysErrDeliverer) Deliver(context.Context, outbox.PendingEvent) error {
	return errors.New("ledger unreachable")
}

func TestOutboxFlows_CommitEvenIfLedgerUnreachable(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)

	receiptID := f.postReceipt(t, day(2026, 1, 1), flour.ID, 1000, inv.UnitG, 5)

	w, err := f.app.CreateWriteOff(f.ctx, f.restID, inv.ReasonSpoilage, day(2026, 1, 2), "", []inventoryapp.WriteOffLineInput{{ProductID: flour.ID, QtyInput: 100, Unit: inv.UnitG}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostWriteOff(f.ctx, f.restID, w.ID, f.userID); err != nil {
		t.Fatal(err)
	}

	st, err := f.app.StartStocktake(f.ctx, f.restID, day(2026, 1, 3), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.EnterCounts(f.ctx, f.restID, st.ID, []inventoryapp.StocktakeCountInput{{ProductID: flour.ID, QtyInput: 500, Unit: inv.UnitG}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.PostStocktake(f.ctx, f.restID, st.ID, f.userID); err != nil {
		t.Fatal(err)
	}

	menuItem := uuid.New()
	dish := f.product(t, "BORSCHT", "Borscht", inv.TypeDish, inv.UnitPcs, &menuItem)
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000, inventoryapp.TechCardMeta{},
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	ticketID := uuid.New()
	applied, err := f.app.HandleTicketClosed(f.ctx, f.restID, f.userID, ticketID, day(2026, 1, 4),
		[]inventoryapp.SaleLine{{MenuItemID: menuItem, Qty: 1, TicketLineID: uuid.New()}})
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("ticket close: applied = false, want true")
	}

	// All four business writes already committed above, independent of
	// any delivery attempt. Now simulate ledger being unreachable: a
	// single poll pass must retry (not crash, not lose events) and leave
	// every one of them unpublished.
	poller := &outbox.Poller{DB: f.db, Deliver: alwaysErrDeliverer{}}
	pollOnce(poller, f.ctx)

	for _, aggID := range []uuid.UUID{receiptID, w.ID, st.ID, ticketID} {
		var publishedAt sql.NullTime
		err := f.db.QueryRowContext(f.ctx, `SELECT published_at FROM events WHERE aggregate_id = $1`, aggID).Scan(&publishedAt)
		if err != nil {
			t.Fatalf("event for aggregate %s: %v", aggID, err)
		}
		if publishedAt.Valid {
			t.Errorf("event for aggregate %s: published_at set after a failed delivery, want still pending", aggID)
		}
	}

	// The stock movements themselves are unaffected by the delivery
	// failure — on_hand already reflects every write above.
	oh, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	// receipt +1000g, write-off -100g, stocktake count 500g (overwrites
	// expected 900g -> shortage -400g), sale -200g: 1000-100-400-200=300g.
	if oh.QtyMilli != 300_000 {
		t.Errorf("on_hand after all 4 flows = %d, want 300000 (business writes unaffected by delivery failure)", oh.QtyMilli)
	}
}

// --- helpers -----------------------------------------------------------

func ptr(u uuid.UUID) *uuid.UUID { return &u }

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// pollOnce drives exactly one Poller pass: Run always calls its
// (unexported) pollOnce before its first ctx.Done() check, so a context
// that's still valid when Run starts — but expires well before the
// default 2s poll interval's next tick — makes Run execute one real pass
// synchronously (the DB fetch+deliver attempt actually happens, unlike an
// already-cancelled context which would fail the fetch before it runs)
// and return once the timeout fires.
func pollOnce(p *outbox.Poller, ctx context.Context) {
	timed, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	p.Run(timed)
}

type eventRow struct {
	Payload     json.RawMessage
	PublishedAt sql.NullTime
}

func queryEvent(t *testing.T, f fixture, name string, aggregateID uuid.UUID) eventRow {
	t.Helper()
	var row eventRow
	err := f.db.QueryRowContext(f.ctx,
		`SELECT payload, published_at FROM events WHERE name = $1 AND aggregate_id = $2 AND restaurant_id = $3`,
		name, aggregateID, f.restID).Scan(&row.Payload, &row.PublishedAt)
	if err != nil {
		t.Fatalf("event %s for aggregate %s: %v", name, aggregateID, err)
	}
	return row
}

func countEvents(t *testing.T, f fixture, name string, aggregateID uuid.UUID) int {
	t.Helper()
	var n int
	err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM events WHERE name = $1 AND aggregate_id = $2 AND restaurant_id = $3`,
		name, aggregateID, f.restID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func assertEventExists(t *testing.T, f fixture, name string, aggregateID uuid.UUID) eventRow {
	t.Helper()
	row := queryEvent(t, f, name, aggregateID)
	if row.PublishedAt.Valid {
		t.Errorf("event %s for %s: published_at already set, want pending (no Deliverer ran)", name, aggregateID)
	}
	return row
}

// assertPendingEvent checks a Post*Journal event exists, is unpublished,
// and its payload's lines sum to wantDebit on the debit side.
func assertPendingEvent(t *testing.T, f fixture, name string, aggregateID uuid.UUID, wantDebit int64) {
	t.Helper()
	row := assertEventExists(t, f, name, aggregateID)
	var payload struct {
		Lines []ports.JournalLine `json:"lines"`
	}
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var debit int64
	for _, l := range payload.Lines {
		if l.Side == "debit" {
			debit += l.AmountCents
		}
	}
	if debit != wantDebit {
		t.Errorf("event %s payload debit total = %d, want %d (lines=%+v)", name, debit, wantDebit, payload.Lines)
	}
}
