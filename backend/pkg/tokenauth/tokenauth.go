// Package tokenauth verifies tokens minted by cmd/aivo-auth. It holds
// only the public verification key, never a private key, and has no
// dependency on aivo-auth's signing/minting code — any service (e.g.
// inventory) can import it to authenticate a request from the token
// alone, without calling back to platform or aivo-auth. See
// openspec/changes/split-inventory-microservice/specs/service-auth/spec.md.
package tokenauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"uuid"
)

// Issuer is the only value a valid token's iss claim may hold — aivo-auth
// is the sole signer, so this is a constant, not a Verify parameter.
const Issuer = "aivo-auth"

// alg is the only signing algorithm this package ever produces or
// accepts. Not exposed: there is exactly one.
const alg = "EdDSA"

var (
	// ErrMalformed means the token isn't three base64url segments with a
	// well-formed header/payload, independent of signature validity.
	ErrMalformed = errors.New("tokenauth: malformed token")
	// ErrInvalidSignature means the signature does not verify against
	// the given public key — this also covers any tampering with the
	// header or payload segment, since both are signed.
	ErrInvalidSignature = errors.New("tokenauth: invalid signature")
	// ErrExpired means the token verified but its exp claim is in the past.
	ErrExpired = errors.New("tokenauth: token expired")
	// ErrInvalidIssuer means the token verified but iss != Issuer.
	ErrInvalidIssuer = errors.New("tokenauth: invalid issuer")
)

// Claims are the typed fields a downstream service needs to authorize a
// request without any further lookup.
type Claims struct {
	UserID    uuid.UUID
	TenantID  uuid.UUID
	Roles     []string
	AppID     string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Issuer    string
}

// wireClaims is Claims' JSON wire shape (the JWT-style payload segment).
type wireClaims struct {
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	AppID    string   `json:"app_id"`
	Iat      int64    `json:"iat"`
	Exp      int64    `json:"exp"`
	Iss      string   `json:"iss"`
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Verify checks token's signature against pub, that it isn't expired,
// and that its issuer is aivo-auth, returning the typed claims only if
// all three hold.
func Verify(pub ed25519.PublicKey, token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}
	headerB64, payloadB64, sigB64 := parts[0], parts[1], parts[2]

	headerRaw, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var h header
	if err := json.Unmarshal(headerRaw, &h); err != nil || h.Alg != alg {
		return Claims{}, ErrMalformed
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return Claims{}, ErrMalformed
	}

	if !ed25519.Verify(pub, []byte(headerB64+"."+payloadB64), sig) {
		return Claims{}, ErrInvalidSignature
	}

	var wc wireClaims
	if err := json.Unmarshal(payloadRaw, &wc); err != nil {
		return Claims{}, ErrMalformed
	}
	userID, err := uuid.Parse(wc.UserID)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	tenantID, err := uuid.Parse(wc.TenantID)
	if err != nil {
		return Claims{}, ErrMalformed
	}

	claims := Claims{
		UserID:    userID,
		TenantID:  tenantID,
		Roles:     wc.Roles,
		AppID:     wc.AppID,
		IssuedAt:  time.Unix(wc.Iat, 0).UTC(),
		ExpiresAt: time.Unix(wc.Exp, 0).UTC(),
		Issuer:    wc.Iss,
	}
	if claims.Issuer != Issuer {
		return Claims{}, ErrInvalidIssuer
	}
	if time.Now().After(claims.ExpiresAt) {
		return Claims{}, ErrExpired
	}
	return claims, nil
}

// LoadPublicKey reads an ed25519 public key written by cmd/aivo-auth
// (base64 standard encoding of the raw 32-byte key, see
// backend/cmd/aivo-auth's AUTH_PUBLIC_KEY_FILE) from path.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, err
	}
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("tokenauth: public key file: want %d bytes after base64 decode, got %d", ed25519.PublicKeySize, len(key))
	}
	return ed25519.PublicKey(key), nil
}
