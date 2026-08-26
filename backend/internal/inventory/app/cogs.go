package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// SaleLine is one sold ticket line handed to ConsumeForSale.
type SaleLine struct {
	MenuItemID   uuid.UUID
	Qty          int
	TicketLineID uuid.UUID // for deterministic COGS idempotency
}

// ConsumeForSale depletes stock for a ticket's sold lines and posts the
// COGS GL document, on the caller's (pos) transaction — atomic with the
// ticket close (§7). A line whose menu item has no tracked dish or no
// active tech card is skipped (a sale is never blocked). Returns Σ cost
// consumed. Idempotent: each sale move carries a deterministic
// source_event_id, so a re-run cannot double-deplete.
func (a *App) ConsumeForSale(ctx context.Context, tx *sql.Tx, restaurantID, closedBy, ticketID uuid.UUID, businessDate time.Time, lines []SaleLine) (int64, error) {
	st := a.store.WithTx(tx)
	var total int64
	for _, sl := range lines {
		if sl.Qty <= 0 {
			continue
		}
		product, err := st.ProductByMenuItem(ctx, restaurantID, sl.MenuItemID)
		if errors.Is(err, ports.ErrNotFound) {
			continue // untracked / retail item
		}
		if err != nil {
			return 0, err
		}
		card, err := st.ActiveTechCard(ctx, restaurantID, product.ID, businessDate)
		if errors.Is(err, ports.ErrNotFound) {
			continue // no recipe on the sale date
		}
		if err != nil {
			return 0, err
		}
		qty := int64(sl.Qty)

		if card.Consumption == inv.ConsumeDepleteFinished {
			cost, err := a.consumeOne(ctx, st, restaurantID, product.ID, qty*inv.MilliPerUnit, businessDate, sl.TicketLineID)
			if err != nil {
				return 0, err
			}
			total += cost
			continue
		}
		// assemble: deplete each ingredient (one level).
		for _, l := range card.Lines {
			cost, err := a.consumeOne(ctx, st, restaurantID, l.IngredientProductID, l.Qty*qty, businessDate, sl.TicketLineID)
			if err != nil {
				return 0, err
			}
			total += cost
		}
	}
	if total != 0 {
		// One COGS outbox event per ticket close: debit COGS / credit
		// inventory, posted asynchronously by ledger once it consumes the
		// event (service-events: inventory→ledger edge).
		if err := a.publishPostJournal(ctx, tx, EventCOGSPosted, "ticket", restaurantID, closedBy, ticketID, businessDate, []ports.JournalLine{
			{Purpose: "cogs", Side: "debit", AmountCents: total},
			{Purpose: "inventory", Side: "credit", AmountCents: total},
		}); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// HandleTicketClosed is the gRPC-facing entry point for the pos→inventory
// TicketClosed event (specs/inventory-service, specs/service-events):
// unlike ConsumeForSale (called inside the caller's transaction, back when
// pos and inventory shared one process), this owns its own transaction,
// since pos and inventory no longer share one. Idempotent: redelivering
// ticketID applies nothing new and reports applied=false.
func (a *App) HandleTicketClosed(ctx context.Context, restaurantID, closedBy, ticketID uuid.UUID, businessDate time.Time, lines []SaleLine) (applied bool, err error) {
	err = a.store.InTx(ctx, func(tx *sql.Tx, _ ports.Store) error {
		total, cerr := a.ConsumeForSale(ctx, tx, restaurantID, closedBy, ticketID, businessDate, lines)
		if cerr != nil {
			return cerr
		}
		applied = total != 0
		return nil
	})
	return applied, err
}

// consumeOne issues one sale move for a product, guarded for idempotency by
// a deterministic source_event_id (ticket line + product).
func (a *App) consumeOne(ctx context.Context, st ports.Store, restaurantID, productID uuid.UUID, qtyMilli int64, businessDate time.Time, ticketLineID uuid.UUID) (int64, error) {
	if qtyMilli <= 0 {
		return 0, nil
	}
	eventID := deriveEventID(ticketLineID, productID)
	exists, err := st.MoveExistsBySourceEvent(ctx, eventID)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, nil // already consumed (idempotent re-run)
	}
	cost, _, err := a.applyIssueMove(ctx, st, restaurantID, productID, inv.MoveSale, qtyMilli, businessDate, inv.SourceCOGS, ticketLineID, &eventID)
	return cost, err
}
