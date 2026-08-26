// Package ledgerclient implements outbox.Deliverer over a gRPC client to
// ledger's LedgerService (backend/proto/ledger/v1/ledger.proto) — the
// transport inventory's outbox.Poller uses to deliver every
// inventory→ledger GL posting (design.md D2: both edges use the same
// outbox+poller mechanism, no synchronous leg).
package ledgerclient

import (
	"context"
	"encoding/json"
	"fmt"

	inventoryapp "aivo/internal/inventory/app"
	ledgerv1 "aivo/internal/ledger/v1"
	"aivo/pkg/outbox"

	"google.golang.org/grpc"
)

// journalPayload is the JSON shape events.go's postJournalPayload
// marshals — decoded here to build a ledger.v1 Post*JournalRequest.
type journalPayload struct {
	CreatedBy      string `json:"created_by"`
	AccountingDate string `json:"accounting_date"`
	Lines          []struct {
		Purpose     string `json:"purpose"`
		Side        string `json:"side"`
		AmountCents int64  `json:"amount_cents"`
	} `json:"lines"`
}

// Deliverer dispatches a PendingEvent to the LedgerService RPC matching
// its Name (design.md D4: bespoke typed RPC per event, no generic
// envelope).
type Deliverer struct {
	Client ledgerv1.LedgerServiceClient
}

var _ outbox.Deliverer = (*Deliverer)(nil)

// New builds a Deliverer over an existing gRPC connection to ledger's
// :9080 listener (cmd/aivo-server, tasks.md 6.1).
func New(conn grpc.ClientConnInterface) *Deliverer {
	return &Deliverer{Client: ledgerv1.NewLedgerServiceClient(conn)}
}

func (d *Deliverer) Deliver(ctx context.Context, ev outbox.PendingEvent) error {
	restaurantID := ev.RestaurantID.V.String()
	sourceID := ev.AggregateID.String()

	switch ev.Name {
	case inventoryapp.EventCOGSPosted:
		req, err := buildPostJournalRequest(ev, restaurantID, sourceID)
		if err != nil {
			return err
		}
		_, err = d.Client.PostCOGSJournal(ctx, &ledgerv1.PostCOGSJournalRequest{
			RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, SourceId: req.SourceId,
			AccountingDate: req.AccountingDate, Lines: req.Lines,
		})
		return err
	case inventoryapp.EventReceiptPosted:
		req, err := buildPostJournalRequest(ev, restaurantID, sourceID)
		if err != nil {
			return err
		}
		_, err = d.Client.PostReceiptJournal(ctx, &ledgerv1.PostReceiptJournalRequest{
			RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, SourceId: req.SourceId,
			AccountingDate: req.AccountingDate, Lines: req.Lines,
		})
		return err
	case inventoryapp.EventWriteOffPosted:
		req, err := buildPostJournalRequest(ev, restaurantID, sourceID)
		if err != nil {
			return err
		}
		_, err = d.Client.PostWriteOffJournal(ctx, &ledgerv1.PostWriteOffJournalRequest{
			RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, SourceId: req.SourceId,
			AccountingDate: req.AccountingDate, Lines: req.Lines,
		})
		return err
	case inventoryapp.EventStocktakePosted:
		req, err := buildPostJournalRequest(ev, restaurantID, sourceID)
		if err != nil {
			return err
		}
		_, err = d.Client.PostStocktakeJournal(ctx, &ledgerv1.PostStocktakeJournalRequest{
			RestaurantId: req.RestaurantId, CreatedBy: req.CreatedBy, SourceId: req.SourceId,
			AccountingDate: req.AccountingDate, Lines: req.Lines,
		})
		return err
	case inventoryapp.EventReceiptCancelled:
		_, err := d.Client.CancelReceiptJournal(ctx, &ledgerv1.CancelReceiptJournalRequest{RestaurantId: restaurantID, SourceId: sourceID})
		return err
	case inventoryapp.EventWriteOffCancelled:
		_, err := d.Client.CancelWriteOffJournal(ctx, &ledgerv1.CancelWriteOffJournalRequest{RestaurantId: restaurantID, SourceId: sourceID})
		return err
	case inventoryapp.EventStocktakeCancelled:
		_, err := d.Client.CancelStocktakeJournal(ctx, &ledgerv1.CancelStocktakeJournalRequest{RestaurantId: restaurantID, SourceId: sourceID})
		return err
	default:
		return fmt.Errorf("ledgerclient: unknown event name %q", ev.Name)
	}
}

// postJournalRequest holds the fields shared by every Post*JournalRequest
// message (they're structurally identical — see ledger.proto's comment).
type postJournalRequest struct {
	RestaurantId   string
	CreatedBy      string
	SourceId       string
	AccountingDate string
	Lines          []*ledgerv1.JournalLine
}

func buildPostJournalRequest(ev outbox.PendingEvent, restaurantID, sourceID string) (postJournalRequest, error) {
	var p journalPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return postJournalRequest{}, fmt.Errorf("ledgerclient: decode payload for %s: %w", ev.Name, err)
	}
	lines := make([]*ledgerv1.JournalLine, len(p.Lines))
	for i, l := range p.Lines {
		lines[i] = &ledgerv1.JournalLine{Purpose: l.Purpose, Side: l.Side, AmountCents: l.AmountCents}
	}
	return postJournalRequest{
		RestaurantId: restaurantID, CreatedBy: p.CreatedBy, SourceId: sourceID,
		AccountingDate: p.AccountingDate, Lines: lines,
	}, nil
}
