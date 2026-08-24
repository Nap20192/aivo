package app

import (
	"context"
	"errors"
	"fmt"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/ports"

	"uuid"
)

// ErrSupplierNameTaken is returned when a supplier name is already used.
var ErrSupplierNameTaken = errors.New("inventory: supplier name already used")

func (a *App) CreateSupplier(ctx context.Context, restaurantID uuid.UUID, name string, contacts map[string]string, note string) (inv.Supplier, error) {
	if name == "" {
		return inv.Supplier{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	sup := inv.Supplier{ID: a.newID(), RestaurantID: restaurantID, Name: name, Contacts: contacts, Note: note}
	if err := a.store.InsertSupplier(ctx, sup); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return inv.Supplier{}, ErrSupplierNameTaken
		}
		return inv.Supplier{}, err
	}
	return sup, nil
}

func (a *App) Suppliers(ctx context.Context, restaurantID uuid.UUID) ([]inv.Supplier, error) {
	return a.store.Suppliers(ctx, restaurantID)
}

// SupplierPatch is a partial supplier update.
type SupplierPatch struct {
	Name     *string
	Contacts map[string]string
	Archived *bool
}

func (a *App) UpdateSupplier(ctx context.Context, restaurantID, id uuid.UUID, patch SupplierPatch) (inv.Supplier, error) {
	sup, err := a.store.SupplierByID(ctx, restaurantID, id)
	if err != nil {
		return inv.Supplier{}, err
	}
	if patch.Name != nil {
		sup.Name = *patch.Name
	}
	if patch.Contacts != nil {
		sup.Contacts = patch.Contacts
	}
	if patch.Archived != nil {
		sup.Archived = *patch.Archived
	}
	if err := a.store.UpdateSupplier(ctx, sup); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return inv.Supplier{}, ErrSupplierNameTaken
		}
		return inv.Supplier{}, err
	}
	return sup, nil
}
