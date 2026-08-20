// Package telegram implements ports.Notifier by sending Menu alerts (new
// Order, Service request) to a Restaurant's own Telegram bot (see
// docs/adr/0001).
package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"aivo/internal/menu/domain"
	"aivo/internal/menu/ports"
)

// httpDoer is the subset of *http.Client this package needs, so tests can
// stub it without hitting the network.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// httpClient is the doer used to call the Telegram API. Tests may
// override it with a stub.
var httpClient httpDoer = http.DefaultClient

// Client implements ports.Notifier against the Telegram Bot API.
type Client struct{}

var _ ports.Notifier = Client{}

// New returns a Client ready to send notifications. It carries no state
// (botToken/chatID are per-call, per-Restaurant), so the zero value works
// too — New exists for call-site clarity.
func New() Client {
	return Client{}
}

// SendOrderNotification sends a new-order alert to the Restaurant's
// Telegram chat.
func (Client) SendOrderNotification(ctx context.Context, botToken, chatID, tableLabel string, order domain.Order) error {
	return send(ctx, botToken, chatID, renderOrder(tableLabel, order))
}

// SendServiceRequestNotification sends a service-request alert (call
// waiter / request bill) to the Restaurant's Telegram chat.
func (Client) SendServiceRequestNotification(ctx context.Context, botToken, chatID, tableLabel string, kind domain.ServiceRequestKind) error {
	return send(ctx, botToken, chatID, renderServiceRequest(tableLabel, kind))
}

func renderServiceRequest(tableNumber string, kind domain.ServiceRequestKind) string {
	switch kind {
	case domain.CallWaiter:
		return fmt.Sprintf("Table %s — call waiter", tableNumber)
	case domain.RequestBill:
		return fmt.Sprintf("Table %s — request bill", tableNumber)
	default:
		return fmt.Sprintf("Table %s — %s", tableNumber, kind)
	}
}

func renderOrder(tableNumber string, order domain.Order) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Table %s — new order (#%s)", tableNumber, order.ID)
	for _, line := range order.Lines {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "%d× %s", line.Qty, line.Name)
		if len(line.ChosenOptions) > 0 {
			labels := make([]string, len(line.ChosenOptions))
			for i, opt := range line.ChosenOptions {
				labels[i] = opt.Label
			}
			fmt.Fprintf(&b, " (%s)", strings.Join(labels, ", "))
		}
	}
	if order.Comment != "" {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "Note: %s", order.Comment)
	}
	return b.String()
}

// send POSTs text to the Telegram Bot API's sendMessage endpoint for the
// bot identified by botToken.
func send(ctx context.Context, botToken, chatID, text string) error {
	endpoint := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	form := url.Values{
		"chat_id": {chatID},
		"text":    {text},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: API returned status %d", resp.StatusCode)
	}
	return nil
}
