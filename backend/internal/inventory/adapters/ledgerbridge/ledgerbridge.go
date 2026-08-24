// Package ledgerbridge implements inventory ports.Ledger over the ledger
// app — the synchronous, in-process bridge from inventory to the GL
// context. Postings run inside the inventory transaction (shared *sql.Tx);
// correction is a reversal (append-only).
package ledgerbridge

import (
	"context"
	"database/sql"
	"time"

	"aivo/internal/inventory/ports"
	ledgerapp "aivo/internal/ledger/app"

	"uuid"
)

type Bridge struct {
	ledger *ledgerapp.App
}

var _ ports.Ledger = (*Bridge)(nil)

func New(ledger *ledgerapp.App) *Bridge { return &Bridge{ledger: ledger} }

func (b *Bridge) PostInventoryJournal(ctx context.Context, tx *sql.Tx, restaurantID, createdBy uuid.UUID, sourceKind string, sourceID uuid.UUID, accountingDate time.Time, lines []ports.JournalLine) (uuid.UUID, error) {
	glLines := make([]ledgerapp.InventoryJournalLine, len(lines))
	for i, l := range lines {
		glLines[i] = ledgerapp.InventoryJournalLine{Purpose: l.Purpose, Side: l.Side, AmountCents: l.AmountCents}
	}
	return b.ledger.PostInventoryJournal(ctx, tx, ledgerapp.InventoryJournalInput{
		RestaurantID:   restaurantID,
		CreatedBy:      createdBy,
		SourceKind:     sourceKind,
		SourceID:       sourceID,
		AccountingDate: accountingDate,
		Lines:          glLines,
	})
}

func (b *Bridge) CancelJournalForSource(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID, sourceKind string, sourceID uuid.UUID) (uuid.UUID, error) {
	return b.ledger.CancelJournalForSource(ctx, tx, restaurantID, sourceKind, sourceID)
}
