package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvDefault(t *testing.T) {
	t.Setenv("AIVO_INVENTORY_TEST_VAR", "")
	if got := envDefault("AIVO_INVENTORY_TEST_VAR", "fallback"); got != "fallback" {
		t.Errorf("envDefault(unset) = %q, want fallback", got)
	}
	t.Setenv("AIVO_INVENTORY_TEST_VAR", "set")
	if got := envDefault("AIVO_INVENTORY_TEST_VAR", "fallback"); got != "set" {
		t.Errorf("envDefault(set) = %q, want set", got)
	}
}

func TestAuthPublicKey_Inline(t *testing.T) {
	key := make([]byte, 32)
	encoded := base64.StdEncoding.EncodeToString(key)
	t.Setenv("AUTH_PUBLIC_KEY", encoded)
	t.Setenv("AUTH_PUBLIC_KEY_PATH", "")

	got, err := authPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Errorf("authPublicKey() len = %d, want 32", len(got))
	}
}

func TestAuthPublicKey_FromFile(t *testing.T) {
	key := make([]byte, 32)
	encoded := base64.StdEncoding.EncodeToString(key)
	path := filepath.Join(t.TempDir(), "pub.key")
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_PUBLIC_KEY", "")
	t.Setenv("AUTH_PUBLIC_KEY_PATH", path)

	got, err := authPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Errorf("authPublicKey() len = %d, want 32", len(got))
	}
}

func TestAuthPublicKey_Missing(t *testing.T) {
	t.Setenv("AUTH_PUBLIC_KEY", "")
	t.Setenv("AUTH_PUBLIC_KEY_PATH", "")
	if _, err := authPublicKey(); err == nil {
		t.Error("authPublicKey() with neither env set: error = nil, want an error")
	}
}

func TestAuthPublicKey_BadBase64(t *testing.T) {
	t.Setenv("AUTH_PUBLIC_KEY", "not-valid-base64!!!")
	t.Setenv("AUTH_PUBLIC_KEY_PATH", "")
	if _, err := authPublicKey(); err == nil {
		t.Error("authPublicKey(bad base64): error = nil, want an error")
	}
}

func TestAuthPublicKey_MissingFile(t *testing.T) {
	t.Setenv("AUTH_PUBLIC_KEY", "")
	t.Setenv("AUTH_PUBLIC_KEY_PATH", filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := authPublicKey(); err == nil {
		t.Error("authPublicKey(missing file): error = nil, want an error")
	}
}
