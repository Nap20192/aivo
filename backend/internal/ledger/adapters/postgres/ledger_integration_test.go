package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	ledgerapp "aivo/internal/ledger/app"
	ledger "aivo/internal/domain/ledger"
	"aivo/internal/ledger/ports"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// Integration tests for the ledger (GL) context against Postgres. Runs
// only with TEST_DATABASE_URL (or DATABASE_URL) pointing at a migrated
// database (the compose dev one); skipped otherwise so the suite stays
// green without infra. Each test provisions a throwaway restaurant
// (chart of accounts + map + cost center via SeedRestaurant) and cleans
// it up. Covers the refuted-assumption test cases (reference §15) and the
// GL invariants (contract §5 "Tester") that unit tests can't reach:
// balance/auto-unassigned persistence, append-only posted facts, the
// storno path, two dates, the configurable tender map, tenant isolation,
// and the one-live-document-per-shift partial unique.

func ledgerDSN() string {
	if d := os.Getenv("TEST_DATABASE_URL"); d != "" {
		return d
	}
	return os.Getenv("DATABASE_URL")
}

func setupLedger(t *testing.T) (context.Context, *sql.DB, *ledgerapp.App) {
	t.Helper()
	dsn := ledgerDSN()
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
	return context.Background(), db, ledgerapp.New(NewStore(db))
}

// seedRestaurant provisions org + user + restaurant + the ledger seed,
// returning the restaurant and a user id usable as created_by (FK).
func seedRestaurant(t *testing.T, ctx context.Context, db *sql.DB, app *ledgerapp.App) (restID, userID uuid.UUID) {
	t.Helper()
	orgID := uuid.New()
	restID = uuid.New()
	userID = uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO organizations (id, name) VALUES ($1, 'test-org')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, org_id, email, password_hash, role) VALUES ($1, $2, $3, $4, 'owner')`,
		userID, orgID, "u-"+uuid.New().String()[:8]+"@test", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, 'Test')`,
		restID, orgID, "t-"+uuid.New().String()[:8]); err != nil {
		t.Fatal(err)
	}
	if err := app.SeedRestaurant(ctx, restID); err != nil {
		t.Fatalf("seed restaurant: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM restaurants WHERE id = $1`, restID)
		db.ExecContext(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return restID, userID
}

// buildShiftJournal runs BuildDraftShiftJournal in a committed tx and
// returns the draft document id.
func buildShiftJournal(t *testing.T, ctx context.Context, db *sql.DB, app *ledgerapp.App, in ledgerapp.ShiftJournalInput) uuid.UUID {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	docID, err := app.BuildDraftShiftJournal(ctx, tx, in)
	if err != nil {
		tx.Rollback()
		t.Fatalf("build draft shift journal: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return docID
}

func lineOn(doc ledger.JournalDocument, accountID uuid.UUID) (ledger.JournalLine, bool) {
	for _, l := range doc.Lines {
		if l.AccountID == accountID {
			return l, true
		}
	}
	return ledger.JournalLine{}, false
}

// balanced sums debits and credits and asserts equality.
func assertBalanced(t *testing.T, doc ledger.JournalDocument) {
	t.Helper()
	var d, c int64
	for _, l := range doc.Lines {
		if l.Side == ledger.SideDebit {
			d += l.AmountCents
		} else {
			c += l.AmountCents
		}
		if l.AmountCents <= 0 {
			t.Errorf("line amount must be > 0, got %d on account %s", l.AmountCents, l.AccountID)
		}
		if l.Side != ledger.SideDebit && l.Side != ledger.SideCredit {
			t.Errorf("line side must be debit|credit, got %q", l.Side)
		}
	}
	if d != c {
		t.Errorf("document not balanced: debit %d != credit %d", d, c)
	}
}

func acct(t *testing.T, ctx context.Context, app *ledgerapp.App, restID uuid.UUID, purpose string) uuid.UUID {
	t.Helper()
	id, err := app.AccountForPurpose(ctx, restID, purpose)
	if err != nil {
		t.Fatalf("account for purpose %q: %v", purpose, err)
	}
	return id
}

// §15.5 — variance is a mandatory posting line, plus balance + the cash
// drawer being adjusted by the variance (contract §6 worked example).
func TestShiftJournalVarianceIsAPosting(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restID, userID := seedRestaurant(t, ctx, db, app)

	// cash 10000, card 5000, variance -200 (shortage).
	docID := buildShiftJournal(t, ctx, db, app, ledgerapp.ShiftJournalInput{
		RestaurantID:   restID,
		ShiftID:        uuid.New(),
		CreatedBy:      userID,
		AccountingDate: time.Now().UTC(),
		Tenders: []ledgerapp.TenderTotal{
			{Group: "cash", AmountCents: 10000},
			{Group: "card", AmountCents: 5000},
		},
		VarianceCents: -200,
	})
	doc, err := app.GetJournal(ctx, restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, doc)

	cashID := acct(t, ctx, app, restID, "tender:cash")
	cardID := acct(t, ctx, app, restID, "tender:card")
	osID := acct(t, ctx, app, restID, "cash_over_short")
	salesID := acct(t, ctx, app, restID, "sales_revenue")

	// cash drawer debit = 10000 + (-200) = 9800.
	if l, ok := lineOn(doc, cashID); !ok || l.Side != ledger.SideDebit || l.AmountCents != 9800 {
		t.Errorf("cash line: got %+v ok=%v, want debit 9800", l, ok)
	}
	if l, ok := lineOn(doc, cardID); !ok || l.Side != ledger.SideDebit || l.AmountCents != 5000 {
		t.Errorf("card line: got %+v ok=%v, want debit 5000", l, ok)
	}
	// Mandatory cash_over_short line, debit 200 (shortage).
	if l, ok := lineOn(doc, osID); !ok || l.Side != ledger.SideDebit || l.AmountCents != 200 {
		t.Errorf("cash_over_short line: got %+v ok=%v, want debit 200", l, ok)
	}
	if l, ok := lineOn(doc, salesID); !ok || l.Side != ledger.SideCredit || l.AmountCents != 15000 {
		t.Errorf("sales line: got %+v ok=%v, want credit 15000", l, ok)
	}
}

// §15.5 negative case — with declared == expected (variance 0) there is
// no cash_over_short line at all.
func TestShiftJournalNoVarianceNoOverShortLine(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restID, userID := seedRestaurant(t, ctx, db, app)

	docID := buildShiftJournal(t, ctx, db, app, ledgerapp.ShiftJournalInput{
		RestaurantID:   restID,
		ShiftID:        uuid.New(),
		CreatedBy:      userID,
		AccountingDate: time.Now().UTC(),
		Tenders:        []ledgerapp.TenderTotal{{Group: "cash", AmountCents: 8000}},
		VarianceCents:  0,
	})
	doc, err := app.GetJournal(ctx, restID, docID)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, doc)
	osID := acct(t, ctx, app, restID, "cash_over_short")
	if _, ok := lineOn(doc, osID); ok {
		t.Error("cash_over_short line present with zero variance; must be absent")
	}
}

// §15.1 + §15.3 — a posted fact is corrected only by a storno: the
// reversal is revalidated at the current date, the original is marked
// cancelled but its lines are untouched, and no path re-posts or re-edits
// the posted document (append-only).
func TestReversalIsAppendOnly(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restID, userID := seedRestaurant(t, ctx, db, app)

	cashID := acct(t, ctx, app, restID, "tender:cash")
	salesID := acct(t, ctx, app, restID, "sales_revenue")

	// A backdated manual journal (accounting_date = 30 days ago): two
	// distinct dates on the document (D7).
	backdate := time.Now().UTC().AddDate(0, 0, -30)
	orig, err := app.ManualJournal(ctx, ledgerapp.ManualJournalInput{
		RestaurantID:   restID,
		CreatedBy:      userID,
		AccountingDate: backdate,
		Memo:           "backdated sale",
		Lines: []ledgerapp.ManualLine{
			{AccountID: cashID, Side: ledger.SideDebit, AmountCents: 5000, Memo: "cash"},
			{AccountID: salesID, Side: ledger.SideCredit, AmountCents: 5000, Memo: "sales"},
		},
	}, true)
	if err != nil {
		t.Fatalf("manual journal: %v", err)
	}
	if orig.State != ledger.StatePosted {
		t.Fatalf("original state = %q, want posted", orig.State)
	}
	// Two dates present and distinct in date (D7).
	if orig.AccountingDate.Format("2006-01-02") == orig.RecordedAt.Format("2006-01-02") {
		t.Errorf("accounting_date and recorded_at should differ for a backdated doc: %v vs %v",
			orig.AccountingDate, orig.RecordedAt)
	}

	// Cancel → reversal.
	revID, err := app.CancelJournal(ctx, restID, orig.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	rev, err := app.GetJournal(ctx, restID, revID)
	if err != nil {
		t.Fatal(err)
	}
	// §15.1 — reversal revalidated at the current date, not the original's.
	today := time.Now().UTC().Format("2006-01-02")
	if got := rev.AccountingDate.Format("2006-01-02"); got != today {
		t.Errorf("reversal accounting_date = %s, want today %s (§15.1)", got, today)
	}
	if rev.Kind != ledger.KindReversal || rev.ReversalOf == nil || *rev.ReversalOf != orig.ID {
		t.Errorf("reversal linkage wrong: kind=%q reversal_of=%v", rev.Kind, rev.ReversalOf)
	}
	assertBalanced(t, rev)
	// Mirrored lines: cash was debit 5000 in original, credit 5000 here.
	if l, ok := lineOn(rev, cashID); !ok || l.Side != ledger.SideCredit || l.AmountCents != 5000 {
		t.Errorf("reversal cash line: got %+v ok=%v, want credit 5000", l, ok)
	}

	// Original: state cancelled, lines UNCHANGED (append-only — never
	// mutated beyond state/cancelled_at).
	reRead, err := app.GetJournal(ctx, restID, orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reRead.State != ledger.StateCancelled {
		t.Errorf("original state = %q, want cancelled", reRead.State)
	}
	if reRead.AccountingDate.Format("2006-01-02") != backdate.Format("2006-01-02") {
		t.Errorf("original accounting_date changed: %v, want %v", reRead.AccountingDate, backdate)
	}
	if len(reRead.Lines) != len(orig.Lines) {
		t.Errorf("original line count changed: %d, want %d", len(reRead.Lines), len(orig.Lines))
	}
	if l, ok := lineOn(reRead, cashID); !ok || l.Side != ledger.SideDebit || l.AmountCents != 5000 {
		t.Errorf("original cash line mutated: got %+v ok=%v, want debit 5000", l, ok)
	}

	// §15.3 — no path re-posts / re-edits the posted (now cancelled) fact.
	// A second cancel must fail (already cancelled, not posted).
	if _, err := app.CancelJournal(ctx, restID, orig.ID); err == nil {
		t.Error("second cancel of an already-cancelled document must fail")
	}
	// Override (the draft-only write path) rejected on a non-draft doc.
	if err := app.OverrideDraftLines(ctx, restID, orig.ID, reRead.Lines); !errors.Is(err, ledger.ErrNotDraft) {
		t.Errorf("override of cancelled document: got %v, want ErrNotDraft", err)
	}
}

// Manual journals: an imbalance is an error (no auto-balance), a balanced
// one posts. Contract §4 — manual disbalance = 422, not auto-filled.
func TestManualJournalBalanceRule(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restID, userID := seedRestaurant(t, ctx, db, app)

	cashID := acct(t, ctx, app, restID, "tender:cash")
	salesID := acct(t, ctx, app, restID, "sales_revenue")

	// Unbalanced → ErrUnbalanced.
	_, err := app.ManualJournal(ctx, ledgerapp.ManualJournalInput{
		RestaurantID: restID, CreatedBy: userID, AccountingDate: time.Now().UTC(),
		Lines: []ledgerapp.ManualLine{
			{AccountID: cashID, Side: ledger.SideDebit, AmountCents: 5000},
			{AccountID: salesID, Side: ledger.SideCredit, AmountCents: 4000},
		},
	}, true)
	if !errors.Is(err, ledger.ErrUnbalanced) {
		t.Errorf("unbalanced manual journal: got %v, want ErrUnbalanced", err)
	}

	// Balanced + post → persisted posted.
	doc, err := app.ManualJournal(ctx, ledgerapp.ManualJournalInput{
		RestaurantID: restID, CreatedBy: userID, AccountingDate: time.Now().UTC(),
		Lines: []ledgerapp.ManualLine{
			{AccountID: cashID, Side: ledger.SideDebit, AmountCents: 5000},
			{AccountID: salesID, Side: ledger.SideCredit, AmountCents: 5000},
		},
	}, true)
	if err != nil {
		t.Fatalf("balanced manual journal: %v", err)
	}
	reRead, err := app.GetJournal(ctx, restID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reRead.State != ledger.StatePosted {
		t.Errorf("state = %q, want posted", reRead.State)
	}
	assertBalanced(t, reRead)
}

// §15.6 — GL treatment of a tender is per-restaurant config, not a fixed
// property: remap tender:cash to a different account and the same cash
// lands there; two restaurants with different maps route cash differently.
func TestTenderMapIsConfigurable(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restA, userA := seedRestaurant(t, ctx, db, app)

	defaultCash := acct(t, ctx, app, restA, "tender:cash")

	// Find the "1020" undeposited-funds account and remap tender:cash to it.
	accounts, err := app.Accounts(ctx, restA)
	if err != nil {
		t.Fatal(err)
	}
	var newCash uuid.UUID
	var found bool
	for _, a := range accounts {
		if a.Code == "1020" {
			newCash, found = a.ID, true
		}
	}
	if !found {
		t.Fatal("account 1020 not seeded")
	}
	if newCash == defaultCash {
		t.Fatal("precondition: 1020 should differ from default cash 1000")
	}
	if _, err := app.AccountMapPut(ctx, restA, []ledgerapp.MapEntry{{Purpose: "tender:cash", AccountID: newCash}}); err != nil {
		t.Fatalf("remap tender:cash: %v", err)
	}

	docID := buildShiftJournal(t, ctx, db, app, ledgerapp.ShiftJournalInput{
		RestaurantID: restA, ShiftID: uuid.New(), CreatedBy: userA,
		AccountingDate: time.Now().UTC(),
		Tenders:        []ledgerapp.TenderTotal{{Group: "cash", AmountCents: 7000}},
	})
	doc, err := app.GetJournal(ctx, restA, docID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lineOn(doc, newCash); !ok {
		t.Error("cash did not post to the remapped account 1020")
	}
	if _, ok := lineOn(doc, defaultCash); ok {
		t.Error("cash still posted to the old account 1000 after remap")
	}

	// A second restaurant keeps its default mapping → cash on 1000.
	restB, userB := seedRestaurant(t, ctx, db, app)
	bCash := acct(t, ctx, app, restB, "tender:cash")
	docBID := buildShiftJournal(t, ctx, db, app, ledgerapp.ShiftJournalInput{
		RestaurantID: restB, ShiftID: uuid.New(), CreatedBy: userB,
		AccountingDate: time.Now().UTC(),
		Tenders:        []ledgerapp.TenderTotal{{Group: "cash", AmountCents: 7000}},
	})
	docB, err := app.GetJournal(ctx, restB, docBID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lineOn(docB, bCash); !ok {
		t.Error("restaurant B cash did not post to its default account")
	}
	if bCash == newCash {
		t.Error("precondition: the two restaurants should have independent maps")
	}
}

// Tenant isolation: restaurant B cannot read A's document, accounts, or
// account map (every ledger query is restaurant-scoped).
func TestLedgerTenantIsolation(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restA, userA := seedRestaurant(t, ctx, db, app)
	restB, _ := seedRestaurant(t, ctx, db, app)

	docID := buildShiftJournal(t, ctx, db, app, ledgerapp.ShiftJournalInput{
		RestaurantID: restA, ShiftID: uuid.New(), CreatedBy: userA,
		AccountingDate: time.Now().UTC(),
		Tenders:        []ledgerapp.TenderTotal{{Group: "cash", AmountCents: 5000}},
	})
	// A's document is invisible to B.
	if _, err := app.GetJournal(ctx, restB, docID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("cross-tenant GetJournal: got %v, want ErrNotFound", err)
	}
	// A's account id is not resolvable as one of B's accounts.
	aCash := acct(t, ctx, app, restA, "tender:cash")
	store := NewStore(db)
	if _, err := store.AccountByID(ctx, restB, aCash); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("cross-tenant AccountByID: got %v, want ErrNotFound", err)
	}
}

// The "one live document per shift" partial unique guarantees idempotent
// acceptance: a shift posts at most one live journal. A second build for
// the same shift conflicts; after the first is cancelled, a build is
// allowed again (§15.2 idempotency contract).
func TestOneLiveDocumentPerShift(t *testing.T) {
	ctx, db, app := setupLedger(t)
	restID, userID := seedRestaurant(t, ctx, db, app)
	shiftID := uuid.New()

	in := ledgerapp.ShiftJournalInput{
		RestaurantID: restID, ShiftID: shiftID, CreatedBy: userID,
		AccountingDate: time.Now().UTC(),
		Tenders:        []ledgerapp.TenderTotal{{Group: "cash", AmountCents: 5000}},
	}
	first := buildShiftJournal(t, ctx, db, app, in)

	// Second build for the same shift → conflict (partial unique).
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.BuildDraftShiftJournal(ctx, tx, in)
	tx.Rollback()
	if !errors.Is(err, ports.ErrConflict) {
		t.Errorf("second live journal for shift: got %v, want ErrConflict", err)
	}

	// LiveDocumentForShift resolves the single live draft.
	if live, err := app.LiveDocumentForShift(ctx, restID, shiftID); err != nil || live.ID != first {
		t.Errorf("live document for shift: got %v (err %v), want %v", live.ID, err, first)
	}

	// Post then cancel the first. The reversal RETAINS the shift source
	// linkage and is itself live (posted), so it becomes the single live
	// document — a rebuild stays blocked (append-only linkage; a shift
	// whose acceptance journal was reversed is not re-buildable in
	// increment-1). See E2E report note.
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.PostShiftJournal(ctx, tx2, restID, first); err != nil {
		tx2.Rollback()
		t.Fatalf("post shift journal: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	revID, err := app.CancelJournal(ctx, restID, first)
	if err != nil {
		t.Fatalf("cancel first: %v", err)
	}
	live, err := app.LiveDocumentForShift(ctx, restID, shiftID)
	if err != nil {
		t.Fatalf("live document after cancel: %v", err)
	}
	if live.ID != revID {
		t.Errorf("live document after cancel = %v, want the reversal %v", live.ID, revID)
	}
	tx3, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.BuildDraftShiftJournal(ctx, tx3, in)
	tx3.Rollback()
	if !errors.Is(err, ports.ErrConflict) {
		t.Errorf("rebuild after reversal: got %v, want ErrConflict (reversal holds the live slot)", err)
	}
}
