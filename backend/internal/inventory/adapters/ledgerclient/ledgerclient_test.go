package ledgerclient

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	inventoryapp "aivo/internal/inventory/app"
	ledgerv1 "aivo/internal/ledger/v1"
	"aivo/internal/sharedkernel"
	"aivo/pkg/outbox"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"uuid"
)

// fakeLedger records every RPC it receives, standing in for
// cmd/aivo-server's real LedgerService (tasks.md 6.1, not yet built when
// this worktree started).
type fakeLedger struct {
	ledgerv1.UnimplementedLedgerServiceServer
	postCalls   []string // method names, in call order
	lastPost    *ledgerv1.PostCOGSJournalRequest
	cancelCalls []string
	failNext    bool
}

func (f *fakeLedger) PostCOGSJournal(ctx context.Context, req *ledgerv1.PostCOGSJournalRequest) (*ledgerv1.PostCOGSJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostCOGSJournal")
	f.lastPost = req
	return &ledgerv1.PostCOGSJournalResponse{DocId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostReceiptJournal(ctx context.Context, req *ledgerv1.PostReceiptJournalRequest) (*ledgerv1.PostReceiptJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostReceiptJournal")
	f.lastPost = &ledgerv1.PostCOGSJournalRequest{RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, SourceId: req.SourceId, AccountingDate: req.AccountingDate, Lines: req.Lines}
	return &ledgerv1.PostReceiptJournalResponse{DocId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostWriteOffJournal(ctx context.Context, req *ledgerv1.PostWriteOffJournalRequest) (*ledgerv1.PostWriteOffJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostWriteOffJournal")
	return &ledgerv1.PostWriteOffJournalResponse{DocId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostStocktakeJournal(ctx context.Context, req *ledgerv1.PostStocktakeJournalRequest) (*ledgerv1.PostStocktakeJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostStocktakeJournal")
	return &ledgerv1.PostStocktakeJournalResponse{DocId: uuid.New().String()}, nil
}
func (f *fakeLedger) CancelReceiptJournal(ctx context.Context, req *ledgerv1.CancelReceiptJournalRequest) (*ledgerv1.CancelReceiptJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "CancelReceiptJournal")
	return &ledgerv1.CancelReceiptJournalResponse{ReversalId: uuid.New().String()}, nil
}
func (f *fakeLedger) CancelWriteOffJournal(ctx context.Context, req *ledgerv1.CancelWriteOffJournalRequest) (*ledgerv1.CancelWriteOffJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "CancelWriteOffJournal")
	return &ledgerv1.CancelWriteOffJournalResponse{ReversalId: uuid.New().String()}, nil
}
func (f *fakeLedger) CancelStocktakeJournal(ctx context.Context, req *ledgerv1.CancelStocktakeJournalRequest) (*ledgerv1.CancelStocktakeJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "CancelStocktakeJournal")
	return &ledgerv1.CancelStocktakeJournalResponse{ReversalId: uuid.New().String()}, nil
}

func dialFake(t *testing.T, fake ledgerv1.LedgerServiceServer) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	ledgerv1.RegisterLedgerServiceServer(srv, fake)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func restaurantEvent(name string, payload any) outbox.PendingEvent {
	raw, _ := json.Marshal(payload)
	return outbox.PendingEvent{
		ID: sharedkernel.NewID(), Name: name, AggregateType: "test", AggregateID: sharedkernel.NewID(),
		RestaurantID: sharedkernel.NullID{V: sharedkernel.NewID(), Valid: true},
		Payload:      raw, OccurredAt: time.Now(),
	}
}

func TestDeliver_DispatchesPostEvents(t *testing.T) {
	fake := &fakeLedger{}
	d := New(dialFake(t, fake))

	payload := struct {
		CreatedBy      string `json:"created_by"`
		AccountingDate string `json:"accounting_date"`
		Lines          []struct {
			Purpose     string `json:"purpose"`
			Side        string `json:"side"`
			AmountCents int64  `json:"amount_cents"`
		} `json:"lines"`
	}{
		CreatedBy: uuid.New().String(), AccountingDate: "2026-01-01",
		Lines: []struct {
			Purpose     string `json:"purpose"`
			Side        string `json:"side"`
			AmountCents int64  `json:"amount_cents"`
		}{{Purpose: "cogs", Side: "debit", AmountCents: 100}},
	}

	names := []string{
		inventoryapp.EventCOGSPosted, inventoryapp.EventReceiptPosted,
		inventoryapp.EventWriteOffPosted, inventoryapp.EventStocktakePosted,
	}
	for _, name := range names {
		if err := d.Deliver(context.Background(), restaurantEvent(name, payload)); err != nil {
			t.Errorf("Deliver(%s) error = %v", name, err)
		}
	}
	wantCalls := []string{"PostCOGSJournal", "PostReceiptJournal", "PostWriteOffJournal", "PostStocktakeJournal"}
	if len(fake.postCalls) != len(wantCalls) {
		t.Fatalf("post calls = %v, want %v", fake.postCalls, wantCalls)
	}
	for i, w := range wantCalls {
		if fake.postCalls[i] != w {
			t.Errorf("call[%d] = %s, want %s", i, fake.postCalls[i], w)
		}
	}
	if fake.lastPost.AccountingDate != "2026-01-01" || len(fake.lastPost.Lines) != 1 || fake.lastPost.Lines[0].AmountCents != 100 {
		t.Errorf("last request = %+v, want decoded payload fields", fake.lastPost)
	}
}

func TestDeliver_DispatchesCancelEvents(t *testing.T) {
	fake := &fakeLedger{}
	d := New(dialFake(t, fake))

	names := []string{
		inventoryapp.EventReceiptCancelled, inventoryapp.EventWriteOffCancelled, inventoryapp.EventStocktakeCancelled,
	}
	for _, name := range names {
		if err := d.Deliver(context.Background(), restaurantEvent(name, struct{}{})); err != nil {
			t.Errorf("Deliver(%s) error = %v", name, err)
		}
	}
	want := []string{"CancelReceiptJournal", "CancelWriteOffJournal", "CancelStocktakeJournal"}
	if len(fake.cancelCalls) != len(want) {
		t.Fatalf("cancel calls = %v, want %v", fake.cancelCalls, want)
	}
}

func TestDeliver_UnknownEventName(t *testing.T) {
	d := New(dialFake(t, &fakeLedger{}))
	if err := d.Deliver(context.Background(), restaurantEvent("SomethingElse", struct{}{})); err == nil {
		t.Error("Deliver(unknown name) error = nil, want an error")
	}
}

func TestDeliver_BadPayloadJSON(t *testing.T) {
	d := New(dialFake(t, &fakeLedger{}))
	ev := restaurantEvent(inventoryapp.EventReceiptPosted, struct{}{})
	ev.Payload = []byte("not json")
	if err := d.Deliver(context.Background(), ev); err == nil {
		t.Error("Deliver(bad payload) error = nil, want an error")
	}
}
