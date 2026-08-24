package app

import (
	"context"
	"database/sql"

	ledger "aivo/internal/domain/ledger"
	"aivo/internal/ledger/ports"

	"uuid"
)

// defaultAccounts is the per-restaurant chart of accounts seed (contract §6).
var defaultAccounts = []ledger.Account{
	{Code: "1000", Name: "Cash on hand (drawer)", Type: ledger.TypeAsset, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "1010", Name: "Card clearing", Type: ledger.TypeAsset, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "1020", Name: "Undeposited funds", Type: ledger.TypeAsset, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "1100", Name: "House account receivable", Type: ledger.TypeAsset, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "2000", Name: "Gift card liability", Type: ledger.TypeLiability, NormalSide: ledger.SideCredit, Postable: true},
	{Code: "4000", Name: "Sales revenue", Type: ledger.TypeRevenue, NormalSide: ledger.SideCredit, Postable: true},
	{Code: "4900", Name: "Comps / contra-revenue", Type: ledger.TypeRevenue, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "5900", Name: "Cash over/short", Type: ledger.TypeExpense, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "6000", Name: "Cash movements (pay in/out)", Type: ledger.TypeExpense, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "9999", Name: "Unassigned / rounding", Type: ledger.TypeExpense, NormalSide: ledger.SideDebit, Postable: true},
	// Inventory / COGS (increment-2, §9).
	{Code: "1200", Name: "Inventory", Type: ledger.TypeAsset, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "2100", Name: "Accounts payable", Type: ledger.TypeLiability, NormalSide: ledger.SideCredit, Postable: true},
	{Code: "2110", Name: "Received not billed", Type: ledger.TypeLiability, NormalSide: ledger.SideCredit, Postable: true}, // seeded placeholder, unused
	{Code: "5000", Name: "Cost of goods sold", Type: ledger.TypeExpense, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "5910", Name: "Inventory shrinkage / write-off", Type: ledger.TypeExpense, NormalSide: ledger.SideDebit, Postable: true},
	{Code: "4910", Name: "Inventory surplus", Type: ledger.TypeRevenue, NormalSide: ledger.SideCredit, Postable: true},
}

// defaultMap is the per-restaurant purpose→account-code seed (contract §6).
// void has no mapping (creates no postings).
var defaultMap = []struct{ purpose, code string }{
	{"sales_revenue", "4000"},
	{"cash_drawer", "1000"},
	{"cash_over_short", "5900"},
	{"cash_movement", "6000"},
	{"rounding_unassigned", "9999"},
	{"tender:cash", "1000"},
	{"tender:card", "1010"},
	{"tender:gift_card", "2000"},
	{"tender:comp", "4900"},
	{"tender:house_account", "1100"},
	// Inventory / COGS purposes (increment-2, §9).
	{"inventory", "1200"},
	{"accounts_payable", "2100"},
	{"received_not_billed", "2110"},
	{"cogs", "5000"},
	{"inventory_shrinkage", "5910"},
	{"inventory_surplus", "4910"},
}

// SeedRestaurant provisions the default chart of accounts, the "main"
// cost center, and the account map for a restaurant. Runs in one
// transaction. Not idempotent (unique code) — call once at provisioning.
func (a *App) SeedRestaurant(ctx context.Context, restaurantID uuid.UUID) error {
	return a.store.InTx(ctx, func(st ports.Store) error {
		return a.seedOn(ctx, st, restaurantID)
	})
}

// SeedRestaurantTx seeds the chart of accounts + cost center + account map
// on an externally-provided transaction, so live restaurant provisioning
// (platform) can seed the GL in the SAME transaction as the restaurant
// row (M3/BUG-1). The tx is the platform's; both tables share one Postgres.
func (a *App) SeedRestaurantTx(ctx context.Context, tx *sql.Tx, restaurantID uuid.UUID) error {
	return a.seedOn(ctx, a.store.WithTx(tx), restaurantID)
}

func (a *App) seedOn(ctx context.Context, st ports.Store, restaurantID uuid.UUID) error {
	codeToID := map[string]uuid.UUID{}
	for _, acc := range defaultAccounts {
		acc.ID = a.newID()
		acc.RestaurantID = restaurantID
		if err := st.InsertAccount(ctx, acc); err != nil {
			return err
		}
		codeToID[acc.Code] = acc.ID
	}
	if err := st.InsertCostCenter(ctx, ledger.CostCenter{
		ID: a.newID(), RestaurantID: restaurantID, Code: CostCenterMain, Name: "Main",
	}); err != nil {
		return err
	}
	for _, m := range defaultMap {
		if err := st.PutAccountMap(ctx, restaurantID, m.purpose, codeToID[m.code]); err != nil {
			return err
		}
	}
	return nil
}
