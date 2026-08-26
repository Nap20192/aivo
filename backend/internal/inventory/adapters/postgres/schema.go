package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// OpenSchemaDB opens dsn with search_path "inventory, public" set on every
// pooled connection (via pgx RuntimeParams, sent at connection startup —
// unlike a one-off `SET search_path`, this survives connection pooling
// picking a different physical connection later). Every unqualified
// table/query inventory issues then resolves against its own schema
// first, falling back to public for cross-schema reads (pos sales for the
// food-cost report) and FK targets (restaurants, users). Shared by
// cmd/aivo-inventory and this package's integration tests, so both talk
// to inventory's schema the same way (design.md D1).
func OpenSchemaDB(dsn string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("inventory: parse dsn: %w", err)
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["search_path"] = "inventory, public"
	return stdlib.OpenDB(*config), nil
}

// EnsureSchema creates the "inventory" schema if it doesn't exist yet.
// Schema-qualified (not reliant on search_path), so it lands correctly
// regardless of connection state — call this once, before running any
// migration or query that assumes the schema (and so OpenSchemaDB's
// search_path) already resolves: if "inventory" doesn't exist, Postgres
// silently skips it in search_path and unqualified CREATE TABLE would
// land in "public" instead, with no error.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS inventory`)
	return err
}
