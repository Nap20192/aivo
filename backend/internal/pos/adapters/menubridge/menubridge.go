// Package menubridge implements pos ports.Menu on top of the Menu
// context's AdminStore — the in-process bridge between the two contexts
// (Go interfaces, not gRPC, per ADR 0001).
package menubridge

import (
	"context"
	"errors"

	menudomain "aivo/internal/domain/menu"
	menuports "aivo/internal/menu/ports"
	"aivo/internal/pos/ports"

	"github.com/google/uuid"
)

type Bridge struct {
	menu menuports.AdminStore
}

var _ ports.Menu = (*Bridge)(nil)

func New(menu menuports.AdminStore) *Bridge { return &Bridge{menu: menu} }

// mapErr translates the menu context's not-found sentinel into the POS
// one so POS callers only ever see their own errors.
func mapErr(err error) error {
	if errors.Is(err, menuports.ErrNotFound) {
		return ports.ErrNotFound
	}
	return err
}

func (b *Bridge) MenuItemByID(ctx context.Context, restaurantID, id uuid.UUID) (menudomain.MenuItem, error) {
	it, err := b.menu.MenuItemByID(ctx, restaurantID, id)
	return it, mapErr(err)
}

func (b *Bridge) Tables(ctx context.Context, restaurantID uuid.UUID) ([]menudomain.Table, error) {
	return b.menu.Tables(ctx, restaurantID)
}

func (b *Bridge) TableByID(ctx context.Context, restaurantID, id uuid.UUID) (menudomain.Table, error) {
	t, err := b.menu.TableByID(ctx, restaurantID, id)
	return t, mapErr(err)
}

func (b *Bridge) PendingServiceRequests(ctx context.Context, restaurantID uuid.UUID) ([]menudomain.ServiceRequest, error) {
	return b.menu.PendingServiceRequests(ctx, restaurantID)
}

func (b *Bridge) AckServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return mapErr(b.menu.SetServiceRequestStatus(ctx, restaurantID, id, menudomain.ServiceRequestAcknowledged))
}

func (b *Bridge) DismissServiceRequest(ctx context.Context, restaurantID, id uuid.UUID) error {
	return mapErr(b.menu.SetServiceRequestStatus(ctx, restaurantID, id, menudomain.ServiceRequestDismissed))
}
