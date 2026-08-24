// Package inventorybridge implements pos ports.Inventory over the inventory
// app — the synchronous pos→inventory bridge (§7). At ticket close it runs
// inside the pos transaction (shared *sql.Tx).
package inventorybridge

import (
	"context"
	"database/sql"
	"time"

	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/pos/ports"

	"uuid"
)

type Bridge struct {
	inventory *inventoryapp.App
}

var _ ports.Inventory = (*Bridge)(nil)

func New(inventory *inventoryapp.App) *Bridge { return &Bridge{inventory: inventory} }

func (b *Bridge) ConsumeForSale(ctx context.Context, tx *sql.Tx, restaurantID, closedBy, ticketID uuid.UUID, businessDate time.Time, lines []ports.SaleLine) (int64, error) {
	sl := make([]inventoryapp.SaleLine, len(lines))
	for i, l := range lines {
		sl[i] = inventoryapp.SaleLine{MenuItemID: l.MenuItemID, Qty: l.Qty, TicketLineID: l.TicketLineID}
	}
	return b.inventory.ConsumeForSale(ctx, tx, restaurantID, closedBy, ticketID, businessDate, sl)
}
