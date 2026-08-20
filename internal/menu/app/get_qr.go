package app

import (
	"context"
	"fmt"
	"strings"

	"aivo/internal/menu/ports"
	"aivo/pkg/qrcode"
)

// GetQR asks for a Table link's QR code, PNG-encoded.
type GetQR struct {
	RestaurantSlug string
	TableToken     string
}

type GetQRHandler struct {
	store   ports.Store
	baseURL string // origin table links are built under, e.g. "https://menu.example.com"
}

func NewGetQRHandler(store ports.Store, baseURL string) GetQRHandler {
	return GetQRHandler{store: store, baseURL: strings.TrimSuffix(baseURL, "/")}
}

// Handle resolves the Table link, builds its full URL (see CONTEXT.md
// "Table link": {restaurant_slug}/t/{table_token}), and renders it as a
// PNG QR code — generated on demand and never persisted (see issue #17).
func (h GetQRHandler) Handle(ctx context.Context, q GetQR) ([]byte, error) {
	if _, _, err := resolveTable(ctx, h.store, q.RestaurantSlug, q.TableToken); err != nil {
		return nil, err
	}

	link := fmt.Sprintf("%s/%s/t/%s", h.baseURL, q.RestaurantSlug, q.TableToken)
	png, err := qrcode.PNG(link, 256)
	if err != nil {
		return nil, fmt.Errorf("query: get qr: %w", err)
	}
	return png, nil
}
