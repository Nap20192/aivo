// Package migrations embeds the SQL migrations for every context, one
// directory per service (menu, platform, ledger, pos, inventory). Naming:
// {version}_{title}.up.sql / {version}_{title}.down.sql. The runner
// (pkg/migrate) applies only *.up.sql at startup; *.down.sql files are
// the documented rollbacks, run manually via psql.
package migrations

import "embed"

//go:embed menu platform ledger pos inventory
var FS embed.FS
