// Package events holds the wire payload shapes of pos's outbox events —
// owned by pos (the producer), imported by cmd/aivo-server's outbox
// Deliverer to build the gRPC request it sends to inventory.
package events

// TicketClosedName is the outbox event name for a closed ticket
// (backend/proto/inventory/v1/inventory.proto's HandleTicketClosed RPC).
const TicketClosedName = "TicketClosed"

// TicketClosedPayload is TicketClosed's JSON payload, mirroring
// TicketClosedRequest in inventory.proto field for field.
type TicketClosedPayload struct {
	RestaurantID string             `json:"restaurant_id"`
	TicketID     string             `json:"ticket_id"`
	ClosedBy     string             `json:"closed_by"`
	BusinessDate string             `json:"business_date"` // YYYY-MM-DD
	Lines        []TicketClosedLine `json:"lines"`
}

// TicketClosedLine is one sold ticket line.
type TicketClosedLine struct {
	MenuItemID   string `json:"menu_item_id"`
	Qty          int    `json:"qty"`
	TicketLineID string `json:"ticket_line_id"`
}
