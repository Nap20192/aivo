// Package ledgerbridge implements pos ports.Ledger over the ledger app —
// the synchronous, in-process bridge from the pos context to the GL
// context (ADR 0001: Go interfaces, not gRPC). Build and Post run inside
// the pos transaction via the shared *sql.Tx.
package ledgerbridge

import (
	"context"
	"database/sql"

	ledgerapp "aivo/internal/ledger/app"
	"aivo/internal/pos/ports"

	"uuid"
)

type Bridge struct {
	ledger *ledgerapp.App
}

var _ ports.Ledger = (*Bridge)(nil)

func New(ledger *ledgerapp.App) *Bridge { return &Bridge{ledger: ledger} }

func (b *Bridge) BuildDraftShiftJournal(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID, draft ports.ShiftJournalDraft) (uuid.UUID, error) {
	tenders := make([]ledgerapp.TenderTotal, len(draft.Tenders))
	for i, t := range draft.Tenders {
		tenders[i] = ledgerapp.TenderTotal{Group: t.Group, AmountCents: t.AmountCents}
	}
	return b.ledger.BuildDraftShiftJournal(ctx, tx, ledgerapp.ShiftJournalInput{
		RestaurantID:   restaurantID,
		ShiftID:        draft.ShiftID,
		CreatedBy:      draft.CreatedBy,
		AccountingDate: draft.AccountingDate,
		Tenders:        tenders,
		VarianceCents:  draft.VarianceCents,
	})
}

func (b *Bridge) PostJournal(ctx context.Context, tx *sql.Tx, restaurantID, docID uuid.UUID) error {
	return b.ledger.PostShiftJournal(ctx, tx, restaurantID, docID)
}
