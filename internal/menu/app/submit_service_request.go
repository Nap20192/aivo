package app

import (
	"context"
	"errors"
	"fmt"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"
	"aivo/pkg/session"
)

// ErrServiceRequestAlreadyOpen is returned when the Table already has an
// open (pending, unexpired) ServiceRequest of the same Kind — see
// pkg/session.AllowServiceRequest and CONTEXT.md "Service
// request" (deduped per Table, not per diner session).
var ErrServiceRequestAlreadyOpen = errors.New("a request of this kind is already open for this table")

// SubmitServiceRequest is a diner's request for staff attention with no
// items (e.g. call waiter, request bill) at a Table.
type SubmitServiceRequest struct {
	RestaurantSlug string
	TableToken     string
	Kind           domain.ServiceRequestKind
}

// SubmitServiceRequestHandler resolves the Table, enforces the per-Table
// open-request dedupe, persists the ServiceRequest — then best-effort
// notifies the Restaurant's configured NotificationChannel.
type SubmitServiceRequestHandler struct {
	store    ports.Store
	notifier ports.Notifier
	encKey   []byte
}

// NewSubmitServiceRequestHandler builds a SubmitServiceRequestHandler.
func NewSubmitServiceRequestHandler(store ports.Store, notifier ports.Notifier, encKey []byte) SubmitServiceRequestHandler {
	return SubmitServiceRequestHandler{store: store, notifier: notifier, encKey: encKey}
}

func (h SubmitServiceRequestHandler) Handle(ctx context.Context, cmd SubmitServiceRequest) (domain.ServiceRequest, error) {
	restaurant, table, err := resolveTable(ctx, h.store, cmd.RestaurantSlug, cmd.TableToken)
	if err != nil {
		return domain.ServiceRequest{}, err
	}

	// AllowServiceRequest itself marks the (table, kind) pair open when it
	// returns true — no separate "mark open" call needed.
	if !session.AllowServiceRequest(table.ID, cmd.Kind) {
		return domain.ServiceRequest{}, ErrServiceRequestAlreadyOpen
	}

	sr, err := h.store.CreateServiceRequest(ctx, domain.ServiceRequest{
		RestaurantID: restaurant.ID,
		TableID:      table.ID,
		Kind:         cmd.Kind,
	})
	if err != nil {
		return domain.ServiceRequest{}, fmt.Errorf("command: submit service request: create: %w", err)
	}

	notifyServiceRequest(ctx, h.store, h.notifier, h.encKey, restaurant, table, cmd.Kind)

	return sr, nil
}
