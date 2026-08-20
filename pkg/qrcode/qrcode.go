// Package qrcode generates QR code images for table links. Codes are
// generated on demand and never persisted (see issue #17).
package qrcode

import (
	"errors"

	"github.com/skip2/go-qrcode"
)

// PNG renders tableLinkURL as a QR code, PNG-encoded, size x size pixels.
func PNG(tableLinkURL string, size int) ([]byte, error) {
	if tableLinkURL == "" {
		return nil, errors.New("qrcode: tableLinkURL is empty")
	}
	return qrcode.Encode(tableLinkURL, qrcode.Medium, size)
}
