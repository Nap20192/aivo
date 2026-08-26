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
// cmd/aivo-server's real LedgerService (tasks.md 6.1).
type fakeLedger struct {
	ledgerv1.UnimplementedLedgerServiceServer
	postCalls   []string // method names, in call order
	lastPost    *ledgerv1.PostReceiptJournalRequest
	cancelCalls []string
	failNext    bool
}

func (f *fakeLedger) PostCOGSJournal(ctx context.Context, req *ledgerv1.PostCOGSJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostCOGSJournal")
	f.lastPost = &ledgerv1.PostReceiptJournalRequest{RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, ReceiptId: req.TicketId, AccountingDate: req.AccountingDate, Lines: req.Lines}
	return &ledgerv1.PostJournalResponse{DocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostReceiptJournal(ctx context.Context, req *ledgerv1.PostReceiptJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostReceiptJournal")
	f.lastPost = req
	return &ledgerv1.PostJournalResponse{DocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostWriteOffJournal(ctx context.Context, req *ledgerv1.PostWriteOffJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostWriteOffJournal")
	return &ledgerv1.PostJournalResponse{DocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) PostStocktakeJournal(ctx context.Context, req *ledgerv1.PostStocktakeJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	f.postCalls = append(f.postCalls, "PostStocktakeJournal")
	return &ledgerv1.PostJournalResponse{DocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) ReverseCOGSJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "ReverseCOGSJournal")
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) ReverseReceiptJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "ReverseReceiptJournal")
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) ReverseWriteOffJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "ReverseWriteOffJournal")
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: uuid.New().String()}, nil
}
func (f *fakeLedger) ReverseStocktakeJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	f.cancelCalls = append(f.cancelCalls, "ReverseStocktakeJournal")
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: uuid.New().String()}, nil
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
	want := []string{"ReverseReceiptJournal", "ReverseWriteOffJournal", "ReverseStocktakeJournal"}
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
