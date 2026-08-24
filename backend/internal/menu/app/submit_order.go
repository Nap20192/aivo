package app

import (
	"context"
	"errors"
	"fmt"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"
	"aivo/pkg/session"

	"uuid"
)

// ErrOrderRateLimited is returned when SessionID has submitted an Order
// too recently (see pkg/session.AllowOrder).
var ErrOrderRateLimited = errors.New("order rate limit exceeded, try again shortly")

// ErrUnknownMenuItem is returned when an OrderLine references a
// MenuItemID that doesn't exist under the resolved Restaurant.
var ErrUnknownMenuItem = errors.New("unknown menu_item_id")

// OrderLineInput is one requested line of a SubmitOrder command: a diner's
// chosen MenuItem, Option picks, and quantity.
type OrderLineInput struct {
	MenuItemID uuid.UUID
	OptionIDs  []uuid.UUID
	Qty        int
}

// SubmitOrder is a diner's request to place an Order at a Table. SessionID
// is the diner's session (see pkg/session) — the HTTP adapter
// owns issuing/reading the session cookie and passes the resulting ID in,
// since cookie I/O is a transport concern this command struct stays
// agnostic of.
type SubmitOrder struct {
	RestaurantSlug string
	TableToken     string
	SessionID      string
	CustomerID     *uuid.UUID // logged-in customer, nil for anonymous diners
	Lines          []OrderLineInput
	Comment        string
}

// SubmitOrderHandler resolves the Table, enforces the per-session order
// rate limit, snapshots the requested lines against the current Menu, and
// persists the Order — then best-effort notifies the Restaurant's
// configured NotificationChannel.
type SubmitOrderHandler struct {
	store    ports.Store
	notifier ports.Notifier
	encKey   []byte
}

// NewSubmitOrderHandler builds a SubmitOrderHandler. encKey decrypts the
// Restaurant's NotificationChannel bot token (see pkg/crypto)
// before notifying — it is never used to encrypt here.
func NewSubmitOrderHandler(store ports.Store, notifier ports.Notifier, encKey []byte) SubmitOrderHandler {
	return SubmitOrderHandler{store: store, notifier: notifier, encKey: encKey}
}

func (h SubmitOrderHandler) Handle(ctx context.Context, cmd SubmitOrder) (domain.Order, error) {
	restaurant, table, err := resolveTable(ctx, h.store, cmd.RestaurantSlug, cmd.TableToken)
	if err != nil {
		return domain.Order{}, err
	}

	if !session.AllowOrder(cmd.SessionID) {
		return domain.Order{}, ErrOrderRateLimited
	}

	_, items, err := h.store.Menu(ctx, restaurant.ID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("command: submit order: menu lookup: %w", err)
	}
	itemsByID := make(map[uuid.UUID]domain.MenuItem, len(items))
	for _, it := range items {
		itemsByID[it.ID] = it
	}

	lines := make([]domain.OrderLine, 0, len(cmd.Lines))
	for _, lr := range cmd.Lines {
		item, ok := itemsByID[lr.MenuItemID]
		if !ok {
			return domain.Order{}, ErrUnknownMenuItem
		}
		line, err := domain.NewOrderLine(item, lr.OptionIDs, lr.Qty)
		if err != nil {
			return domain.Order{}, err
		}
		lines = append(lines, line)
	}

	order, err := h.store.CreateOrder(ctx, domain.Order{
		RestaurantID: restaurant.ID,
		TableID:      table.ID,
		CustomerID:   cmd.CustomerID,
		Lines:        lines,
		Comment:      cmd.Comment,
	})
	if err != nil {
		return domain.Order{}, fmt.Errorf("command: submit order: create order: %w", err)
	}

	notifyOrder(ctx, h.store, h.notifier, h.encKey, restaurant, table, order)

	return order, nil
}
