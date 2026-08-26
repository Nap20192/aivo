package grpcserver

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	inventorypg "aivo/internal/inventory/adapters/postgres"
	inventoryapp "aivo/internal/inventory/app"
	inventoryv1 "aivo/internal/inventory/v1"
	"aivo/internal/pos/adapters/salesreader"
	"aivo/migrations"
	"aivo/pkg/migrate"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uuid"
)

// setup mirrors the postgres package's own integration-test fixture: a
// real App against inventory's schema, skipped without DATABASE_URL.
func setup(t *testing.T) (*Server, *sql.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := inventorypg.OpenSchemaDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := inventorypg.EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Apply(ctx, db, []migrate.Source{{Name: "inventory", FS: migrations.FS, Dir: "inventory"}}); err != nil {
		t.Fatal(err)
	}

	orgID, restID, userID := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'grpc-test-org')`, orgID)
	exec(`INSERT INTO users (id, org_id, email, password_hash, role) VALUES ($1, $2, $3, $4, 'owner')`,
		userID, orgID, "u-"+uuid.New().String()[:8]+"@t", []byte("x"))
	exec(`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, 'T')`, restID, orgID, "t-"+uuid.New().String()[:8])
	t.Cleanup(func() {
		bg := context.Background()
		db.ExecContext(bg, `DELETE FROM restaurants WHERE id = $1`, restID)
		db.ExecContext(bg, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	app := inventoryapp.New(inventorypg.NewStore(db), salesreader.New(db))
	return New(app), db, restID, userID
}

func TestHandleTicketClosed_ValidatesInput(t *testing.T) {
	srv, _, restID, userID := setup(t)
	valid := &inventoryv1.HandleTicketClosedRequest{
		RestaurantId: restID.String(), TicketId: uuid.New().String(),
		ClosedBy: userID.String(), BusinessDate: "2026-01-01",
	}

	cases := []struct {
		name string
		req  *inventoryv1.HandleTicketClosedRequest
	}{
		{"bad restaurant_id", &inventoryv1.HandleTicketClosedRequest{RestaurantId: "nope", TicketId: valid.TicketId, ClosedBy: valid.ClosedBy, BusinessDate: valid.BusinessDate}},
		{"bad ticket_id", &inventoryv1.HandleTicketClosedRequest{RestaurantId: valid.RestaurantId, TicketId: "nope", ClosedBy: valid.ClosedBy, BusinessDate: valid.BusinessDate}},
		{"bad closed_by", &inventoryv1.HandleTicketClosedRequest{RestaurantId: valid.RestaurantId, TicketId: valid.TicketId, ClosedBy: "nope", BusinessDate: valid.BusinessDate}},
		{"bad business_date", &inventoryv1.HandleTicketClosedRequest{RestaurantId: valid.RestaurantId, TicketId: valid.TicketId, ClosedBy: valid.ClosedBy, BusinessDate: "not-a-date"}},
		{"bad line menu_item_id", &inventoryv1.HandleTicketClosedRequest{RestaurantId: valid.RestaurantId, TicketId: valid.TicketId, ClosedBy: valid.ClosedBy, BusinessDate: valid.BusinessDate,
			Lines: []*inventoryv1.SaleLine{{MenuItemId: "nope", Qty: 1, TicketLineId: uuid.New().String()}}}},
		{"bad line ticket_line_id", &inventoryv1.HandleTicketClosedRequest{RestaurantId: valid.RestaurantId, TicketId: valid.TicketId, ClosedBy: valid.ClosedBy, BusinessDate: valid.BusinessDate,
			Lines: []*inventoryv1.SaleLine{{MenuItemId: uuid.New().String(), Qty: 1, TicketLineId: "nope"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := srv.HandleTicketClosed(context.Background(), c.req)
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.InvalidArgument {
				t.Errorf("%s: err = %v, want codes.InvalidArgument", c.name, err)
			}
		})
	}
}

func TestHandleTicketClosed_Idempotent(t *testing.T) {
	srv, _, restID, userID := setup(t)
	ticketID := uuid.New()
	// No product/tech card is set up, so every line is skipped — applied
	// still reflects whether anything NEW happened, which is false both
	// times here; this test's point is that the SAME ticket_id can be
	// delivered twice without erroring (idempotent no-op path), matching
	// the fuller idempotent-with-real-stock-movement coverage in
	// internal/inventory/adapters/postgres's integration tests.
	req := &inventoryv1.HandleTicketClosedRequest{
		RestaurantId: restID.String(), TicketId: ticketID.String(), ClosedBy: userID.String(),
		BusinessDate: time.Now().Format("2006-01-02"),
		Lines:        []*inventoryv1.SaleLine{{MenuItemId: uuid.New().String(), Qty: 1, TicketLineId: uuid.New().String()}},
	}
	resp1, err := srv.HandleTicketClosed(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := srv.HandleTicketClosed(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp1.Applied || resp2.Applied {
		t.Errorf("no tracked product: applied = %v/%v, want false/false", resp1.Applied, resp2.Applied)
	}
}
