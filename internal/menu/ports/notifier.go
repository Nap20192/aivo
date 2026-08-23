package ports

import (
	"context"

	"aivo/internal/domain/menu"
)

// Notifier sends Menu alerts (new Order, Service request) to a
// Restaurant's own configured NotificationChannel. botToken/chatID are
// the channel's decrypted credentials — decrypting them is the caller's
// job (see pkg/crypto), not the Notifier's.
type Notifier interface {
	// SendOrderNotification sends a new-order alert for order, addressed
	// to tableLabel (domain.Table.Label), to the Telegram chat identified
	// by botToken/chatID.
	SendOrderNotification(ctx context.Context, botToken, chatID, tableLabel string, order domain.Order) error

	// SendServiceRequestNotification sends a service-request alert (call
	// waiter / request bill) for tableLabel, to the Telegram chat
	// identified by botToken/chatID.
	SendServiceRequestNotification(ctx context.Context, botToken, chatID, tableLabel string, kind domain.ServiceRequestKind) error
}
