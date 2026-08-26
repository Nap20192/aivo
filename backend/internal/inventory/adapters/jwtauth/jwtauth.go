// Package jwtauth verifies the Ed25519-signed JWTs minted by aivo-auth
// (specs/service-auth), so inventory's REST handlers can authorize a
// request from the token's claims alone, without calling platform per
// request. This is a self-contained verifier (parse + verify signature +
// check exp/iss), not a JWT library — svc-auth's own worktree is building
// a shared verification helper package (tasks.md 4.6); once that lands,
// this package is a natural candidate to be replaced by it or deduped
// into it (see the split-inventory-microservice report).
//
// Token shape is a standard compact JWS: base64url(header) + "." +
// base64url(payload) + "." + base64url(signature), alg "EdDSA", signature
// over the ASCII "header.payload" string.
package jwtauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"uuid"
)

var (
	ErrMalformed        = errors.New("jwtauth: malformed token")
	ErrUnsupportedAlg   = errors.New("jwtauth: unsupported algorithm")
	ErrBadSignature     = errors.New("jwtauth: signature verification failed")
	ErrExpired          = errors.New("jwtauth: token expired")
	ErrIssuerMismatch   = errors.New("jwtauth: unexpected issuer")
	ErrInvalidPublicKey = errors.New("jwtauth: public key must be 32 bytes (Ed25519)")
)

// Claims is a minted token's payload, per specs/service-auth/spec.md: the
// user id, tenant/restaurant id, the user's role(s), the app id it was
// minted for, and expiry/issuer.
type Claims struct {
	UserID    uuid.UUID
	TenantID  uuid.UUID
	Roles     []string
	AppID     string
	ExpiresAt time.Time
	Issuer    string
}

// HasRole reports whether claims carries any of the given roles.
func (c Claims) HasRole(roles ...string) bool {
	for _, have := range c.Roles {
		for _, want := range roles {
			if have == want {
				return true
			}
		}
	}
	return false
}

// jwtHeader is the fixed header this verifier accepts.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtPayload is the wire shape of Claims.
type jwtPayload struct {
	Sub      string   `json:"sub"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	AppID    string   `json:"app_id"`
	Exp      int64    `json:"exp"`
	Iss      string   `json:"iss"`
}

// Verifier checks tokens signed by the Ed25519 key matching PublicKey. If
// Issuer is non-empty, a token's iss claim must match it exactly.
type Verifier struct {
	PublicKey ed25519.PublicKey
	Issuer    string
	// Now defaults to time.Now when nil; overridable for tests.
	Now func() time.Time
}

// NewVerifier constructs a Verifier from a raw 32-byte Ed25519 public key.
func NewVerifier(publicKey []byte, issuer string) (Verifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Verifier{}, ErrInvalidPublicKey
	}
	return Verifier{PublicKey: ed25519.PublicKey(publicKey), Issuer: issuer}, nil
}

// Verify parses token, checks its Ed25519 signature, expiry, and (if
// configured) issuer, and returns its claims.
func (v Verifier) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Claims{}, ErrMalformed
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var header jwtHeader
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return Claims{}, ErrMalformed
	}
	if header.Alg != "EdDSA" {
		return Claims{}, ErrUnsupportedAlg
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	signed := parts[0] + "." + parts[1]
	if len(v.PublicKey) != ed25519.PublicKeySize || !ed25519.Verify(v.PublicKey, []byte(signed), sig) {
		return Claims{}, ErrBadSignature
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return Claims{}, ErrMalformed
	}
	userID, err := uuid.Parse(payload.Sub)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return Claims{}, ErrMalformed
	}

	now := v.Now
	if now == nil {
		now = time.Now
	}
	exp := time.Unix(payload.Exp, 0)
	if payload.Exp == 0 || !now().Before(exp) {
		return Claims{}, ErrExpired
	}
	if v.Issuer != "" && payload.Iss != v.Issuer {
		return Claims{}, ErrIssuerMismatch
	}

	return Claims{
		UserID: userID, TenantID: tenantID, Roles: payload.Roles,
		AppID: payload.AppID, ExpiresAt: exp, Issuer: payload.Iss,
	}, nil
}
