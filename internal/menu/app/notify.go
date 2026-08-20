package app

import (
	"context"
	"errors"
	"log"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"
	"aivo/pkg/crypto"

	"github.com/google/uuid"
)

// notifyOrder best-effort alerts the restaurant's Telegram bot. A missing
// channel (nothing configured yet) or a send failure is logged, never
// fails the command — the order is already durably persisted.
func notifyOrder(ctx context.Context, store ports.Store, notifier ports.Notifier, encKey []byte, restaurant domain.Restaurant, table domain.Table, order domain.Order) {
	botToken, chatID, ok := decryptChannel(ctx, store, encKey, restaurant.ID)
	if !ok {
		return
	}
	if err := notifier.SendOrderNotification(ctx, botToken, chatID, table.Label, order); err != nil {
		log.Printf("command: send order notification: %v", err)
	}
}

// notifyServiceRequest is notifyOrder's counterpart for Service requests —
// same best-effort semantics.
func notifyServiceRequest(ctx context.Context, store ports.Store, notifier ports.Notifier, encKey []byte, restaurant domain.Restaurant, table domain.Table, kind domain.ServiceRequestKind) {
	botToken, chatID, ok := decryptChannel(ctx, store, encKey, restaurant.ID)
	if !ok {
		return
	}
	if err := notifier.SendServiceRequestNotification(ctx, botToken, chatID, table.Label, kind); err != nil {
		log.Printf("command: send service request notification: %v", err)
	}
}

func decryptChannel(ctx context.Context, store ports.Store, encKey []byte, restaurantID uuid.UUID) (botToken, chatID string, ok bool) {
	ch, err := store.NotificationChannel(ctx, restaurantID)
	if err != nil {
		if !errors.Is(err, ports.ErrNotFound) {
			log.Printf("command: notification channel lookup: %v", err)
		}
		return "", "", false
	}
	plaintext, err := crypto.Decrypt(ch.EncryptedBotToken, restaurantID, encKey)
	if err != nil {
		log.Printf("command: decrypt bot token: %v", err)
		return "", "", false
	}
	return string(plaintext), ch.TelegramChatID, true
}
