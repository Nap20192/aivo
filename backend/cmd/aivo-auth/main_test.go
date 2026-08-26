package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"aivo/pkg/tokenauth"
)

func TestPrivateKeyFromSeed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	seedB64 := base64.StdEncoding.EncodeToString(seed)

	t.Run("valid seed is deterministic", func(t *testing.T) {
		k1, err := privateKeyFromSeed(seedB64)
		if err != nil {
			t.Fatalf("privateKeyFromSeed: %v", err)
		}
		k2, err := privateKeyFromSeed(seedB64)
		if err != nil {
			t.Fatalf("privateKeyFromSeed: %v", err)
		}
		if !k1.Equal(k2) {
			t.Fatal("same seed produced different keys")
		}
	})

	t.Run("not base64 errors", func(t *testing.T) {
		if _, err := privateKeyFromSeed("not-base64!!"); err == nil {
			t.Fatal("expected error for non-base64 seed")
		}
	})

	t.Run("wrong length errors", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString([]byte("too-short"))
		if _, err := privateKeyFromSeed(short); err == nil {
			t.Fatal("expected error for wrong-length seed")
		}
	})
}

func TestLoadOrGeneratePrivateKey(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(2 * i)
	}
	seedB64 := base64.StdEncoding.EncodeToString(seed)
	want := ed25519.NewKeyFromSeed(seed)

	t.Run("inline seed wins, not ephemeral", func(t *testing.T) {
		priv, ephemeral, err := loadOrGeneratePrivateKey(seedB64, "")
		if err != nil {
			t.Fatalf("loadOrGeneratePrivateKey: %v", err)
		}
		if ephemeral {
			t.Fatal("expected ephemeral=false when a seed is configured")
		}
		if !priv.Equal(want) {
			t.Fatal("private key does not match the configured seed")
		}
	})

	t.Run("file path, not ephemeral", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "seed")
		if err := os.WriteFile(path, []byte(seedB64+"\n"), 0o600); err != nil {
			t.Fatalf("write seed file: %v", err)
		}
		priv, ephemeral, err := loadOrGeneratePrivateKey("", path)
		if err != nil {
			t.Fatalf("loadOrGeneratePrivateKey: %v", err)
		}
		if ephemeral {
			t.Fatal("expected ephemeral=false when a key file is configured")
		}
		if !priv.Equal(want) {
			t.Fatal("private key does not match the seed file")
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		if _, _, err := loadOrGeneratePrivateKey("", filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("expected error for missing AUTH_PRIVATE_KEY_FILE")
		}
	})

	t.Run("neither set generates an ephemeral, distinct key each call", func(t *testing.T) {
		priv1, ephemeral1, err := loadOrGeneratePrivateKey("", "")
		if err != nil {
			t.Fatalf("loadOrGeneratePrivateKey: %v", err)
		}
		if !ephemeral1 {
			t.Fatal("expected ephemeral=true when neither is set")
		}
		priv2, _, err := loadOrGeneratePrivateKey("", "")
		if err != nil {
			t.Fatalf("loadOrGeneratePrivateKey: %v", err)
		}
		if priv1.Equal(priv2) {
			t.Fatal("expected two ephemeral generations to differ")
		}
	})
}

func TestWritePublicKey(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	path := filepath.Join(t.TempDir(), "pub.key")

	if err := writePublicKey(path, pub); err != nil {
		t.Fatalf("writePublicKey: %v", err)
	}
	got, err := tokenauth.LoadPublicKey(path)
	if err != nil {
		t.Fatalf("tokenauth.LoadPublicKey(written file): %v", err)
	}
	if !got.Equal(pub) {
		t.Fatal("loaded public key does not match the one written")
	}
}

func TestEnvDefault(t *testing.T) {
	t.Run("unset returns default", func(t *testing.T) {
		os.Unsetenv("AIVO_AUTH_TEST_VAR")
		if got := envDefault("AIVO_AUTH_TEST_VAR", "fallback"); got != "fallback" {
			t.Errorf("envDefault = %q, want %q", got, "fallback")
		}
	})

	t.Run("set overrides default", func(t *testing.T) {
		t.Setenv("AIVO_AUTH_TEST_VAR", "custom")
		if got := envDefault("AIVO_AUTH_TEST_VAR", "fallback"); got != "custom" {
			t.Errorf("envDefault = %q, want %q", got, "custom")
		}
	})
}
