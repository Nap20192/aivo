package qrcode

import (
	"bytes"
	"testing"
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func TestPNG(t *testing.T) {
	data, err := PNG("https://example.com/acme/t/abc123", 256)
	if err != nil {
		t.Fatalf("PNG() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("PNG() returned no bytes")
	}
	if !bytes.HasPrefix(data, pngMagic) {
		t.Fatalf("PNG() output missing PNG magic header, got %x", data[:min(len(data), 8)])
	}
}

func TestPNG_EmptyURL(t *testing.T) {
	if _, err := PNG("", 256); err == nil {
		t.Fatal("PNG() with empty URL: expected error, got nil")
	}
}
