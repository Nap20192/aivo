package jwtauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"uuid"
)

// mint builds a compact EdDSA JWT for tests, mirroring the wire format
// Verify parses — svc-auth's real Mint implementation is out of this
// worktree's scope.
func mint(t *testing.T, priv ed25519.PrivateKey, p jwtPayload) string {
	t.Helper()
	h, err := json.Marshal(jwtHeader{Alg: "EdDSA", Typ: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(body)
	sig := ed25519.Sign(priv, []byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validPayload() jwtPayload {
	return jwtPayload{
		Sub: uuid.New().String(), TenantID: uuid.New().String(),
		Roles: []string{"manager"}, AppID: "admin",
		Exp: time.Now().Add(time.Hour).Unix(), Iss: "aivo-auth",
	}
}

func TestVerify_Valid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub, Issuer: "aivo-auth"}
	claims, err := v.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.UserID.String() != p.Sub || claims.TenantID.String() != p.TenantID {
		t.Errorf("claims = %+v, want sub=%s tenant=%s", claims, p.Sub, p.TenantID)
	}
	if claims.AppID != "admin" || !claims.HasRole("manager") {
		t.Errorf("claims = %+v", claims)
	}
}

func TestVerify_TamperedSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token := mint(t, priv, validPayload())
	tampered := token[:len(token)-4] + "abcd"

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(tampered); err != ErrBadSignature {
		t.Errorf("tampered sig: err = %v, want ErrBadSignature", err)
	}
}

func TestVerify_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)
	token := mint(t, priv, validPayload())

	v := Verifier{PublicKey: otherPub}
	if _, err := v.Verify(token); err != ErrBadSignature {
		t.Errorf("wrong key: err = %v, want ErrBadSignature", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	p.Exp = time.Now().Add(-time.Minute).Unix()
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrExpired {
		t.Errorf("expired: err = %v, want ErrExpired", err)
	}
}

func TestVerify_MissingExp(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	p.Exp = 0
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrExpired {
		t.Errorf("missing exp: err = %v, want ErrExpired", err)
	}
}

func TestVerify_IssuerMismatch(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token := mint(t, priv, validPayload())

	v := Verifier{PublicKey: pub, Issuer: "someone-else"}
	if _, err := v.Verify(token); err != ErrIssuerMismatch {
		t.Errorf("issuer mismatch: err = %v, want ErrIssuerMismatch", err)
	}
}

func TestVerify_Malformed(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	v := Verifier{PublicKey: pub}
	cases := []string{
		"",
		"not-a-jwt",
		"a.b",
		"a.b.c.d",
		".b.c",
		"a..c",
		"a.b.",
	}
	for _, tok := range cases {
		if _, err := v.Verify(tok); err != ErrMalformed {
			t.Errorf("Verify(%q) err = %v, want ErrMalformed", tok, err)
		}
	}
}

func TestVerify_BadHeaderJSON(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	bad := base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".e30." + base64.RawURLEncoding.EncodeToString([]byte("sig"))
	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(bad); err != ErrMalformed {
		t.Errorf("bad header json: err = %v, want ErrMalformed", err)
	}
}

func TestVerify_UnsupportedAlg(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	h, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	body, _ := json.Marshal(validPayload())
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(body)
	sig := ed25519.Sign(priv, []byte(signed))
	token := signed + "." + base64.RawURLEncoding.EncodeToString(sig)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrUnsupportedAlg {
		t.Errorf("unsupported alg: err = %v, want ErrUnsupportedAlg", err)
	}
}

func TestVerify_BadPayloadEncoding(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	h, _ := json.Marshal(jwtHeader{Alg: "EdDSA", Typ: "JWT"})
	headerB64 := base64.RawURLEncoding.EncodeToString(h)
	signed := headerB64 + ".not-valid-base64!!!"
	sig := ed25519.Sign(priv, []byte(signed))
	token := signed + "." + base64.RawURLEncoding.EncodeToString(sig)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrMalformed && err != ErrBadSignature {
		t.Errorf("bad payload b64: err = %v", err)
	}
}

func TestVerify_BadPayloadJSON(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	h, _ := json.Marshal(jwtHeader{Alg: "EdDSA", Typ: "JWT"})
	headerB64 := base64.RawURLEncoding.EncodeToString(h)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	signed := headerB64 + "." + payloadB64
	sig := ed25519.Sign(priv, []byte(signed))
	token := signed + "." + base64.RawURLEncoding.EncodeToString(sig)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrMalformed {
		t.Errorf("bad payload json: err = %v, want ErrMalformed", err)
	}
}

func TestVerify_BadSubUUID(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	p.Sub = "not-a-uuid"
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrMalformed {
		t.Errorf("bad sub: err = %v, want ErrMalformed", err)
	}
}

func TestVerify_BadTenantUUID(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	p.TenantID = "not-a-uuid"
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != ErrMalformed {
		t.Errorf("bad tenant: err = %v, want ErrMalformed", err)
	}
}

func TestVerify_NoIssuerConfigured(t *testing.T) {
	// Empty Verifier.Issuer skips the issuer check entirely.
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := validPayload()
	p.Iss = "anything"
	token := mint(t, priv, p)

	v := Verifier{PublicKey: pub}
	if _, err := v.Verify(token); err != nil {
		t.Errorf("no issuer configured: err = %v, want nil", err)
	}
}

func TestNewVerifier(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	if _, err := NewVerifier(pub, "iss"); err != nil {
		t.Errorf("NewVerifier() error = %v", err)
	}
	if _, err := NewVerifier([]byte("too short"), "iss"); err != ErrInvalidPublicKey {
		t.Errorf("short key: err = %v, want ErrInvalidPublicKey", err)
	}
}

func TestClaims_HasRole(t *testing.T) {
	c := Claims{Roles: []string{"manager", "waiter"}}
	if !c.HasRole("owner", "manager") {
		t.Error("expected HasRole to find manager")
	}
	if c.HasRole("owner") {
		t.Error("expected HasRole(owner) to be false")
	}
	if (Claims{}).HasRole("manager") {
		t.Error("expected empty claims to have no roles")
	}
}
