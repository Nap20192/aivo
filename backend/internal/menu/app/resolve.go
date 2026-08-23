package app

import (
	"context"
	"errors"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"
)

// resolveTable resolves slug -> Restaurant -> Table(token), scoped so a
// token is only ever looked up under its claimed restaurant. Returns
// ports.ErrNotFound (wrap-compatible) for either an unknown slug or a
// token that doesn't belong to that restaurant — callers must map that,
// and only that, to a 404; any other error is a real system error. Shared
// by every command/query handler below that takes a (slug, token) pair
// off the wire, so this tenant-isolation check has exactly one
// implementation (see AGENTS.md "Treat tenant isolation ... as
// security-critical").
func resolveTable(ctx context.Context, store ports.Store, slug, token string) (domain.Restaurant, domain.Table, error) {
	restaurant, err := store.RestaurantBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return domain.Restaurant{}, domain.Table{}, ports.ErrNotFound
		}
		return domain.Restaurant{}, domain.Table{}, err
	}
	table, err := store.TableByToken(ctx, restaurant.ID, token)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return domain.Restaurant{}, domain.Table{}, ports.ErrNotFound
		}
		return domain.Restaurant{}, domain.Table{}, err
	}
	return restaurant, table, nil
}
