package grpcserver

import (
	"context"
	"database/sql"
	"os"
	"testing"

	ledgerpg "aivo/internal/ledger/adapters/postgres"
	ledgerapp "aivo/internal/ledger/app"
	ledgerv1 "aivo/internal/ledger/v1"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// Integration tests for the ledger gRPC server's idempotency guarantee
// (service-events spec: a redelivered event is a no-op, not a duplicate
// post/reversal). Runs only with TEST_DATABASE_URL (or DATABASE_URL),
// against a fully migrated database; skipped otherwise.

func dsn() string {
	if d := os.Getenv("TEST_DATABASE_URL"); d != "" {
		return d
	}
	return os.Getenv("DATABASE_URL")
}

func setup(t *testing.T) (*Server, *ledgerapp.App, *sql.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	d := dsn()
	if d == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", d)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ledgerApp := ledgerapp.New(ledgerpg.NewStore(db))

	ctx := context.Background()
	orgID, restaurantID, userID := uuid.New(), uuid.New(), uuid.New()
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
		restaurantID, orgID, "t-"+uuid.New().String()[:8]); err != nil {
		t.Fatal(err)
	}
	if err := ledgerApp.SeedRestaurant(ctx, restaurantID); err != nil {
		t.Fatalf("seed restaurant: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM restaurants WHERE id = $1`, restaurantID)
		db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		db.ExecContext(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return New(ledgerApp), ledgerApp, db, restaurantID, userID
}

func TestPostCOGSJournalIdempotent(t *testing.T) {
	srv, ledgerApp, db, restaurantID, userID := setup(t)
	ctx := context.Background()

	req := &ledgerv1.PostCOGSJournalRequest{
		RestaurantId:   restaurantID.String(),
		CreatedBy:      userID.String(),
		TicketId:       uuid.New().String(),
		AccountingDate: "2026-01-15",
		Lines: []*ledgerv1.JournalLine{
			{Purpose: "cogs", Side: "debit", AmountCents: 500},
			{Purpose: "inventory", Side: "credit", AmountCents: 500},
		},
	}

	first, err := srv.PostCOGSJournal(ctx, req)
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if first.DocumentId == "" {
		t.Fatal("first delivery: empty document_id")
	}

	second, err := srv.PostCOGSJournal(ctx, req)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if second.DocumentId != first.DocumentId {
		t.Errorf("redelivery posted a new document: first %s, second %s", first.DocumentId, second.DocumentId)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM journal_documents WHERE restaurant_id = $1 AND source_kind = 'cogs' AND source_id = $2`,
		restaurantID, req.TicketId).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("journal_documents rows for ticket = %d, want 1 (one journal entry, not two)", count)
	}

	doc, err := ledgerApp.GetJournal(ctx, restaurantID, uuid.MustParse(first.DocumentId))
	if err != nil {
		t.Fatalf("get journal: %v", err)
	}
	if len(doc.Lines) != 2 {
		t.Errorf("posted document lines = %d, want 2", len(doc.Lines))
	}
}

func TestReverseCOGSJournalIdempotent(t *testing.T) {
	srv, _, db, restaurantID, userID := setup(t)
	ctx := context.Background()

	postReq := &ledgerv1.PostCOGSJournalRequest{
		RestaurantId:   restaurantID.String(),
		CreatedBy:      userID.String(),
		TicketId:       uuid.New().String(),
		AccountingDate: "2026-01-15",
		Lines: []*ledgerv1.JournalLine{
			{Purpose: "cogs", Side: "debit", AmountCents: 900},
			{Purpose: "inventory", Side: "credit", AmountCents: 900},
		},
	}
	if _, err := srv.PostCOGSJournal(ctx, postReq); err != nil {
		t.Fatalf("post: %v", err)
	}

	revReq := &ledgerv1.ReverseJournalRequest{RestaurantId: restaurantID.String(), SourceId: postReq.TicketId}
	first, err := srv.ReverseCOGSJournal(ctx, revReq)
	if err != nil {
		t.Fatalf("first reversal: %v", err)
	}
	if first.ReversalDocumentId == "" {
		t.Fatal("first reversal: empty reversal_document_id")
	}

	second, err := srv.ReverseCOGSJournal(ctx, revReq)
	if err != nil {
		t.Fatalf("redelivered reversal: %v", err)
	}
	if second.ReversalDocumentId != first.ReversalDocumentId {
		t.Errorf("redelivered reversal created a new document: first %s, second %s", first.ReversalDocumentId, second.ReversalDocumentId)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM journal_documents WHERE restaurant_id = $1 AND source_kind = 'cogs' AND source_id = $2 AND kind = 'reversal'`,
		restaurantID, postReq.TicketId).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("reversal documents for ticket = %d, want 1", count)
	}
}

// TestPostAndReverseEveryDocumentType exercises the remaining Post*/
// Reverse* RPC pairs (Receipt, WriteOff, Stocktake) — thin dispatch
// wrappers around the same postJournal/reverseJournal helpers already
// covered above for COGS.
func TestPostAndReverseEveryDocumentType(t *testing.T) {
	srv, _, _, restaurantID, userID := setup(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		post    func(sourceID string) (string, error)
		reverse func(sourceID string) (string, error)
	}{
		{
			name: "receipt",
			post: func(id string) (string, error) {
				resp, err := srv.PostReceiptJournal(ctx, &ledgerv1.PostReceiptJournalRequest{
					RestaurantId: restaurantID.String(), CreatedBy: userID.String(), ReceiptId: id, AccountingDate: "2026-01-15",
					Lines: []*ledgerv1.JournalLine{{Purpose: "inventory", Side: "debit", AmountCents: 100}, {Purpose: "accounts_payable", Side: "credit", AmountCents: 100}},
				})
				if err != nil {
					return "", err
				}
				return resp.DocumentId, nil
			},
			reverse: func(id string) (string, error) {
				resp, err := srv.ReverseReceiptJournal(ctx, &ledgerv1.ReverseJournalRequest{RestaurantId: restaurantID.String(), SourceId: id})
				if err != nil {
					return "", err
				}
				return resp.ReversalDocumentId, nil
			},
		},
		{
			name: "write_off",
			post: func(id string) (string, error) {
				resp, err := srv.PostWriteOffJournal(ctx, &ledgerv1.PostWriteOffJournalRequest{
					RestaurantId: restaurantID.String(), CreatedBy: userID.String(), WriteOffId: id, AccountingDate: "2026-01-15",
					Lines: []*ledgerv1.JournalLine{{Purpose: "inventory_shrinkage", Side: "debit", AmountCents: 100}, {Purpose: "inventory", Side: "credit", AmountCents: 100}},
				})
				if err != nil {
					return "", err
				}
				return resp.DocumentId, nil
			},
			reverse: func(id string) (string, error) {
				resp, err := srv.ReverseWriteOffJournal(ctx, &ledgerv1.ReverseJournalRequest{RestaurantId: restaurantID.String(), SourceId: id})
				if err != nil {
					return "", err
				}
				return resp.ReversalDocumentId, nil
			},
		},
		{
			name: "stocktake",
			post: func(id string) (string, error) {
				resp, err := srv.PostStocktakeJournal(ctx, &ledgerv1.PostStocktakeJournalRequest{
					RestaurantId: restaurantID.String(), CreatedBy: userID.String(), StocktakeId: id, AccountingDate: "2026-01-15",
					Lines: []*ledgerv1.JournalLine{{Purpose: "inventory", Side: "debit", AmountCents: 100}, {Purpose: "inventory_surplus", Side: "credit", AmountCents: 100}},
				})
				if err != nil {
					return "", err
				}
				return resp.DocumentId, nil
			},
			reverse: func(id string) (string, error) {
				resp, err := srv.ReverseStocktakeJournal(ctx, &ledgerv1.ReverseJournalRequest{RestaurantId: restaurantID.String(), SourceId: id})
				if err != nil {
					return "", err
				}
				return resp.ReversalDocumentId, nil
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sourceID := uuid.New().String()
			docID, err := c.post(sourceID)
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			if docID == "" {
				t.Fatal("post: empty document_id")
			}
			docID2, err := c.post(sourceID)
			if err != nil {
				t.Fatalf("redelivered post: %v", err)
			}
			if docID2 != docID {
				t.Errorf("redelivered post: got %s, want %s (same document)", docID2, docID)
			}

			revID, err := c.reverse(sourceID)
			if err != nil {
				t.Fatalf("reverse: %v", err)
			}
			if revID == "" {
				t.Fatal("reverse: empty reversal_document_id")
			}
			revID2, err := c.reverse(sourceID)
			if err != nil {
				t.Fatalf("redelivered reverse: %v", err)
			}
			if revID2 != revID {
				t.Errorf("redelivered reverse: got %s, want %s (same reversal)", revID2, revID)
			}
		})
	}
}

// TestPostJournalRejectsInvalidInput exercises postJournal's parse-error
// branches — reached identically by every Post*Journal RPC.
func TestPostJournalRejectsInvalidInput(t *testing.T) {
	srv, _, _, restaurantID, userID := setup(t)
	ctx := context.Background()

	valid := func() *ledgerv1.PostCOGSJournalRequest {
		return &ledgerv1.PostCOGSJournalRequest{
			RestaurantId: restaurantID.String(), CreatedBy: userID.String(), TicketId: uuid.New().String(), AccountingDate: "2026-01-15",
		}
	}

	cases := []struct {
		name string
		req  *ledgerv1.PostCOGSJournalRequest
	}{
		{"bad restaurant_id", func() *ledgerv1.PostCOGSJournalRequest { r := valid(); r.RestaurantId = "not-a-uuid"; return r }()},
		{"bad created_by", func() *ledgerv1.PostCOGSJournalRequest { r := valid(); r.CreatedBy = "not-a-uuid"; return r }()},
		{"bad ticket_id", func() *ledgerv1.PostCOGSJournalRequest { r := valid(); r.TicketId = "not-a-uuid"; return r }()},
		{"bad accounting_date", func() *ledgerv1.PostCOGSJournalRequest { r := valid(); r.AccountingDate = "15/01/2026"; return r }()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := srv.PostCOGSJournal(ctx, c.req); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

// TestReverseJournalRejectsInvalidInput exercises reverseJournal's
// parse-error branches — reached identically by every Reverse*Journal RPC.
func TestReverseJournalRejectsInvalidInput(t *testing.T) {
	srv, _, _, restaurantID, _ := setup(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  *ledgerv1.ReverseJournalRequest
	}{
		{"bad restaurant_id", &ledgerv1.ReverseJournalRequest{RestaurantId: "not-a-uuid", SourceId: uuid.New().String()}},
		{"bad source_id", &ledgerv1.ReverseJournalRequest{RestaurantId: restaurantID.String(), SourceId: "not-a-uuid"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := srv.ReverseCOGSJournal(ctx, c.req); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
