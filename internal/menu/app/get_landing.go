package app

import (
	"context"
	"fmt"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"
)

// GetLanding asks for a Restaurant's Landing page as seen through one of
// its Tables. Diner session cookie issuance is an HTTP-adapter concern
// (see adapters/http) — this query has no side effect on the caller.
type GetLanding struct {
	RestaurantSlug string
	TableToken     string
}

// LandingResult is a Restaurant's Landing page: the Restaurant and Table
// as resolved, plus every LandingBlock configured for it.
type LandingResult struct {
	Restaurant    domain.Restaurant
	Table         domain.Table
	LandingBlocks []domain.LandingBlock
}

type GetLandingHandler struct {
	store ports.Store
}

func NewGetLandingHandler(store ports.Store) GetLandingHandler {
	return GetLandingHandler{store: store}
}

func (h GetLandingHandler) Handle(ctx context.Context, q GetLanding) (LandingResult, error) {
	restaurant, table, err := resolveTable(ctx, h.store, q.RestaurantSlug, q.TableToken)
	if err != nil {
		return LandingResult{}, err
	}

	blocks, err := h.store.LandingBlocks(ctx, restaurant.ID)
	if err != nil {
		return LandingResult{}, fmt.Errorf("query: get landing: landing blocks: %w", err)
	}

	return LandingResult{Restaurant: restaurant, Table: table, LandingBlocks: blocks}, nil
}
