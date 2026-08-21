package postgres

import "embed"

// MigrationsFS embeds this context's migrations for pkg/migrate.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
