-- name: InsertAccount :exec
INSERT INTO accounts (id, restaurant_id, code, name, type, normal_side, postable)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: AccountsByRestaurant :many
SELECT id, restaurant_id, code, name, type, normal_side, postable, created_at
FROM accounts WHERE restaurant_id = $1 ORDER BY code;

-- name: AccountByID :one
SELECT id, restaurant_id, code, name, type, normal_side, postable, created_at
FROM accounts WHERE restaurant_id = $1 AND id = $2;

-- name: AccountPostedCount :one
SELECT count(*) FROM journal_lines jl
JOIN journal_documents jd ON jd.id = jl.document_id
WHERE jl.account_id = $1 AND jd.state = 'posted';

-- name: InsertCostCenter :exec
INSERT INTO cost_centers (id, restaurant_id, code, name) VALUES ($1, $2, $3, $4);

-- name: CostCenterByCode :one
SELECT id, restaurant_id, code, name FROM cost_centers
WHERE restaurant_id = $1 AND code = $2;

-- name: UpsertAccountMap :exec
INSERT INTO ledger_account_map (restaurant_id, purpose, account_id)
VALUES ($1, $2, $3)
ON CONFLICT (restaurant_id, purpose) DO UPDATE SET account_id = EXCLUDED.account_id;

-- name: AccountForPurpose :one
SELECT account_id FROM ledger_account_map WHERE restaurant_id = $1 AND purpose = $2;

-- name: AccountMap :many
SELECT m.purpose, m.account_id, a.code
FROM ledger_account_map m JOIN accounts a ON a.id = m.account_id
WHERE m.restaurant_id = $1 ORDER BY m.purpose;

-- name: InsertDocument :exec
INSERT INTO journal_documents
    (id, restaurant_id, kind, state, accounting_date, recorded_at, posted_at,
     cancelled_at, source_kind, source_id, reversal_of, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: InsertLine :exec
INSERT INTO journal_lines
    (id, document_id, account_id, side, amount_cents, cost_center_id, memo, seq)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DocumentByID :one
SELECT id, restaurant_id, kind, state, accounting_date, recorded_at, posted_at,
       cancelled_at, source_kind, source_id, reversal_of, created_by
FROM journal_documents WHERE restaurant_id = $1 AND id = $2;

-- name: LinesForDocument :many
SELECT id, document_id, account_id, side, amount_cents, cost_center_id, memo, seq
FROM journal_lines WHERE document_id = $1 ORDER BY seq;

-- name: MarkDocumentPosted :exec
UPDATE journal_documents SET state = 'posted', posted_at = $3
WHERE restaurant_id = $1 AND id = $2 AND state = 'draft';

-- name: MarkDocumentCancelled :exec
UPDATE journal_documents SET state = 'cancelled', cancelled_at = $3
WHERE restaurant_id = $1 AND id = $2 AND state = 'posted';

-- name: PostedJournals :many
SELECT id, kind, state, accounting_date, recorded_at, source_kind, source_id, reversal_of
FROM journal_documents
WHERE restaurant_id = $1 AND state = 'posted' AND accounting_date >= $2
ORDER BY accounting_date, recorded_at;
