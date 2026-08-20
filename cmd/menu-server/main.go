// Command server runs the Menu app: the REST/JSON API
// (internal/menu/adapters/http, backed by internal/menu/app) plus the
// static diner-facing frontend (web/menu/), as one process. Reads
// DATABASE_URL, TOKEN_ENCRYPTION_KEY (base64), BASE_URL, and PORT from env.
//
// ponytail: menu talks to nothing internally yet, so no gRPC/proto layer.
// Add one when a second service needs to call into this module's domain
// logic (see internal/menu/docs/adr/0001-per-restaurant-telegram-bot.md's
// sibling, root docs/adr/0001-shared-grpc-gateway-in-front-of-core.md).
package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	menuhttp "aivo/internal/menu/adapters/http"
	"aivo/internal/menu/adapters/postgres"
	"aivo/internal/menu/adapters/telegram"
	"aivo/internal/menu/app"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("server: DATABASE_URL is required")
	}
	key, err := encryptionKey()
	if err != nil {
		return err
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	st, err := postgres.NewPostgresStore(dsn)
	if err != nil {
		return fmt.Errorf("server: open store: %w", err)
	}

	application := app.NewApplication(st, telegram.New(), key, baseURL)

	mux := http.NewServeMux()
	mux.Handle("/api/", menuhttp.NewMux(application))
	mux.Handle("/", spaFileServer("web/menu"))

	addr := ":" + port
	log.Printf("server: listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// spaFileServer serves static files under dir, falling back to
// dir/index.html for any request that doesn't match a real file — the
// diner table-link path (/{slug}/t/{token}) is a client-side route
// parsed by web/app.js, not a file on disk.
func spaFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, filepath.Clean(r.URL.Path))); err != nil {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

// encryptionKey reads TOKEN_ENCRYPTION_KEY as base64-encoded bytes, same
// convention as cmd/seed.
func encryptionKey() ([]byte, error) {
	raw := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if raw == "" {
		return nil, fmt.Errorf("server: TOKEN_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("server: TOKEN_ENCRYPTION_KEY: decode base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("server: TOKEN_ENCRYPTION_KEY: want 32 bytes (AES-256) after base64 decode, got %d", len(key))
	}
	return key, nil
}
