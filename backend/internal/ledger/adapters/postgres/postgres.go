// Package postgres implements ledger ports.Store against Postgres via
// database/sql (pgx/v5/stdlib driver), hand-written like the other
// contexts' adapters. A Store carries a querier (pool or *sql.Tx) so it
// can participate in the pos context's transaction (WithTx).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	ledger "aivo/internal/domain/ledger"
	"aivo/internal/ledger/ports"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"uuid"
)

// dbtx is the query surface shared by *sql.DB and *sql.Tx.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	pool *sql.DB // for BeginTx/InTx; nil on a tx-bound store
	q    dbtx    // active querier: pool or a *sql.Tx
}

var _ ports.Store = (*Store)(nil)

func NewStore(db *sql.DB) *Store { return &Store{pool: db, q: db} }

// WithTx returns a Store whose queries run on tx.
func (s *Store) WithTx(tx *sql.Tx) ports.Store { return &Store{pool: s.pool, q: tx} }

// InTx runs fn in one transaction against a tx-bound Store.
func (s *Store) InTx(ctx context.Context, fn func(ports.Store) error) error {
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ledger store: begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := fn(s.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- Accounts ----------------------------------------------------------

func (s *Store) InsertAccount(ctx context.Context, a ledger.Account) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO accounts (id, restaurant_id, code, name, type, normal_side, postable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.ID, a.RestaurantID, a.Code, a.Name, a.Type, a.NormalSide, a.Postable)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("account code exists: %w", ports.ErrConflict)
		}
		return fmt.Errorf("ledger store: insert account: %w", err)
	}
	return nil
}

const accountCols = `id, restaurant_id, code, name, type, normal_side, postable, created_at`

func scanAccount(row interface{ Scan(...any) error }) (ledger.Account, error) {
	var a ledger.Account
	err := row.Scan(&a.ID, &a.RestaurantID, &a.Code, &a.Name, &a.Type, &a.NormalSide, &a.Postable, &a.CreatedAt)
	return a, err
}

func (s *Store) Accounts(ctx context.Context, restaurantID uuid.UUID) ([]ledger.Account, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE restaurant_id = $1 ORDER BY code`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("ledger store: accounts: %w", err)
	}
	defer rows.Close()
	out := []ledger.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("ledger store: accounts: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AccountByID(ctx context.Context, restaurantID, id uuid.UUID) (ledger.Account, error) {
	a, err := scanAccount(s.q.QueryRowContext(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE restaurant_id = $1 AND id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.Account{}, ports.ErrNotFound
	}
	if err != nil {
		return ledger.Account{}, fmt.Errorf("ledger store: account by id: %w", err)
	}
	return a, nil
}

// --- Cost centers ------------------------------------------------------

func (s *Store) InsertCostCenter(ctx context.Context, c ledger.CostCenter) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO cost_centers (id, restaurant_id, code, name) VALUES ($1, $2, $3, $4)`,
		c.ID, c.RestaurantID, c.Code, c.Name)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("cost center exists: %w", ports.ErrConflict)
		}
		return fmt.Errorf("ledger store: insert cost center: %w", err)
	}
	return nil
}

func (s *Store) CostCenterByCode(ctx context.Context, restaurantID uuid.UUID, code string) (ledger.CostCenter, error) {
	var c ledger.CostCenter
	err := s.q.QueryRowContext(ctx,
		`SELECT id, restaurant_id, code, name FROM cost_centers WHERE restaurant_id = $1 AND code = $2`,
		restaurantID, code).Scan(&c.ID, &c.RestaurantID, &c.Code, &c.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.CostCenter{}, ports.ErrNotFound
	}
	if err != nil {
		return ledger.CostCenter{}, fmt.Errorf("ledger store: cost center: %w", err)
	}
	return c, nil
}

// --- Account map -------------------------------------------------------

func (s *Store) PutAccountMap(ctx context.Context, restaurantID uuid.UUID, purpose string, accountID uuid.UUID) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO ledger_account_map (restaurant_id, purpose, account_id) VALUES ($1, $2, $3)
		 ON CONFLICT (restaurant_id, purpose) DO UPDATE SET account_id = EXCLUDED.account_id`,
		restaurantID, purpose, accountID)
	if err != nil {
		return fmt.Errorf("ledger store: put account map: %w", err)
	}
	return nil
}

func (s *Store) AccountForPurpose(ctx context.Context, restaurantID uuid.UUID, purpose string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.q.QueryRowContext(ctx,
		`SELECT account_id FROM ledger_account_map WHERE restaurant_id = $1 AND purpose = $2`,
		restaurantID, purpose).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil(), ports.ErrNotFound
	}
	if err != nil {
		return uuid.Nil(), fmt.Errorf("ledger store: account for purpose: %w", err)
	}
	return id, nil
}

func (s *Store) AccountMap(ctx context.Context, restaurantID uuid.UUID) ([]ports.AccountMapEntry, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT m.purpose, m.account_id, a.code
		 FROM ledger_account_map m JOIN accounts a ON a.id = m.account_id
		 WHERE m.restaurant_id = $1 ORDER BY m.purpose`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("ledger store: account map: %w", err)
	}
	defer rows.Close()
	out := []ports.AccountMapEntry{}
	for rows.Next() {
		var e ports.AccountMapEntry
		if err := rows.Scan(&e.Purpose, &e.AccountID, &e.AccountCode); err != nil {
			return nil, fmt.Errorf("ledger store: account map: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Documents ---------------------------------------------------------

func (s *Store) InsertDocument(ctx context.Context, d *ledger.JournalDocument) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO journal_documents
		   (id, restaurant_id, kind, state, accounting_date, recorded_at, posted_at,
		    cancelled_at, source_kind, source_id, reversal_of, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		d.ID, d.RestaurantID, d.Kind, d.State, d.AccountingDate, d.RecordedAt, d.PostedAt,
		d.CancelledAt, nullString(d.SourceKind), d.SourceID, d.ReversalOf, d.CreatedBy)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("live document already exists for source: %w", ports.ErrConflict)
		}
		return fmt.Errorf("ledger store: insert document: %w", err)
	}
	return s.insertLines(ctx, d.Lines)
}

func (s *Store) insertLines(ctx context.Context, lines []ledger.JournalLine) error {
	for _, l := range lines {
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO journal_lines (id, document_id, account_id, side, amount_cents, cost_center_id, memo, seq)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			l.ID, l.DocumentID, l.AccountID, l.Side, l.AmountCents, l.CostCenterID, l.Memo, l.Seq); err != nil {
			return fmt.Errorf("ledger store: insert line: %w", err)
		}
	}
	return nil
}

func (s *Store) DocumentByID(ctx context.Context, restaurantID, id uuid.UUID) (ledger.JournalDocument, error) {
	var d ledger.JournalDocument
	var sourceKind sql.NullString
	err := s.q.QueryRowContext(ctx,
		`SELECT id, restaurant_id, kind, state, accounting_date, recorded_at, posted_at,
		        cancelled_at, source_kind, source_id, reversal_of, created_by
		 FROM journal_documents WHERE restaurant_id = $1 AND id = $2`, restaurantID, id).
		Scan(&d.ID, &d.RestaurantID, &d.Kind, &d.State, &d.AccountingDate, &d.RecordedAt, &d.PostedAt,
			&d.CancelledAt, &sourceKind, &d.SourceID, &d.ReversalOf, &d.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.JournalDocument{}, ports.ErrNotFound
	}
	if err != nil {
		return ledger.JournalDocument{}, fmt.Errorf("ledger store: document by id: %w", err)
	}
	d.SourceKind = sourceKind.String
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, document_id, account_id, side, amount_cents, cost_center_id, memo, seq
		 FROM journal_lines WHERE document_id = $1 ORDER BY seq`, id)
	if err != nil {
		return ledger.JournalDocument{}, fmt.Errorf("ledger store: document lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l ledger.JournalLine
		if err := rows.Scan(&l.ID, &l.DocumentID, &l.AccountID, &l.Side, &l.AmountCents, &l.CostCenterID, &l.Memo, &l.Seq); err != nil {
			return ledger.JournalDocument{}, fmt.Errorf("ledger store: document lines: scan: %w", err)
		}
		d.Lines = append(d.Lines, l)
	}
	return d, rows.Err()
}

func (s *Store) LiveDocumentBySource(ctx context.Context, restaurantID uuid.UUID, sourceKind string, sourceID uuid.UUID) (ledger.JournalDocument, error) {
	var id uuid.UUID
	err := s.q.QueryRowContext(ctx,
		`SELECT id FROM journal_documents
		 WHERE restaurant_id = $1 AND source_kind = $2 AND source_id = $3 AND state <> 'cancelled'`,
		restaurantID, sourceKind, sourceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.JournalDocument{}, ports.ErrNotFound
	}
	if err != nil {
		return ledger.JournalDocument{}, fmt.Errorf("ledger store: live document by source: %w", err)
	}
	return s.DocumentByID(ctx, restaurantID, id)
}

func (s *Store) ReplaceDraftLines(ctx context.Context, documentID uuid.UUID, lines []ledger.JournalLine) error {
	run := func(st *Store) error {
		if _, err := st.q.ExecContext(ctx, `DELETE FROM journal_lines WHERE document_id = $1`, documentID); err != nil {
			return fmt.Errorf("ledger store: clear draft lines: %w", err)
		}
		return st.insertLines(ctx, lines)
	}
	// Already in a tx? run directly; else wrap one.
	if s.pool == nil {
		return run(s)
	}
	return s.InTx(ctx, func(st ports.Store) error { return run(st.(*Store)) })
}

func (s *Store) MarkPosted(ctx context.Context, restaurantID, id uuid.UUID, postedAt time.Time) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE journal_documents SET state = 'posted', posted_at = $3
		 WHERE restaurant_id = $1 AND id = $2 AND state = 'draft'`, restaurantID, id, postedAt)
	if err != nil {
		return fmt.Errorf("ledger store: mark posted: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("document not draft: %w", ports.ErrConflict)
	}
	return nil
}

func (s *Store) MarkCancelled(ctx context.Context, restaurantID, id uuid.UUID, cancelledAt time.Time) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE journal_documents SET state = 'cancelled', cancelled_at = $3
		 WHERE restaurant_id = $1 AND id = $2 AND state = 'posted'`, restaurantID, id, cancelledAt)
	if err != nil {
		return fmt.Errorf("ledger store: mark cancelled: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("document not posted: %w", ports.ErrConflict)
	}
	return nil
}

func (s *Store) PostedJournals(ctx context.Context, restaurantID uuid.UUID, from string, account *uuid.UUID, source string) ([]ports.JournalSummary, error) {
	// Optional filters via COALESCE-style predicates kept explicit.
	rows, err := s.q.QueryContext(ctx,
		`SELECT d.id, d.kind, d.state, d.accounting_date, d.recorded_at, d.source_kind,
		        d.source_id, d.reversal_of,
		        COALESCE((SELECT sum(amount_cents) FROM journal_lines l
		                  WHERE l.document_id = d.id AND l.side = 'debit'), 0) AS total_cents
		 FROM journal_documents d
		 WHERE d.restaurant_id = $1 AND d.state = 'posted' AND d.accounting_date >= $2::date
		   AND ($3::uuid IS NULL OR EXISTS (
		         SELECT 1 FROM journal_lines l WHERE l.document_id = d.id AND l.account_id = $3::uuid))
		   AND ($4 = '' OR d.source_kind = $4)
		 ORDER BY d.accounting_date, d.recorded_at`,
		restaurantID, from, account, source)
	if err != nil {
		return nil, fmt.Errorf("ledger store: posted journals: %w", err)
	}
	defer rows.Close()
	out := []ports.JournalSummary{}
	for rows.Next() {
		var (
			j          ports.JournalSummary
			accDate    time.Time
			recAt      time.Time
			sourceKind sql.NullString
		)
		if err := rows.Scan(&j.ID, &j.Kind, &j.State, &accDate, &recAt, &sourceKind,
			&j.SourceID, &j.ReversalOf, &j.TotalCents); err != nil {
			return nil, fmt.Errorf("ledger store: posted journals: scan: %w", err)
		}
		j.AccountingDate = accDate.Format("2006-01-02")
		j.RecordedAt = recAt.Format(time.RFC3339)
		j.SourceKind = sourceKind.String
		out = append(out, j)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
