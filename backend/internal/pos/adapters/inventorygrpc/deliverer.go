// Package inventorygrpc implements outbox.Deliverer for pos's outbox,
// delivering TicketClosed events to cmd/aivo-inventory's gRPC listener —
// the pos→inventory edge (openspec/changes/split-inventory-microservice,
// service-events spec).
package inventorygrpc

import (
	"context"
	"encoding/json"
	"fmt"

	inventoryv1 "aivo/internal/inventory/v1"
	"aivo/internal/pos/events"
	"aivo/pkg/outbox"
)

// Deliverer delivers pos's outbox events to inventory over gRPC.
type Deliverer struct {
	client inventoryv1.InventoryServiceClient
}

func New(client inventoryv1.InventoryServiceClient) *Deliverer { return &Deliverer{client: client} }

var _ outbox.Deliverer = (*Deliverer)(nil)

// Deliver decodes ev and calls inventory's HandleTicketClosed RPC. A
// non-nil error (network, decode, RPC) leaves the event unpublished for
// the Poller to retry — see pkg/outbox's Deliverer contract.
func (d *Deliverer) Deliver(ctx context.Context, ev outbox.PendingEvent) error {
	if ev.Name != events.TicketClosedName {
		return fmt.Errorf("inventorygrpc: unknown event %q", ev.Name)
	}
	var p events.TicketClosedPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("inventorygrpc: decode payload: %w", err)
	}
	lines := make([]*inventoryv1.SoldLine, len(p.Lines))
	for i, l := range p.Lines {
		lines[i] = &inventoryv1.SoldLine{MenuItemId: l.MenuItemID, Qty: int32(l.Qty), TicketLineId: l.TicketLineID}
	}
	_, err := d.client.HandleTicketClosed(ctx, &inventoryv1.TicketClosedRequest{
		RestaurantId: p.RestaurantID,
		TicketId:     p.TicketID,
		ClosedBy:     p.ClosedBy,
		BusinessDate: p.BusinessDate,
		Lines:        lines,
	})
	return err
}
