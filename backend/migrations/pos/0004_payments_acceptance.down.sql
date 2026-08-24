-- Rollback of pos 0004 (manual use only).
DROP INDEX IF EXISTS shifts_open_per_cashier_idx;
ALTER TABLE tickets DROP COLUMN IF EXISTS closed_at;
ALTER TABLE shifts DROP COLUMN IF EXISTS journal_document_id;
ALTER TABLE shifts DROP COLUMN IF EXISTS accepted_by;
ALTER TABLE shifts DROP COLUMN IF EXISTS accepted_at;
DROP TABLE IF EXISTS cash_operations;
DROP TABLE IF EXISTS ticket_payments;
DROP TABLE IF EXISTS payment_methods;
