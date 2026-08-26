// Command aivo-auth is a thin gRPC token-minting service: AuthService.Mint
// signs a JWT-shaped token (Ed25519, stdlib crypto/ed25519) for a user
// platform has already authenticated via its own session-cookie login.
// It owns no user/credential storage, has no HTTP surface, and no
// database — see openspec/changes/split-inventory-microservice/design.md
// (decision D5) and specs/service-auth/spec.md.
//
// Env: PORT (default 9082); one of AUTH_PRIVATE_KEY_SEED (base64 32-byte
// Ed25519 seed) or AUTH_PRIVATE_KEY_FILE (path to a file containing that
// same base64 seed) to run with a stable identity across restarts — if
// neither is set, a fresh key is generated at startup (fine for local
// dev; every previously minted token stops verifying on the next
// restart, and every other running aivo-auth-fronted process would need
// the new public key). AUTH_PUBLIC_KEY_FILE (optional) is where the
// derived public key is written (base64 of the raw 32 bytes) on every
// startup — point a downstream service's pkg/tokenauth.LoadPublicKey at
// this same path (e.g. a shared volume in docker-compose) to verify
// tokens this process mints.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	grpcadapter "aivo/internal/auth/adapters/grpc"
	"aivo/internal/auth/app"
	authv1 "aivo/internal/auth/v1"

	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	priv, ephemeral, err := loadOrGeneratePrivateKey(os.Getenv("AUTH_PRIVATE_KEY_SEED"), os.Getenv("AUTH_PRIVATE_KEY_FILE"))
	if err != nil {
		return err
	}
	if ephemeral {
		log.Print("aivo-auth: no AUTH_PRIVATE_KEY_SEED/AUTH_PRIVATE_KEY_FILE set, generated an ephemeral signing key — fine for local dev, but every minted token stops verifying on the next restart")
	}

	if path := os.Getenv("AUTH_PUBLIC_KEY_FILE"); path != "" {
		pub := priv.Public().(ed25519.PublicKey)
		if err := writePublicKey(path, pub); err != nil {
			return fmt.Errorf("aivo-auth: write public key: %w", err)
		}
		log.Printf("aivo-auth: public key written to %s", path)
	}

	port := envDefault("PORT", "9082")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("aivo-auth: listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcServer, grpcadapter.New(app.New(priv)))

	log.Printf("aivo-auth: listening on :%s", port)
	return grpcServer.Serve(lis)
}

// loadOrGeneratePrivateKey resolves the signing key from a base64 seed
// (inline via seedB64, or read from filePath), or generates a fresh one
// if neither is set — reporting which via the ephemeral return.
func loadOrGeneratePrivateKey(seedB64, filePath string) (priv ed25519.PrivateKey, ephemeral bool, err error) {
	switch {
	case seedB64 != "":
		priv, err = privateKeyFromSeed(seedB64)
		return priv, false, err
	case filePath != "":
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, false, fmt.Errorf("aivo-auth: AUTH_PRIVATE_KEY_FILE: %w", err)
		}
		priv, err = privateKeyFromSeed(strings.TrimSpace(string(raw)))
		return priv, false, err
	default:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, true, err
	}
}

func privateKeyFromSeed(seedB64 string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, fmt.Errorf("aivo-auth: private key seed: decode base64: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("aivo-auth: private key seed: want %d bytes after base64 decode, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func writePublicKey(path string, pub ed25519.PublicKey) error {
	return os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)
}

func envDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
