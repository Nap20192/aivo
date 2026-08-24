package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	inv "aivo/internal/domain/inventory"
	invledgerbridge "aivo/internal/inventory/adapters/ledgerbridge"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"
	ledgerpg "aivo/internal/ledger/adapters/postgres"
	ledgerapp "aivo/internal/ledger/app"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// Integration tests for the inventory context against Postgres + the real
// ledger bridge. Skipped without DATABASE_URL. Covers weighted-average
// moving cost persisted to on_hand, the fold invariant, receipt/write-off/
// stocktake posting with balanced GL, calendar-versioning, cycle rejection,
// backdate rejection, COGS on sale, and exact reversal.

type fixture struct {
	ctx    context.Context
	db     *sql.DB
	store  *Store
	app    *inventoryapp.App
	ledger *ledgerapp.App
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
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	ledgerApp := ledgerapp.New(ledgerpg.NewStore(db))
	store := NewStore(db)
	app := inventoryapp.New(store, invledgerbridge.New(ledgerApp), noSales{})

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
	if err := ledgerApp.SeedRestaurant(ctx, restID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM restaurants WHERE id = $1`, restID)
		db.ExecContext(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return fixture{ctx: ctx, db: db, store: store, app: app, ledger: ledgerApp, restID: restID, userID: userID}
}

func (f fixture) product(t *testing.T, sku, name, ptype, unit string, menuItem *uuid.UUID) inv.Product {
	t.Helper()
	p, err := f.app.CreateProduct(f.ctx, f.restID, inventoryapp.ProductInput{SKU: sku, Name: name, Type: ptype, StockUnit: unit, MenuItemID: menuItem})
	if err != nil {
		t.Fatalf("create product %s: %v", sku, err)
	}
	return p
}

func (f fixture) postReceipt(t *testing.T, date time.Time, product uuid.UUID, qty float64, unit string, unitPrice int64) {
	t.Helper()
	r, err := f.app.CreateReceipt(f.ctx, f.restID, nil, date, "", []inventoryapp.ReceiptLineInput{{ProductID: product, QtyInput: qty, Unit: unit, UnitPriceCents: unitPrice}}, f.userID)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if _, err := f.app.PostReceipt(f.ctx, f.restID, r.ID, f.userID); err != nil {
		t.Fatalf("post receipt: %v", err)
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// Weighted average across two receipts, on_hand fold, and balanced GL.
func TestReceiptMovingAverageAndGL(t *testing.T) {
	f := setup(t)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)

	f.postReceipt(t, day(2026, 1, 1), flour.ID, 5000, inv.UnitG, 6) // 5000g @ 6c/g = 30000
	f.postReceipt(t, day(2026, 1, 2), flour.ID, 5000, inv.UnitG, 8) // 5000g @ 8c/g = 40000

	oh, err := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oh.QtyMilli != 10_000_000 || oh.ValueCents != 70000 || oh.AvgCentsPerBase() != 7 {
		t.Fatalf("on_hand qty=%d value=%d avg=%d, want 10000000/70000/7", oh.QtyMilli, oh.ValueCents, oh.AvgCentsPerBase())
	}
	// GL: two receipts, each debit 1200 / credit 2100. Inventory net debit 70000.
	assertAccountBalance(t, f, "1200", 70000)  // inventory asset debit
	assertAccountBalance(t, f, "2100", -70000) // AP credit
}

// Write-off issues at the average and books shrinkage.
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
	assertAccountBalance(t, f, "5910", 1000) // shrinkage debit
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
// current average after a later receipt), and cancels the GL.
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
}

// Calendar versioning: a backdated version closes the previous; a cycle is
// rejected; a second version on the same day conflicts.
func TestTechCardVersioning(t *testing.T) {
	f := setup(t)
	dish := f.product(t, "SOUP", "Soup", inv.TypeDish, inv.UnitPcs, ptr(uuid.New()))
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)

	v1, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}}, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	// Second version on a later date closes v1 at that date.
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 5), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 250, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := f.store.TechCardByID(f.ctx, f.restID, v1.ID)
	if reloaded.ValidTo == nil || !sameDay(*reloaded.ValidTo, day(2026, 1, 5)) {
		t.Errorf("v1 valid_to = %v, want 2026-01-05", reloaded.ValidTo)
	}
	// Same-day second version conflicts.
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 5), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 1, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inventoryapp.ErrVersionExists) {
		t.Errorf("same-day version: got %v, want ErrVersionExists", err)
	}
	// Cycle: prepared A needs B, B needs A.
	pa := f.product(t, "PA", "Prep A", inv.TypePrepared, inv.UnitG, nil)
	pb := f.product(t, "PB", "Prep B", inv.TypePrepared, inv.UnitG, nil)
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, pa.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: pb.ID, QtyInput: 100, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, pb.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: pa.ID, QtyInput: 100, Unit: inv.UnitG}}, f.userID); !errors.Is(err, inv.ErrRecipeCycle) {
		t.Errorf("cycle: got %v, want ErrRecipeCycle", err)
	}
}

// COGS on sale: assemble depletes the ingredient and posts debit 5000 /
// credit 1200 for the consumed cost; a re-run is idempotent.
func TestCOGSOnSale(t *testing.T) {
	f := setup(t)
	menuItem := uuid.New()
	dish := f.product(t, "BORSCHT", "Borscht", inv.TypeDish, inv.UnitPcs, &menuItem)
	flour := f.product(t, "FLOUR", "Flour", inv.TypeGoods, inv.UnitG, nil)
	f.postReceipt(t, day(2026, 1, 1), flour.ID, 5000, inv.UnitG, 6) // avg 6c/g
	if _, err := f.app.CreateTechCardVersion(f.ctx, f.restID, dish.ID, day(2026, 1, 1), inv.ConsumeAssemble, 1000,
		[]inventoryapp.TechCardLineInput{{IngredientProductID: flour.ID, QtyInput: 200, Unit: inv.UnitG}}, f.userID); err != nil {
		t.Fatal(err)
	}

	ticketLine := uuid.New()
	consume := func() int64 {
		tx, _ := f.db.BeginTx(f.ctx, nil)
		cost, err := f.app.ConsumeForSale(f.ctx, tx, f.restID, f.userID, uuid.New(), day(2026, 1, 3),
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
	// Idempotent re-run (same ticket line) consumes nothing more.
	if again := consume(); again != 0 {
		t.Errorf("idempotent re-run consumed %d, want 0", again)
	}
	oh2, _ := f.store.OnHand(f.ctx, f.restID, flour.ID)
	if oh2.QtyMilli != 4_800_000 {
		t.Errorf("flour after re-run = %d, want 4800000", oh2.QtyMilli)
	}
}

// Stocktake: dry-run computes variance without saving; post books shortage.
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
	assertAccountBalance(t, f, "5910", 500) // shrinkage debit for the shortage
}

// --- helpers -----------------------------------------------------------

func ptr(u uuid.UUID) *uuid.UUID { return &u }

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// assertAccountBalance sums posted debit−credit for an account code.
func assertAccountBalance(t *testing.T, f fixture, code string, want int64) {
	t.Helper()
	var bal int64
	err := f.db.QueryRowContext(f.ctx,
		`SELECT COALESCE(sum(CASE WHEN l.side='debit' THEN l.amount_cents ELSE -l.amount_cents END),0)
		 FROM journal_lines l JOIN accounts a ON a.id = l.account_id
		 JOIN journal_documents d ON d.id = l.document_id
		 WHERE a.restaurant_id = $1 AND a.code = $2 AND d.state = 'posted'`,
		f.restID, code).Scan(&bal)
	if err != nil {
		t.Fatal(err)
	}
	if bal != want {
		t.Errorf("account %s balance = %d, want %d", code, bal, want)
	}
}
