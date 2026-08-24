// Package provisioning wires the per-restaurant seeding that must happen
// when a restaurant is created (live self-registration or admin), across
// the ledger and pos contexts, inside the platform's restaurant-creation
// transaction. Without it a self-registered restaurant has no chart of
// accounts and no tender methods, and the whole financial path 404/422s
// (M3 / BUG-1).
package provisioning

import (
	"context"
	"database/sql"

	ledgerapp "aivo/internal/ledger/app"
	pospg "aivo/internal/pos/adapters/postgres"

	"uuid"
)

// RestaurantProvisioner returns a hook that seeds the chart of accounts +
// account map + cost center (ledger) and the default payment methods (pos)
// for a new restaurant, on the provided transaction — atomic with the
// restaurant row.
func RestaurantProvisioner(ledger *ledgerapp.App) func(context.Context, *sql.Tx, uuid.UUID) error {
	return func(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID) error {
		if err := ledger.SeedRestaurantTx(ctx, tx, restaurantID); err != nil {
			return err
		}
		return pospg.SeedDefaultPaymentMethods(ctx, tx, restaurantID)
	}
}
