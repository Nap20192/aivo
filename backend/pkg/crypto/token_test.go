package crypto

import (
	"bytes"
	"testing"

	"uuid"
)

func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestEncryptDecrypt(t *testing.T) {
	key := testKey()
	restaurantID := uuid.New()
	otherRestaurantID := uuid.New()
	plaintext := []byte("123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")

	ciphertext, err := Encrypt(plaintext, restaurantID, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatalf("ciphertext contains plaintext")
	}

	cases := []struct {
		name    string
		mutate  func(ct []byte, rid uuid.UUID) ([]byte, uuid.UUID)
		wantErr bool
	}{
		{
			name: "round trip succeeds",
			mutate: func(ct []byte, rid uuid.UUID) ([]byte, uuid.UUID) {
				return ct, rid
			},
			wantErr: false,
		},
		{
			name: "wrong restaurant id fails",
			mutate: func(ct []byte, _ uuid.UUID) ([]byte, uuid.UUID) {
				return ct, otherRestaurantID
			},
			wantErr: true,
		},
		{
			name: "tampered ciphertext fails",
			mutate: func(ct []byte, rid uuid.UUID) ([]byte, uuid.UUID) {
				tampered := append([]byte(nil), ct...)
				tampered[len(tampered)-1] ^= 0xFF
				return tampered, rid
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ct, rid := tc.mutate(ciphertext, restaurantID)
			got, err := Decrypt(ct, rid, key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Decrypt: expected error, got plaintext %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Fatalf("Decrypt: got %q, want %q", got, plaintext)
			}
		})
	}
}

func TestEncryptRejectsBadKeyLength(t *testing.T) {
	_, err := Encrypt([]byte("x"), uuid.New(), []byte("too-short"))
	if err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
