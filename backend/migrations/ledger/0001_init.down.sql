-- Rollback of ledger 0001 (manual use only; the runner never executes
-- .down.sql). Drop in FK-dependency order.
DROP TABLE IF EXISTS ledger_account_map;
DROP TABLE IF EXISTS journal_lines;
DROP TABLE IF EXISTS journal_documents;
DROP TABLE IF EXISTS cost_centers;
DROP TABLE IF EXISTS accounts;
