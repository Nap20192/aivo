// Package app is the Menu context's use-case layer: commands (writes) and
// queries (reads) per CQRS, all as one flat package. adapters/http depends
// on this package's Application, never on ports or the adapters directly —
// that's what keeps the HTTP layer thin.
package app

import (
	"aivo/internal/menu/ports"
)

// Application is every use case the Menu app exposes, grouped into
// Commands (writes) and Queries (reads) per CQRS.
type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	SubmitOrder          SubmitOrderHandler
	SubmitServiceRequest SubmitServiceRequestHandler
}

type Queries struct {
	GetLanding GetLandingHandler
	GetMenu    GetMenuHandler
	GetQR      GetQRHandler
}

// NewApplication wires every command/query handler against store and
// notifier. encKey decrypts a Restaurant's NotificationChannel bot token
// before notifying (see pkg/crypto); baseURL is the origin Table links
// are built under (see GetQRHandler).
func NewApplication(store ports.Store, notifier ports.Notifier, encKey []byte, baseURL string) Application {
	return Application{
		Commands: Commands{
			SubmitOrder:          NewSubmitOrderHandler(store, notifier, encKey),
			SubmitServiceRequest: NewSubmitServiceRequestHandler(store, notifier, encKey),
		},
		Queries: Queries{
			GetLanding: NewGetLandingHandler(store),
			GetMenu:    NewGetMenuHandler(store),
			GetQR:      NewGetQRHandler(store, baseURL),
		},
	}
}
