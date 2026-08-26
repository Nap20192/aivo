package tokenauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"uuid"
)

// sign builds a token the same way cmd/aivo-auth does, without importing
// internal/auth/app — keeps this test self-contained and lets it
// construct deliberately broken tokens the real signer never would.
func sign(t *testing.T, priv ed25519.PrivateKey, h header, wc wireClaims) string {
	t.Helper()
	headerRaw, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadRaw, err := json.Marshal(wc)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerRaw)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadRaw)
	sig := ed25519.Sign(priv, []byte(headerB64+"."+payloadB64))
	return headerB64 + "." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validClaims(userID, tenantID uuid.UUID) wireClaims {
	now := time.Now()
	return wireClaims{
		UserID:   userID.String(),
		TenantID: tenantID.String(),
		Roles:    []string{"owner"},
		AppID:    "admin",
		Iat:      now.Unix(),
		Exp:      now.Add(time.Hour).Unix(),
		Iss:      Issuer,
	}
}

func TestVerify_Valid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	userID, tenantID := uuid.New(), uuid.New()
	wc := validClaims(userID, tenantID)
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, wc)

	claims, err := Verify(pub, token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
	if claims.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", claims.TenantID, tenantID)
	}
	if claims.AppID != "admin" {
		t.Errorf("AppID = %q, want %q", claims.AppID, "admin")
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "owner" {
		t.Errorf("Roles = %v, want [owner]", claims.Roles)
	}
	if claims.Issuer != Issuer {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, Issuer)
	}
}

func TestVerify_TenantsAreDistinguishable(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	userID := uuid.New()
	tenantA, tenantB := uuid.New(), uuid.New()

	tokenA := sign(t, priv, header{Alg: alg, Typ: "JWT"}, validClaims(userID, tenantA))
	tokenB := sign(t, priv, header{Alg: alg, Typ: "JWT"}, validClaims(userID, tenantB))

	claimsA, err := Verify(pub, tokenA)
	if err != nil {
		t.Fatalf("Verify tokenA: %v", err)
	}
	claimsB, err := Verify(pub, tokenB)
	if err != nil {
		t.Fatalf("Verify tokenB: %v", err)
	}
	if claimsA.TenantID == claimsB.TenantID {
		t.Fatal("expected distinct tenant IDs to remain distinguishable after verification")
	}
	if claimsA.TenantID != tenantA || claimsB.TenantID != tenantB {
		t.Fatal("verified tenant ID does not match the tenant each token was minted for")
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, validClaims(uuid.New(), uuid.New()))

	// Re-sign a payload for a different (higher-privilege) tenant using
	// the token's own signature — i.e. splice in a forged payload
	// segment without access to the private key.
	parts := splitToken(t, token)
	forged := wireClaims{
		UserID:   uuid.New().String(),
		TenantID: uuid.New().String(),
		Roles:    []string{"owner"},
		AppID:    "admin",
		Iat:      time.Now().Unix(),
		Exp:      time.Now().Add(time.Hour).Unix(),
		Iss:      Issuer,
	}
	forgedRaw, _ := json.Marshal(forged)
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forgedRaw) + "." + parts[2]

	if _, err := Verify(pub, tampered); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify(tampered payload) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerify_TamperedSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, validClaims(uuid.New(), uuid.New()))
	parts := splitToken(t, token)

	sigRaw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sigRaw[0] ^= 0xFF
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sigRaw)

	if _, err := Verify(pub, tampered); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify(tampered signature) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerify_WrongPublicKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, validClaims(uuid.New(), uuid.New()))

	if _, err := Verify(otherPub, token); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify(wrong pubkey) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	wc := validClaims(uuid.New(), uuid.New())
	wc.Exp = time.Now().Add(-time.Minute).Unix()
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, wc)

	if _, err := Verify(pub, token); !errors.Is(err, ErrExpired) {
		t.Fatalf("Verify(expired) = %v, want ErrExpired", err)
	}
}

func TestVerify_WrongIssuer(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	wc := validClaims(uuid.New(), uuid.New())
	wc.Iss = "someone-else"
	token := sign(t, priv, header{Alg: alg, Typ: "JWT"}, wc)

	if _, err := Verify(pub, token); !errors.Is(err, ErrInvalidIssuer) {
		t.Fatalf("Verify(wrong issuer) = %v, want ErrInvalidIssuer", err)
	}
}

func TestVerify_Malformed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	validWC := validClaims(uuid.New(), uuid.New())

	cases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"not three segments", "a.b"},
		{"too many segments", "a.b.c.d"},
		{"header not base64", "not-base64!!.b.c"},
		{"payload not base64", func() string {
			parts := splitToken(t, sign(t, priv, header{Alg: alg, Typ: "JWT"}, validWC))
			return parts[0] + ".not-base64!!." + parts[2]
		}()},
		{"signature not base64", func() string {
			parts := splitToken(t, sign(t, priv, header{Alg: alg, Typ: "JWT"}, validWC))
			return parts[0] + "." + parts[1] + ".not-base64!!"
		}()},
		{"header not JSON", base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".b.c"},
		{"unknown alg", sign(t, priv, header{Alg: "HS256", Typ: "JWT"}, validWC)},
		{"payload not JSON", func() string {
			parts := splitToken(t, sign(t, priv, header{Alg: alg, Typ: "JWT"}, validWC))
			badPayload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
			sig := ed25519.Sign(priv, []byte(parts[0]+"."+badPayload))
			return parts[0] + "." + badPayload + "." + base64.RawURLEncoding.EncodeToString(sig)
		}()},
		{"bad user_id UUID", func() string {
			wc := validWC
			wc.UserID = "not-a-uuid"
			return sign(t, priv, header{Alg: alg, Typ: "JWT"}, wc)
		}()},
		{"bad tenant_id UUID", func() string {
			wc := validWC
			wc.TenantID = "not-a-uuid"
			return sign(t, priv, header{Alg: alg, Typ: "JWT"}, wc)
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Verify(pub, tc.token); !errors.Is(err, ErrMalformed) {
				t.Fatalf("Verify(%q) = %v, want ErrMalformed", tc.name, err)
			}
		})
	}
}

func splitToken(t *testing.T, token string) [3]string {
	t.Helper()
	var out [3]string
	n := 0
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			out[n] = token[start:i]
			n++
			start = i + 1
		}
	}
	out[n] = token[start:]
	return out
}

func TestLoadPublicKey(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()

	t.Run("valid file round-trips the key", func(t *testing.T) {
		path := filepath.Join(dir, "pub.key")
		if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := LoadPublicKey(path)
		if err != nil {
			t.Fatalf("LoadPublicKey: %v", err)
		}
		if !got.Equal(pub) {
			t.Fatal("loaded key does not match written key")
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		if _, err := LoadPublicKey(filepath.Join(dir, "missing")); err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("not base64 errors", func(t *testing.T) {
		path := filepath.Join(dir, "bad-b64.key")
		if err := os.WriteFile(path, []byte("not-base64!!"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := LoadPublicKey(path); err == nil {
			t.Fatal("expected error for non-base64 content")
		}
	})

	t.Run("wrong length errors", func(t *testing.T) {
		path := filepath.Join(dir, "wrong-len.key")
		if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString([]byte("too-short"))), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := LoadPublicKey(path); err == nil {
			t.Fatal("expected error for wrong-length key")
		}
	})
}
