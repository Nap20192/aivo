-- Rollback of ledger 0002 (manual use only). Removes the inventory/COGS
-- map purposes and accounts (accounts referenced by posted journal_lines
-- cannot be deleted — this is a documented rollback for a fresh DB).
DELETE FROM ledger_account_map WHERE purpose IN
    ('inventory', 'accounts_payable', 'received_not_billed', 'cogs', 'inventory_shrinkage', 'inventory_surplus');
DELETE FROM accounts WHERE code IN ('1200', '2100', '2110', '5000', '5910', '4910');
