// Package migrate is a minimal forward-only SQL migration runner: apply
// each embedded *.sql file once, in the order the caller lists sources,
// tracked in a schema_migrations table. No down migrations — applied
// files are immutable (extend via new numbered files).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
)

// Source is one context's migration directory.
type Source struct {
	Name string // context name, e.g. "menu", "platform", "pos"
	FS   fs.FS  // contains migrations/*.sql
	Dir  string // path of the sql files within FS, e.g. "migrations"
}

// Apply runs every unapplied migration of every source, sources in the
// given order, files in lexical order within a source. Each file runs in
// its own transaction together with its bookkeeping row.
func Apply(ctx context.Context, db *sql.DB, sources []Source) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		source   text NOT NULL,
		filename text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		PRIMARY KEY (source, filename)
	)`); err != nil {
		return fmt.Errorf("migrate: bookkeeping table: %w", err)
	}

	for _, src := range sources {
		entries, err := fs.ReadDir(src.FS, src.Dir)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", src.Name, err)
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)

		for _, name := range names {
			var applied bool
			if err := db.QueryRowContext(ctx,
				`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE source = $1 AND filename = $2)`,
				src.Name, name).Scan(&applied); err != nil {
				return fmt.Errorf("migrate: check %s/%s: %w", src.Name, name, err)
			}
			if applied {
				continue
			}

			sqlBytes, err := fs.ReadFile(src.FS, src.Dir+"/"+name)
			if err != nil {
				return fmt.Errorf("migrate: read %s/%s: %w", src.Name, name, err)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("migrate: begin %s/%s: %w", src.Name, name, err)
			}
			if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
				tx.Rollback()
				return fmt.Errorf("migrate: apply %s/%s: %w", src.Name, name, err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO schema_migrations (source, filename) VALUES ($1, $2)`, src.Name, name); err != nil {
				tx.Rollback()
				return fmt.Errorf("migrate: record %s/%s: %w", src.Name, name, err)
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("migrate: commit %s/%s: %w", src.Name, name, err)
			}
		}
	}
	return nil
}
