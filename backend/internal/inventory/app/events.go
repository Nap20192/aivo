package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"aivo/internal/inventory/ports"
	"aivo/internal/sharedkernel"
	"aivo/pkg/outbox"

	"uuid"
)

// Outbox event names for the inventory→ledger edge (service-events:
// "inventory publishes an event for every GL posting it needs from
// ledger"). One event per LedgerService RPC in
// backend/proto/ledger/v1/ledger.proto — inventory's outbox Deliverer
// maps a PendingEvent's Name to the matching typed RPC.
const (
	EventCOGSPosted         = "InventoryCOGSPosted"
	EventReceiptPosted      = "InventoryReceiptPosted"
	EventReceiptCancelled   = "InventoryReceiptCancelled"
	EventWriteOffPosted     = "InventoryWriteOffPosted"
	EventWriteOffCancelled  = "InventoryWriteOffCancelled"
	EventStocktakePosted    = "InventoryStocktakePosted"
	EventStocktakeCancelled = "InventoryStocktakeCancelled"
)

// postJournalPayload is the JSON shape of a Post*Journal outbox event —
// the fields a Deliverer needs to build a ledger.v1.PostJournalRequest
// (design.md D3: idempotency key is the event's AggregateID, the existing
// document id already in play, so it isn't repeated in the payload).
type postJournalPayload struct {
	CreatedBy      uuid.UUID           `json:"created_by"`
	AccountingDate string              `json:"accounting_date"`
	Lines          []ports.JournalLine `json:"lines"`
}

// publishPostJournal inserts a Post*Journal outbox event in tx — the same
// transaction as the stock movement it accompanies, so the business write
// and the event either both commit or both roll back (service-events:
// "Events are published in the same transaction as the write that causes
// them").
func (a *App) publishPostJournal(ctx context.Context, tx *sql.Tx, name, aggregateType string, restaurantID, createdBy, sourceID uuid.UUID, accountingDate time.Time, lines []ports.JournalLine) error {
	payload, err := json.Marshal(postJournalPayload{
		CreatedBy: createdBy, AccountingDate: accountingDate.Format("2006-01-02"), Lines: lines,
	})
	if err != nil {
		return err
	}
	return outbox.Publish(ctx, tx, outbox.Event{
		ID: a.newID(), Name: name, AggregateType: aggregateType, AggregateID: sourceID,
		RestaurantID: sharedkernel.NullID{V: restaurantID, Valid: true},
		Payload:      payload, OccurredAt: a.now(),
	})
}

// publishCancelJournal inserts a Cancel*Journal outbox event in tx. The
// event's AggregateID (the original document's id) is the only field a
// Deliverer needs to build a ledger.v1.CancelJournalRequest.
func (a *App) publishCancelJournal(ctx context.Context, tx *sql.Tx, name, aggregateType string, restaurantID, sourceID uuid.UUID) error {
	return outbox.Publish(ctx, tx, outbox.Event{
		ID: a.newID(), Name: name, AggregateType: aggregateType, AggregateID: sourceID,
		RestaurantID: sharedkernel.NullID{V: restaurantID, Valid: true},
		Payload:      json.RawMessage("{}"), OccurredAt: a.now(),
	})
}
