// Package app is aivo-auth's Mint business logic: given claims platform
// already authenticated, sign a token. It never sees a password or any
// other credential — the narrow MintParams shape is the only input this
// package accepts. Verification lives separately in pkg/tokenauth, which
// this package deliberately does not import from the signing direction
// (only pkg/tokenauth's Issuer constant is shared, so both sides agree
// on it).
package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aivo/pkg/tokenauth"

	"uuid"
)

// ErrInvalid is wrapped with a detail message for any malformed or
// missing required field in a MintParams.
var ErrInvalid = errors.New("auth: invalid mint request")

// App mints tokens signed by PrivateKey. One per process.
type App struct {
	PrivateKey ed25519.PrivateKey
}

func New(priv ed25519.PrivateKey) *App {
	return &App{PrivateKey: priv}
}

// MintParams is the narrow input Mint accepts — deliberately has no
// password/credential field, matching the service-auth spec's
// requirement that aivo-auth never has a code path for one.
type MintParams struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	Roles    []string
	AppID    AppID
}

// wireHeader/wireClaims mirror pkg/tokenauth's unexported wire types
// field-for-field (same JSON tags) — tokenauth exposes no Sign function
// by design (verifiers only ever hold a public key), so the signing
// side owns its own copy of the wire shape.
type wireHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type wireClaims struct {
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	AppID    string   `json:"app_id"`
	Iat      int64    `json:"iat"`
	Exp      int64    `json:"exp"`
	Iss      string   `json:"iss"`
}

// Mint validates params and returns a signed token whose expiry is
// params.AppID's documented default.
func (a *App) Mint(params MintParams) (string, error) {
	if params.UserID == (uuid.UUID{}) {
		return "", fmt.Errorf("%w: user_id is required", ErrInvalid)
	}
	if params.TenantID == (uuid.UUID{}) {
		return "", fmt.Errorf("%w: tenant_id is required", ErrInvalid)
	}
	if len(params.Roles) == 0 {
		return "", fmt.Errorf("%w: roles is required", ErrInvalid)
	}
	ttl, ok := DefaultExpiry(params.AppID)
	if !ok {
		return "", fmt.Errorf("%w: unknown app_id %q", ErrInvalid, params.AppID)
	}

	now := time.Now()
	headerRaw, err := json.Marshal(wireHeader{Alg: "EdDSA", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	payloadRaw, err := json.Marshal(wireClaims{
		UserID:   params.UserID.String(),
		TenantID: params.TenantID.String(),
		Roles:    params.Roles,
		AppID:    string(params.AppID),
		Iat:      now.Unix(),
		Exp:      now.Add(ttl).Unix(),
		Iss:      tokenauth.Issuer,
	})
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerRaw)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadRaw)
	sig := ed25519.Sign(a.PrivateKey, []byte(headerB64+"."+payloadB64))
	return headerB64 + "." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
