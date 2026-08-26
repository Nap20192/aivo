// Package grpcserver implements ledgerv1.LedgerServiceServer over the
// ledger app — cmd/aivo-server's :9080 gRPC listener, the inbound side
// of inventory's outbox (openspec/changes/split-inventory-microservice).
// Every RPC is idempotent on its request's source id: a redelivered
// event is a no-op ack, not a duplicate post/reversal (service-events
// spec's idempotency requirement).
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	ledger "aivo/internal/domain/ledger"
	ledgerapp "aivo/internal/ledger/app"
	"aivo/internal/ledger/ports"
	ledgerv1 "aivo/internal/ledger/v1"

	"uuid"
)

const dateLayout = "2006-01-02"

// Source kinds inventory posts under (internal/domain/inventory's
// SourceCOGS/SourceReceipt/SourceWriteoff/SourceStocktake). Ledger
// cannot import inventory's domain package (contexts don't share enum
// packages, D6) so these are duplicated literals — keep in sync.
const (
	sourceCOGS      = "cogs"
	sourceReceipt   = "inventory_receipt"
	sourceWriteoff  = "inventory_writeoff"
	sourceStocktake = "inventory_stocktake"
)

// Server implements ledgerv1.LedgerServiceServer over the ledger app.
type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	ledger *ledgerapp.App
}

func New(ledger *ledgerapp.App) *Server { return &Server{ledger: ledger} }

var _ ledgerv1.LedgerServiceServer = (*Server)(nil)

func toLines(in []*ledgerv1.JournalLine) []ledgerapp.InventoryJournalLine {
	out := make([]ledgerapp.InventoryJournalLine, len(in))
	for i, l := range in {
		out[i] = ledgerapp.InventoryJournalLine{Purpose: l.Purpose, Side: ledger.Side(l.Side), AmountCents: l.AmountCents}
	}
	return out
}

// postJournal is the shared idempotent-post path for every Post*Journal
// RPC. sourceKind names the RPC's document family; sourceIDStr is the
// idempotency key (D3): a live document already existing for
// (sourceKind, sourceID) means this is a redelivery, so its id is
// returned without posting again.
func (s *Server) postJournal(ctx context.Context, restaurantIDStr, createdByStr, sourceKind, sourceIDStr, accountingDateStr string, lines []*ledgerv1.JournalLine) (string, error) {
	restaurantID, err := uuid.Parse(restaurantIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid restaurant_id: %w", err)
	}
	createdBy, err := uuid.Parse(createdByStr)
	if err != nil {
		return "", fmt.Errorf("invalid created_by: %w", err)
	}
	sourceID, err := uuid.Parse(sourceIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid source id: %w", err)
	}
	accountingDate, err := time.Parse(dateLayout, accountingDateStr)
	if err != nil {
		return "", fmt.Errorf("invalid accounting_date: %w", err)
	}

	if existing, err := s.ledger.LiveDocumentBySource(ctx, restaurantID, sourceKind, sourceID); err == nil {
		return existing.ID.String(), nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return "", err
	}

	docID, err := s.ledger.PostInventoryJournalAtomic(ctx, ledgerapp.InventoryJournalInput{
		RestaurantID: restaurantID, CreatedBy: createdBy, SourceKind: sourceKind, SourceID: sourceID,
		AccountingDate: accountingDate, Lines: toLines(lines),
	})
	if errors.Is(err, ports.ErrConflict) {
		// Lost a race with a concurrent redelivery: the winner already
		// posted the live document for this source.
		existing, lookErr := s.ledger.LiveDocumentBySource(ctx, restaurantID, sourceKind, sourceID)
		if lookErr != nil {
			return "", lookErr
		}
		return existing.ID.String(), nil
	}
	if err != nil {
		return "", err
	}
	return docID.String(), nil
}

// reverseJournal is the shared idempotent-reverse path for every
// Reverse*Journal RPC.
func (s *Server) reverseJournal(ctx context.Context, restaurantIDStr, sourceKind, sourceIDStr string) (string, error) {
	restaurantID, err := uuid.Parse(restaurantIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid restaurant_id: %w", err)
	}
	sourceID, err := uuid.Parse(sourceIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid source id: %w", err)
	}

	if existing, err := s.ledger.LiveDocumentBySource(ctx, restaurantID, sourceKind, sourceID); err == nil {
		if existing.Kind == ledger.KindReversal {
			return existing.ID.String(), nil
		}
	} else if !errors.Is(err, ports.ErrNotFound) {
		return "", err
	}

	revID, err := s.ledger.CancelJournalForSourceAtomic(ctx, restaurantID, sourceKind, sourceID)
	if errors.Is(err, ports.ErrConflict) {
		existing, lookErr := s.ledger.LiveDocumentBySource(ctx, restaurantID, sourceKind, sourceID)
		if lookErr != nil {
			return "", lookErr
		}
		return existing.ID.String(), nil
	}
	if err != nil {
		return "", err
	}
	return revID.String(), nil
}

func (s *Server) PostCOGSJournal(ctx context.Context, req *ledgerv1.PostCOGSJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	id, err := s.postJournal(ctx, req.RestaurantId, req.CreatedBy, sourceCOGS, req.TicketId, req.AccountingDate, req.Lines)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.PostJournalResponse{DocumentId: id}, nil
}

func (s *Server) ReverseCOGSJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	id, err := s.reverseJournal(ctx, req.RestaurantId, sourceCOGS, req.SourceId)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: id}, nil
}

func (s *Server) PostReceiptJournal(ctx context.Context, req *ledgerv1.PostReceiptJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	id, err := s.postJournal(ctx, req.RestaurantId, req.CreatedBy, sourceReceipt, req.ReceiptId, req.AccountingDate, req.Lines)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.PostJournalResponse{DocumentId: id}, nil
}

func (s *Server) ReverseReceiptJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	id, err := s.reverseJournal(ctx, req.RestaurantId, sourceReceipt, req.SourceId)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: id}, nil
}

func (s *Server) PostWriteOffJournal(ctx context.Context, req *ledgerv1.PostWriteOffJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	id, err := s.postJournal(ctx, req.RestaurantId, req.CreatedBy, sourceWriteoff, req.WriteOffId, req.AccountingDate, req.Lines)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.PostJournalResponse{DocumentId: id}, nil
}

func (s *Server) ReverseWriteOffJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	id, err := s.reverseJournal(ctx, req.RestaurantId, sourceWriteoff, req.SourceId)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: id}, nil
}

func (s *Server) PostStocktakeJournal(ctx context.Context, req *ledgerv1.PostStocktakeJournalRequest) (*ledgerv1.PostJournalResponse, error) {
	id, err := s.postJournal(ctx, req.RestaurantId, req.CreatedBy, sourceStocktake, req.StocktakeId, req.AccountingDate, req.Lines)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.PostJournalResponse{DocumentId: id}, nil
}

func (s *Server) ReverseStocktakeJournal(ctx context.Context, req *ledgerv1.ReverseJournalRequest) (*ledgerv1.ReverseJournalResponse, error) {
	id, err := s.reverseJournal(ctx, req.RestaurantId, sourceStocktake, req.SourceId)
	if err != nil {
		return nil, err
	}
	return &ledgerv1.ReverseJournalResponse{ReversalDocumentId: id}, nil
}
