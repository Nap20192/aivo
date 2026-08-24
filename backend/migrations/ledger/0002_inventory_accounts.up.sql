-- Increment-2 (§9): inventory / COGS chart-of-accounts + map purposes.
-- New restaurants get these through the code seed (ledger SeedRestaurantTx);
-- this backfills every EXISTING restaurant. Idempotent via ON CONFLICT.

INSERT INTO accounts (id, restaurant_id, code, name, type, normal_side, postable)
SELECT gen_random_uuid(), r.id, a.code, a.name, a.type, a.normal_side, true
FROM restaurants r
CROSS JOIN (VALUES
    ('1200', 'Inventory', 'asset', 'debit'),
    ('2100', 'Accounts payable', 'liability', 'credit'),
    ('2110', 'Received not billed', 'liability', 'credit'),
    ('5000', 'Cost of goods sold', 'expense', 'debit'),
    ('5910', 'Inventory shrinkage / write-off', 'expense', 'debit'),
    ('4910', 'Inventory surplus', 'revenue', 'credit')
) AS a (code, name, type, normal_side)
ON CONFLICT (restaurant_id, code) DO NOTHING;

INSERT INTO ledger_account_map (restaurant_id, purpose, account_id)
SELECT r.id, m.purpose, acc.id
FROM restaurants r
CROSS JOIN (VALUES
    ('inventory', '1200'),
    ('accounts_payable', '2100'),
    ('received_not_billed', '2110'),
    ('cogs', '5000'),
    ('inventory_shrinkage', '5910'),
    ('inventory_surplus', '4910')
) AS m (purpose, code)
JOIN accounts acc ON acc.restaurant_id = r.id AND acc.code = m.code
ON CONFLICT (restaurant_id, purpose) DO NOTHING;
