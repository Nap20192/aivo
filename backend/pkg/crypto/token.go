// Package crypto encrypts/decrypts per-restaurant secrets (the Telegram
// bot token) at rest, per docs/adr/0003. AES-256-GCM via stdlib only; the
// restaurant id is bound in as AEAD additional data so ciphertext from one
// tenant can't be decrypted under another's id. Pure: no env/config reads
// here, callers own the master key's source.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Token returns nBytes of crypto/rand entropy, base64url-encoded — the
// one random-token generator (sessions use 32 bytes, table tokens 16
// per CONTEXT.md "Table link" ~128 bits).
func Token(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto: token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Encrypt seals plaintext under key (must be 32 bytes, AES-256), binding
// the ciphertext to restaurantID via AEAD additional data. The returned
// ciphertext is nonce||sealed and is what gets stored as
// NotificationChannel.EncryptedBotToken.
func Encrypt(plaintext []byte, restaurantID uuid.UUID, key []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	rid := restaurantID[:]
	return gcm.Seal(nonce, nonce, plaintext, rid), nil
}

// Decrypt reverses Encrypt. It fails if key, restaurantID, or the
// ciphertext bytes don't match what Encrypt produced (wrong tenant or
// tampered data).
func Decrypt(ciphertext []byte, restaurantID uuid.UUID, key []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	rid := restaurantID[:]
	plaintext, err := gcm.Open(nil, nonce, sealed, rid)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes (AES-256), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
