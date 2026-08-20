package app

import (
	"context"
	"errors"
	"fmt"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"
)

// GetMenu asks for a Restaurant's Menu (Categories + MenuItems) by slug.
type GetMenu struct {
	RestaurantSlug string
}

// MenuResult is a Restaurant's Menu.
type MenuResult struct {
	Categories []domain.Category
	Items      []domain.MenuItem
}

type GetMenuHandler struct {
	store ports.Store
}

func NewGetMenuHandler(store ports.Store) GetMenuHandler {
	return GetMenuHandler{store: store}
}

func (h GetMenuHandler) Handle(ctx context.Context, q GetMenu) (MenuResult, error) {
	restaurant, err := h.store.RestaurantBySlug(ctx, q.RestaurantSlug)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return MenuResult{}, ports.ErrNotFound
		}
		return MenuResult{}, fmt.Errorf("query: get menu: restaurant by slug: %w", err)
	}

	cats, items, err := h.store.Menu(ctx, restaurant.ID)
	if err != nil {
		return MenuResult{}, fmt.Errorf("query: get menu: %w", err)
	}

	return MenuResult{Categories: cats, Items: items}, nil
}
