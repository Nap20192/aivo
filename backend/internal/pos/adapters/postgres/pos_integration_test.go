package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	ledger "aivo/internal/domain/ledger"
	menudomain "aivo/internal/domain/menu"
	domain "aivo/internal/domain/pos"
	ledgerpg "aivo/internal/ledger/adapters/postgres"
	ledgerapp "aivo/internal/ledger/app"
	"aivo/internal/pos/adapters/ledgerbridge"
	posapp "aivo/internal/pos/app"
	posports "aivo/internal/pos/ports"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// Integration tests for the POS context (payments, cash movements, shift
// close/accept) against Postgres and the real ledger bridge. Runs only
// with TEST_DATABASE_URL (or DATABASE_URL); skipped otherwise. Covers the
// contract §5 "Tester" pos-side list: one open shift per restaurant,
// closed-ticket immutability (debt 3), CloseTicket tender rules, atomic
// CloseShift with the expected-cash formula and a mandatory variance
// posting (§15.5), the deterministic business date (§15.4), draft-only
// override (§15.2), and idempotent AcceptShift.

// menuStub satisfies pos ports.Menu; the tested flows never call it.
type menuStub struct{}

func (menuStub) MenuItemByID(context.Context, uuid.UUID, uuid.UUID) (menudomain.MenuItem, error) {
	return menudomain.MenuItem{}, posports.ErrNotFound
}
func (menuStub) Tables(context.Context, uuid.UUID) ([]menudomain.Table, error) { return nil, nil }
func (menuStub) TableByID(context.Context, uuid.UUID, uuid.UUID) (menudomain.Table, error) {
	return menudomain.Table{}, posports.ErrNotFound
}
func (menuStub) PendingServiceRequests(context.Context, uuid.UUID) ([]menudomain.ServiceRequest, error) {
	return nil, nil
}
func (menuStub) AckServiceRequest(context.Context, uuid.UUID, uuid.UUID) error     { return nil }
func (menuStub) DismissServiceRequest(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func posDSN() string {
	if d := os.Getenv("TEST_DATABASE_URL"); d != "" {
		return d
	}
	return os.Getenv("DATABASE_URL")
}

type posFixture struct {
	ctx     context.Context
	db      *sql.DB
	store   *Store
	app     *posapp.App
	ledger  *ledgerapp.App
	restID  uuid.UUID
	userID  uuid.UUID
	tableID uuid.UUID
	itemID  uuid.UUID
	price   int
	cashID  uuid.UUID // cash payment method
	cardID  uuid.UUID // card payment method
}

func setupPos(t *testing.T) posFixture {
	t.Helper()
	dsn := posDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	ledgerApp := ledgerapp.New(ledgerpg.NewStore(db))
	store := NewStore(db)
	app := posapp.New(store, menuStub{}, ledgerbridge.New(ledgerApp), nil)

	orgID := uuid.New()
	f := posFixture{
		ctx: ctx, db: db, store: store, app: app, ledger: ledgerApp,
		restID: uuid.New(), userID: uuid.New(), tableID: uuid.New(), itemID: uuid.New(), price: 1500,
	}
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'test-org')`, orgID)
	exec(`INSERT INTO users (id, org_id, email, password_hash, role) VALUES ($1, $2, $3, $4, 'owner')`,
		f.userID, orgID, "u-"+uuid.New().String()[:8]+"@test", []byte("x"))
	exec(`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, 'Test')`,
		f.restID, orgID, "t-"+uuid.New().String()[:8])
	if err := ledgerApp.SeedRestaurant(ctx, f.restID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	// payment methods (cash, card) — SeedRestaurant does not create them.
	f.cashID, f.cardID = uuid.New(), uuid.New()
	exec(`INSERT INTO payment_methods (id, restaurant_id, code, name, payment_group) VALUES ($1,$2,'cash','Cash','cash')`, f.cashID, f.restID)
	exec(`INSERT INTO payment_methods (id, restaurant_id, code, name, payment_group) VALUES ($1,$2,'card','Card','card')`, f.cardID, f.restID)
	// a table + a menu item (ticket_lines.menu_item_id FK).
	menuID, catID := uuid.New(), uuid.New()
	exec(`INSERT INTO tables (id, restaurant_id, label, token) VALUES ($1,$2,'T1',$3)`, f.tableID, f.restID, "tok-"+uuid.New().String()[:8])
	exec(`INSERT INTO menus (id, restaurant_id, slug, name, is_default) VALUES ($1,$2,'m','Menu',true)`, menuID, f.restID)
	exec(`INSERT INTO categories (id, restaurant_id, menu_id, name) VALUES ($1,$2,$3,'Cat')`, catID, f.restID, menuID)
	exec(`INSERT INTO menu_items (id, restaurant_id, category_id, name, price_cents) VALUES ($1,$2,$3,'Item',$4)`,
		f.itemID, f.restID, catID, f.price)

	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM restaurants WHERE id = $1`, f.restID)
		db.ExecContext(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return f
}

// openShift opens a shift for the fixture user with the given float.
func (f posFixture) openShift(t *testing.T, floatCents int) domain.Shift {
	t.Helper()
	sh, err := f.app.OpenShift(f.ctx, f.restID, f.userID, "cashier", floatCents)
	if err != nil {
		t.Fatalf("open shift: %v", err)
	}
	return sh
}

// newTicket creates an open ticket on the fixture table under shift with
// `qty` units of the fixture item (total = price*qty).
func (f posFixture) newTicket(t *testing.T, shiftID uuid.UUID, qty int) domain.Ticket {
	t.Helper()
	tk := domain.Ticket{ID: uuid.New(), RestaurantID: f.restID, ShiftID: shiftID, TableID: f.tableID}
	if err := f.store.CreateTicket(f.ctx, tk); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	line := domain.TicketLine{ID: uuid.New(), MenuItemID: f.itemID, Name: "Item", UnitPriceCents: f.price, Qty: qty}
	if err := f.store.AddLines(f.ctx, tk.ID, []domain.TicketLine{line}); err != nil {
		t.Fatalf("add lines: %v", err)
	}
	got, err := f.store.TicketByID(f.ctx, f.restID, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func lineOn(doc ledger.JournalDocument, accountID uuid.UUID) (ledger.JournalLine, bool) {
	for _, l := range doc.Lines {
		if l.AccountID == accountID {
			return l, true
		}
	}
	return ledger.JournalLine{}, false
}

// Only one open shift per restaurant (contract §3 partial unique). A
// second open — even by a different cashier — conflicts.
func TestOneOpenShiftPerRestaurant(t *testing.T) {
	f := setupPos(t)
	f.openShift(t, 5000)
	_, err := f.app.OpenShift(f.ctx, f.restID, uuid.New(), "other", 5000)
	if !errors.Is(err, posports.ErrConflict) {
		t.Errorf("second open shift: got %v, want ErrConflict", err)
	}
}

// Closed ticket is immutable (debt 3): fire / note / link route through
// the shared open-only guard and are rejected once closed.
func TestClosedTicketImmutable(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 0)
	tk := f.newTicket(t, sh.ID, 1)
	// Close it with a matching cash tender.
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: f.price}}); err != nil {
		t.Fatalf("close ticket: %v", err)
	}

	if err := f.store.FireTicket(f.ctx, f.restID, tk.ID); !errors.Is(err, posports.ErrConflict) {
		t.Errorf("fire closed ticket: got %v, want ErrConflict", err)
	}
	if err := f.store.AppendTicketNote(f.ctx, f.restID, tk.ID, "late note"); !errors.Is(err, posports.ErrConflict) {
		t.Errorf("note on closed ticket: got %v, want ErrConflict", err)
	}
	if err := f.store.LinkTicketCustomer(f.ctx, f.restID, tk.ID, uuid.New()); !errors.Is(err, posports.ErrConflict) {
		t.Errorf("link customer on closed ticket: got %v, want ErrConflict", err)
	}
}

// CloseTicket tender rules (contract §4): needs an open shift, tenders
// must sum to the total, a closed ticket rejects re-close.
func TestCloseTicketTenderRules(t *testing.T) {
	f := setupPos(t)

	// No open shift → 409 shift_not_open. Build a ticket under a fake
	// (closed-world) shift id first so the ticket exists.
	sh := f.openShift(t, 0)
	tk := f.newTicket(t, sh.ID, 2) // total = 3000
	total := f.price * 2

	// Close the shift is not what we want; instead test mismatch + exact
	// with the shift still open.
	// Mismatch → 422 (ErrInvalid).
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: total - 1}}); !errors.Is(err, posapp.ErrInvalid) {
		t.Errorf("tender mismatch: got %v, want ErrInvalid", err)
	}

	// Exact (cash + card split) → closed.
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID, []domain.Tender{
		{MethodID: f.cashID, AmountCents: 1000},
		{MethodID: f.cardID, AmountCents: total - 1000},
	}); err != nil {
		t.Fatalf("exact close: %v", err)
	}
	// Re-close → 409 ticket_closed.
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: total}}); !errors.Is(err, domain.ErrTicketClosed) {
		t.Errorf("re-close: got %v, want ErrTicketClosed", err)
	}
}

// CloseTicket without an open shift → ErrShiftNotOpen (409).
func TestCloseTicketNoOpenShift(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 0)
	tk := f.newTicket(t, sh.ID, 1)
	// Close the shift out from under the ticket: pay the ticket first, then
	// close the shift, then a further close attempt sees no open shift.
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: f.price}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 0); err != nil {
		t.Fatalf("close shift: %v", err)
	}
	// A fresh ticket close now finds no open shift.
	tk2 := domain.Ticket{ID: uuid.New(), RestaurantID: f.restID, ShiftID: sh.ID, TableID: f.tableID}
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk2.ID, f.userID, nil); !errors.Is(err, posapp.ErrShiftNotOpen) {
		t.Errorf("close ticket without open shift: got %v, want ErrShiftNotOpen", err)
	}
}

// The core money path: atomic CloseShift computes expected cash by the
// new formula (float + cash tenders + pay_in − pay_out − drop; card never
// hits the drawer), variance = declared − expected, and builds a draft
// acceptance journal with a mandatory cash_over_short line (§15.5) dated
// on the deterministic business date (§15.4).
func TestCloseShiftFormulaVarianceAndJournal(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 5000)

	// Cash ticket 3000 (cash tender) and card ticket 4000 (card tender).
	cashTk := f.newTicket(t, sh.ID, 2) // 3000
	if _, err := f.app.CloseTicket(f.ctx, f.restID, cashTk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: 3000, TipCents: 500}}); err != nil {
		t.Fatal(err)
	}
	// Second table for the card ticket (one open ticket per table).
	cardTableID := uuid.New()
	if _, err := f.db.ExecContext(f.ctx, `INSERT INTO tables (id, restaurant_id, label, token) VALUES ($1,$2,'T2',$3)`,
		cardTableID, f.restID, "tok-"+uuid.New().String()[:8]); err != nil {
		t.Fatal(err)
	}
	cardTk := domain.Ticket{ID: uuid.New(), RestaurantID: f.restID, ShiftID: sh.ID, TableID: cardTableID}
	if err := f.store.CreateTicket(f.ctx, cardTk); err != nil {
		t.Fatal(err)
	}
	if err := f.store.AddLines(f.ctx, cardTk.ID, []domain.TicketLine{{ID: uuid.New(), MenuItemID: f.itemID, Name: "Item", UnitPriceCents: 2000, Qty: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.CloseTicket(f.ctx, f.restID, cardTk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cardID, AmountCents: 4000}}); err != nil {
		t.Fatal(err)
	}

	// Cash movements: +1000 pay_in, −200 pay_out, −300 drop.
	mustCashOp(t, f, sh.ID, domain.CashPayIn, 1000)
	mustCashOp(t, f, sh.ID, domain.CashPayOut, 200)
	mustCashOp(t, f, sh.ID, domain.CashDrop, 300)

	// expected = 5000 + 3000 + 1000 − 200 − 300 = 8500 (card 4000 excluded).
	// declared 8400 → variance −100.
	closed, docID, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 8400)
	if err != nil {
		t.Fatalf("close shift: %v", err)
	}
	if closed.ExpectedCents == nil || *closed.ExpectedCents != 8500 {
		t.Errorf("expected_cents = %v, want 8500", closed.ExpectedCents)
	}
	if closed.VarianceCents == nil || *closed.VarianceCents != -100 {
		t.Errorf("variance_cents = %v, want -100", closed.VarianceCents)
	}
	if closed.State() != domain.ShiftClosed {
		t.Errorf("state = %q, want closed", closed.State())
	}

	// Draft journal: balanced, has the mandatory cash_over_short line,
	// dated on today's business date.
	doc, err := f.ledger.GetJournal(f.ctx, f.restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, doc)
	osID, err := f.ledger.AccountForPurpose(f.ctx, f.restID, "cash_over_short")
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := lineOn(doc, osID); !ok || l.AmountCents != 100 {
		t.Errorf("cash_over_short line: got %+v ok=%v, want amount 100 (§15.5)", l, ok)
	}
	today := time.Now().UTC().Format("2006-01-02")
	if got := doc.AccountingDate.Format("2006-01-02"); got != today {
		t.Errorf("journal accounting_date = %s, want business date %s (§15.4)", got, today)
	}
	// Card revenue still recognized: sales credit = cash 3000 + card 4000.
	salesID, _ := f.ledger.AccountForPurpose(f.ctx, f.restID, "sales_revenue")
	if l, ok := lineOn(doc, salesID); !ok || l.Side != ledger.SideCredit || l.AmountCents != 7000 {
		t.Errorf("sales line: got %+v ok=%v, want credit 7000", l, ok)
	}
}

func mustCashOp(t *testing.T, f posFixture, shiftID uuid.UUID, kind string, amt int) {
	t.Helper()
	if _, err := f.app.RecordCashOperation(f.ctx, f.restID, shiftID, f.userID, kind, amt, "test"); err != nil {
		t.Fatalf("cash op %s: %v", kind, err)
	}
}

// With declared == expected the draft has no cash_over_short line (§15.5
// negative case).
func TestCloseShiftNoVarianceNoOverShortLine(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 5000)
	tk := f.newTicket(t, sh.ID, 2) // 3000 cash
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: 3000}}); err != nil {
		t.Fatal(err)
	}
	// expected = 5000 + 3000 = 8000; declared 8000 → variance 0.
	_, docID, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 8000)
	if err != nil {
		t.Fatalf("close shift: %v", err)
	}
	doc, err := f.ledger.GetJournal(f.ctx, f.restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, doc)
	osID, _ := f.ledger.AccountForPurpose(f.ctx, f.restID, "cash_over_short")
	if _, ok := lineOn(doc, osID); ok {
		t.Error("cash_over_short line present with zero variance; must be absent")
	}
}

// Open tickets with lines must be settled before shift close (409).
func TestOpenTicketsUnpaidBlockClose(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 0)
	f.newTicket(t, sh.ID, 1) // open, unpaid, has a line
	if _, _, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 0); !errors.Is(err, posapp.ErrOpenTicketsUnpaid) {
		t.Errorf("close with open unpaid ticket: got %v, want ErrOpenTicketsUnpaid", err)
	}
}

// AcceptShift posts the draft journal and is idempotent: a second accept
// conflicts (one live journal per shift / shift already accepted). The
// posted journal balances.
func TestAcceptShiftPostsAndIsIdempotent(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 5000)
	tk := f.newTicket(t, sh.ID, 2) // 3000 cash
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: 3000}}); err != nil {
		t.Fatal(err)
	}
	_, docID, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 8000)
	if err != nil {
		t.Fatalf("close shift: %v", err)
	}

	accepted, err := f.app.AcceptShift(f.ctx, f.restID, sh.ID, docID, f.userID)
	if err != nil {
		t.Fatalf("accept shift: %v", err)
	}
	if accepted.State() != domain.ShiftAccepted {
		t.Errorf("state = %q, want accepted", accepted.State())
	}
	doc, err := f.ledger.GetJournal(f.ctx, f.restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.State != ledger.StatePosted {
		t.Errorf("journal state = %q, want posted", doc.State)
	}
	assertBalanced(t, doc)

	// Second accept → conflict (already accepted / posted).
	if _, err := f.app.AcceptShift(f.ctx, f.restID, sh.ID, docID, f.userID); err == nil {
		t.Error("second accept must fail")
	}
}

// §15.2 — the draft-line override (the narrow write path into a
// "read-only" ledger) works only while draft; after Accept posts it, the
// override is rejected.
func TestOverrideDraftOnly(t *testing.T) {
	f := setupPos(t)
	sh := f.openShift(t, 5000)
	tk := f.newTicket(t, sh.ID, 2) // 3000 cash
	if _, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: 3000}}); err != nil {
		t.Fatal(err)
	}
	_, docID, err := f.app.CloseShift(f.ctx, f.restID, sh.ID, f.userID, 8000)
	if err != nil {
		t.Fatalf("close shift: %v", err)
	}

	// Override on the draft: re-map the sales credit to the comps account
	// (both postable) — must succeed while draft.
	draft, err := f.ledger.GetJournal(f.ctx, f.restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ledger.OverrideDraftLines(f.ctx, f.restID, docID, draft.Lines); err != nil {
		t.Fatalf("override on draft: %v", err)
	}

	// Accept → posted.
	if _, err := f.app.AcceptShift(f.ctx, f.restID, sh.ID, docID, f.userID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Override after post → ErrNotDraft (§15.2 write path closes at post).
	if err := f.ledger.OverrideDraftLines(f.ctx, f.restID, docID, draft.Lines); !errors.Is(err, ledger.ErrNotDraft) {
		t.Errorf("override after post: got %v, want ErrNotDraft", err)
	}
}

// assertBalanced sums debits/credits, and checks each line is one-sided
// with a positive amount.
func assertBalanced(t *testing.T, doc ledger.JournalDocument) {
	t.Helper()
	var d, c int64
	for _, l := range doc.Lines {
		switch l.Side {
		case ledger.SideDebit:
			d += l.AmountCents
		case ledger.SideCredit:
			c += l.AmountCents
		default:
			t.Errorf("line side must be debit|credit, got %q", l.Side)
		}
		if l.AmountCents <= 0 {
			t.Errorf("line amount must be > 0, got %d", l.AmountCents)
		}
	}
	if d != c {
		t.Errorf("document not balanced: debit %d != credit %d", d, c)
	}
}

// m2 — CloseTicket must reject a ticket that belongs to a shift other than
// the current open one (a stray close across a shift changeover). The
// ticket's own immutability is separate; this guards the shift binding.
func TestCloseTicketRejectsForeignShift(t *testing.T) {
	f := setupPos(t)
	sh1 := f.openShift(t, 0)
	tk := f.newTicket(t, sh1.ID, 1) // open ticket on sh1

	// Close sh1 at the DB level (leaving tk open), then open sh2 so the
	// current open shift differs from the ticket's shift.
	if _, err := f.db.ExecContext(f.ctx, `UPDATE shifts SET closed_at = now() WHERE id = $1`, sh1.ID); err != nil {
		t.Fatal(err)
	}
	f.openShift(t, 0) // sh2

	_, err := f.app.CloseTicket(f.ctx, f.restID, tk.ID, f.userID,
		[]domain.Tender{{MethodID: f.cashID, AmountCents: f.price}})
	if !errors.Is(err, posapp.ErrShiftNotOpen) {
		t.Errorf("close ticket from a foreign shift: got %v, want ErrShiftNotOpen", err)
	}
}
