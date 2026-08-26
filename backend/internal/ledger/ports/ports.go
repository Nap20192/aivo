// Package ports defines the ledger context's persistence boundary. A
// Store may be bound to a transaction (WithTx) so the pos context can
// build and post a shift journal in the same *sql.Tx as its own writes
// (documented cross-context transaction, contract §3).
package ports

import (
	"context"
	"database/sql"
	"errors"
	"time"

	ledger "aivo/internal/domain/ledger"

	"uuid"
)

// ErrNotFound / ErrConflict mirror the other contexts' sentinels.
var (
	ErrNotFound = errors.New("ledger: not found")
	ErrConflict = errors.New("ledger: conflict")
)

// AccountMapEntry is one purpose→account mapping (with the account code
// for display).
type AccountMapEntry struct {
	Purpose     string
	AccountID   uuid.UUID
	AccountCode string
}

// JournalSummary is a posted document without its lines (list view).
type JournalSummary struct {
	ID             uuid.UUID
	Kind           string
	State          string
	AccountingDate string // YYYY-MM-DD
	RecordedAt     string
	SourceKind     string
	SourceID       *uuid.UUID
	ReversalOf     *uuid.UUID
	TotalCents     int64
}

// Store is ledger persistence, scoped by restaurantID in the query.
type Store interface {
	// WithTx returns a Store bound to tx (same handle set, transactional).
	WithTx(tx *sql.Tx) Store
	// InTx runs fn inside a single transaction against a tx-bound Store.
	InTx(ctx context.Context, fn func(Store) error) error
	// InTxWithConn runs fn inside a single transaction, passing both the
	// raw *sql.Tx and a Store bound to it — for a caller with no
	// pre-existing transaction of its own (the ledger gRPC server) that
	// still needs to invoke PostInventoryJournal/CancelJournalForSource,
	// which take a *sql.Tx because they're normally called inside the
	// producing context's (pos/inventory) own transaction.
	InTxWithConn(ctx context.Context, fn func(tx *sql.Tx, s Store) error) error

	// Accounts.
	InsertAccount(ctx context.Context, a ledger.Account) error
	Accounts(ctx context.Context, restaurantID uuid.UUID) ([]ledger.Account, error)
	AccountByID(ctx context.Context, restaurantID, id uuid.UUID) (ledger.Account, error)

	// Cost centers.
	InsertCostCenter(ctx context.Context, c ledger.CostCenter) error
	CostCenterByCode(ctx context.Context, restaurantID uuid.UUID, code string) (ledger.CostCenter, error)
	CostCenters(ctx context.Context, restaurantID uuid.UUID) ([]ledger.CostCenter, error)

	// Account map.
	PutAccountMap(ctx context.Context, restaurantID uuid.UUID, purpose string, accountID uuid.UUID) error
	AccountForPurpose(ctx context.Context, restaurantID uuid.UUID, purpose string) (uuid.UUID, error)
	AccountMap(ctx context.Context, restaurantID uuid.UUID) ([]AccountMapEntry, error)

	// Documents.
	InsertDocument(ctx context.Context, d *ledger.JournalDocument) error
	DocumentByID(ctx context.Context, restaurantID, id uuid.UUID) (ledger.JournalDocument, error)
	// LiveDocumentBySource returns the single non-cancelled document for a
	// source (e.g. a shift), ErrNotFound if none.
	LiveDocumentBySource(ctx context.Context, restaurantID uuid.UUID, sourceKind string, sourceID uuid.UUID) (ledger.JournalDocument, error)
	// LockDocumentState locks the document row FOR UPDATE and returns its
	// state — so the override and post paths serialize on the same row
	// (append-only guard against a PATCH racing an accept). Must run inside
	// a transaction. ErrNotFound if the document is missing.
	LockDocumentState(ctx context.Context, restaurantID, id uuid.UUID) (ledger.DocumentState, error)
	// ReplaceDraftLines rewrites the lines of a draft document (override
	// path). No-op guard belongs to the caller (must be draft).
	ReplaceDraftLines(ctx context.Context, documentID uuid.UUID, lines []ledger.JournalLine) error
	// MarkPosted flips draft→posted; ErrConflict if not draft.
	MarkPosted(ctx context.Context, restaurantID, id uuid.UUID, postedAt time.Time) error
	// MarkCancelled flips posted→cancelled; ErrConflict if not posted.
	MarkCancelled(ctx context.Context, restaurantID, id uuid.UUID, cancelledAt time.Time) error
	// PostedJournals lists posted documents from a date (inclusive),
	// optionally filtered by account and source kind ("" = any).
	PostedJournals(ctx context.Context, restaurantID uuid.UUID, from string, account *uuid.UUID, source string) ([]JournalSummary, error)
}
