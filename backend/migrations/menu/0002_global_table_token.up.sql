-- The /api/v1/t/{table_token} diner entry point (docs/PLATFORM.md) looks
-- a table up by token alone, without a slug. Tokens are ~128-bit random,
-- so a global uniqueness constraint costs nothing and makes the
-- token-only lookup safe.
CREATE UNIQUE INDEX tables_token_idx ON tables (token);
