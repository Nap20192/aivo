package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"aivo/pkg/tokenauth"

	"uuid"
)

func validParams() MintParams {
	return MintParams{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Roles:    []string{"owner"},
		AppID:    AppAdmin,
	}
}

func TestMint_ProducesAVerifiableToken(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	a := New(priv)
	params := validParams()

	token, err := a.Mint(params)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	claims, err := tokenauth.Verify(pub, token)
	if err != nil {
		t.Fatalf("tokenauth.Verify(minted token): %v", err)
	}
	if claims.UserID != params.UserID {
		t.Errorf("UserID = %v, want %v", claims.UserID, params.UserID)
	}
	if claims.TenantID != params.TenantID {
		t.Errorf("TenantID = %v, want %v", claims.TenantID, params.TenantID)
	}
	if claims.AppID != string(params.AppID) {
		t.Errorf("AppID = %q, want %q", claims.AppID, params.AppID)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "owner" {
		t.Errorf("Roles = %v, want [owner]", claims.Roles)
	}
}

func TestMint_TamperedTokenFailsVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	a := New(priv)
	token, err := a.Mint(validParams())
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	// Flip a bit inside the decoded signature (not the token's raw
	// string) — flipping the trailing base64 character alone can decode
	// to the same byte, since an unpadded base64 quantum's last
	// character may only carry a couple of bits.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("Mint produced a token with %d segments, want 3", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sig[0] ^= 0xFF
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)

	if _, err := tokenauth.Verify(pub, tampered); err == nil {
		t.Fatal("expected a tampered token to fail verification")
	}
}

func TestMint_EachAppIDGetsItsDocumentedExpiry(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	a := New(priv)

	cases := []struct {
		id   AppID
		want time.Duration
	}{
		{AppAdmin, 8 * time.Hour},
		{AppPOS, 12 * time.Hour},
		{AppWaiter, 12 * time.Hour},
		{AppMenu, time.Hour},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			params := validParams()
			params.AppID = tc.id
			before := time.Now()
			token, err := a.Mint(params)
			if err != nil {
				t.Fatalf("Mint: %v", err)
			}
			pub := priv.Public().(ed25519.PublicKey)
			claims, err := tokenauth.Verify(pub, token)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			gotTTL := claims.ExpiresAt.Sub(before)
			// exp/iat are second-granularity unix timestamps, so allow a
			// couple seconds of slack around the expected TTL.
			if gotTTL < tc.want-2*time.Second || gotTTL > tc.want+2*time.Second {
				t.Errorf("expiry for %s = %v from mint time, want ~%v", tc.id, gotTTL, tc.want)
			}
		})
	}
}

func TestMint_RejectsMissingOrMalformedFields(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	a := New(priv)

	cases := []struct {
		name    string
		mutate  func(p *MintParams)
		wantErr error
	}{
		{"zero user_id", func(p *MintParams) { p.UserID = uuid.UUID{} }, ErrInvalid},
		{"zero tenant_id", func(p *MintParams) { p.TenantID = uuid.UUID{} }, ErrInvalid},
		{"nil roles", func(p *MintParams) { p.Roles = nil }, ErrInvalid},
		{"empty roles", func(p *MintParams) { p.Roles = []string{} }, ErrInvalid},
		{"unknown app_id", func(p *MintParams) { p.AppID = "not-a-surface" }, ErrInvalid},
		{"empty app_id", func(p *MintParams) { p.AppID = "" }, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := validParams()
			tc.mutate(&params)
			_, err := a.Mint(params)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Mint(%s) error = %v, want wrapping %v", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestMint_ValidRequestSucceeds(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	a := New(priv)
	if _, err := a.Mint(validParams()); err != nil {
		t.Fatalf("Mint(valid params) unexpected error: %v", err)
	}
}
