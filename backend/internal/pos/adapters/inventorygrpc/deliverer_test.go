package inventorygrpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/grpc"

	inventoryv1 "aivo/internal/inventory/v1"
	"aivo/internal/pos/events"
	"aivo/internal/sharedkernel"
	"aivo/pkg/outbox"
)

type fakeClient struct {
	req *inventoryv1.HandleTicketClosedRequest
	err error
}

func (f *fakeClient) HandleTicketClosed(_ context.Context, in *inventoryv1.HandleTicketClosedRequest, _ ...grpc.CallOption) (*inventoryv1.HandleTicketClosedResponse, error) {
	f.req = in
	if f.err != nil {
		return nil, f.err
	}
	return &inventoryv1.HandleTicketClosedResponse{Applied: true}, nil
}

func mustPayload(t *testing.T, p events.TicketClosedPayload) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDeliverMapsPayloadToRequest(t *testing.T) {
	fc := &fakeClient{}
	d := New(fc)

	payload := events.TicketClosedPayload{
		RestaurantID: "r1", TicketID: "t1", ClosedBy: "u1", BusinessDate: "2026-01-15",
		Lines: []events.TicketClosedLine{{MenuItemID: "m1", Qty: 2, TicketLineID: "l1"}},
	}
	ev := outbox.PendingEvent{Name: events.TicketClosedName, Payload: mustPayload(t, payload)}

	if err := d.Deliver(context.Background(), ev); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if fc.req.RestaurantId != "r1" || fc.req.TicketId != "t1" || fc.req.ClosedBy != "u1" || fc.req.BusinessDate != "2026-01-15" {
		t.Errorf("request = %+v, want fields matching payload", fc.req)
	}
	if len(fc.req.Lines) != 1 || fc.req.Lines[0].MenuItemId != "m1" || fc.req.Lines[0].Qty != 2 || fc.req.Lines[0].TicketLineId != "l1" {
		t.Errorf("request lines = %+v, want one line matching payload", fc.req.Lines)
	}
}

func TestDeliverPropagatesRPCError(t *testing.T) {
	wantErr := errors.New("inventory unreachable")
	fc := &fakeClient{err: wantErr}
	d := New(fc)

	ev := outbox.PendingEvent{Name: events.TicketClosedName, Payload: mustPayload(t, events.TicketClosedPayload{})}
	if err := d.Deliver(context.Background(), ev); !errors.Is(err, wantErr) {
		t.Errorf("Deliver error = %v, want %v", err, wantErr)
	}
}

func TestDeliverRejectsUnknownEvent(t *testing.T) {
	d := New(&fakeClient{})
	ev := outbox.PendingEvent{ID: sharedkernel.NewID(), Name: "SomethingElse", Payload: []byte("{}")}
	if err := d.Deliver(context.Background(), ev); err == nil {
		t.Error("Deliver: want error for unknown event name, got nil")
	}
}

func TestDeliverRejectsBadPayload(t *testing.T) {
	d := New(&fakeClient{})
	ev := outbox.PendingEvent{Name: events.TicketClosedName, Payload: []byte("not json")}
	if err := d.Deliver(context.Background(), ev); err == nil {
		t.Error("Deliver: want error for undecodable payload, got nil")
	}
}
