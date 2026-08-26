// Package grpcserver implements InventoryService.HandleTicketClosed
// (backend/proto/inventory/v1/inventory.proto), inventory's gRPC inbound
// for the pos→inventory TicketClosed event (specs/inventory-service,
// specs/service-events).
package grpcserver

import (
	"context"
	"fmt"
	"time"

	inventoryapp "aivo/internal/inventory/app"
	inventoryv1 "aivo/internal/inventory/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uuid"
)

// Server implements inventoryv1.InventoryServiceServer over an
// *inventoryapp.App.
type Server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	App *inventoryapp.App
}

func New(app *inventoryapp.App) *Server { return &Server{App: app} }

// HandleTicketClosed applies stock consumption for req's sold lines and
// publishes inventory's own outbox event for the resulting COGS posting,
// idempotently on req.TicketId (a redelivery of the same ticket is a
// no-op that still returns success — service-events "Delivery is
// idempotent on the consumer side").
func (s *Server) HandleTicketClosed(ctx context.Context, req *inventoryv1.HandleTicketClosedRequest) (*inventoryv1.HandleTicketClosedResponse, error) {
	restaurantID, err := uuid.Parse(req.GetRestaurantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid restaurant_id")
	}
	ticketID, err := uuid.Parse(req.GetTicketId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid ticket_id")
	}
	closedBy, err := uuid.Parse(req.GetClosedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid closed_by")
	}
	businessDate, err := time.Parse("2006-01-02", req.GetBusinessDate())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid business_date")
	}

	lines := make([]inventoryapp.SaleLine, len(req.GetLines()))
	for i, l := range req.GetLines() {
		menuItemID, err := uuid.Parse(l.GetMenuItemId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid lines[%d].menu_item_id", i))
		}
		ticketLineID, err := uuid.Parse(l.GetTicketLineId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid lines[%d].ticket_line_id", i))
		}
		lines[i] = inventoryapp.SaleLine{MenuItemID: menuItemID, Qty: int(l.GetQty()), TicketLineID: ticketLineID}
	}

	applied, err := s.App.HandleTicketClosed(ctx, restaurantID, closedBy, ticketID, businessDate, lines)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &inventoryv1.HandleTicketClosedResponse{Applied: applied}, nil
}
