// Command aivo-inventory is inventory's own service (split-inventory-microservice):
// its REST API (unchanged endpoint shapes, now JWT-authenticated instead
// of session-cookie) on REST_PORT, and its gRPC inbound
// (InventoryService.HandleTicketClosed, the pos→inventory edge) on
// GRPC_PORT. It owns a dedicated Postgres schema ("inventory") on the
// same shared instance cmd/aivo-server uses (design.md D1) and runs its
// own outbox Poller, delivering every inventory→ledger GL posting to
// ledger's gRPC listener (LEDGER_GRPC_ADDR) — no in-process ledger call.
//
// Env: DATABASE_URL (required), AUTH_PUBLIC_KEY or AUTH_PUBLIC_KEY_PATH
// (required — the aivo-auth Ed25519 public key, base64-encoded 32 bytes),
// AUTH_ISSUER (default "aivo-auth"), REST_PORT (default 8081), GRPC_PORT
// (default 9081), LEDGER_GRPC_ADDR (default "localhost:9080").
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"aivo/internal/inventory/adapters/grpcserver"
	inventoryhttp "aivo/internal/inventory/adapters/http"
	"aivo/internal/inventory/adapters/jwtauth"
	"aivo/internal/inventory/adapters/ledgerclient"
	inventorypg "aivo/internal/inventory/adapters/postgres"
	inventoryapp "aivo/internal/inventory/app"
	inventoryv1 "aivo/internal/inventory/v1"
	"aivo/internal/pos/adapters/salesreader"
	"aivo/migrations"
	"aivo/pkg/migrate"
	"aivo/pkg/outbox"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("inventory: DATABASE_URL is required")
	}
	publicKey, err := authPublicKey()
	if err != nil {
		return err
	}
	verifier, err := jwtauth.NewVerifier(publicKey, envDefault("AUTH_ISSUER", "aivo-auth"))
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	restPort := envDefault("REST_PORT", "8081")
	grpcPort := envDefault("GRPC_PORT", "9081")
	ledgerAddr := envDefault("LEDGER_GRPC_ADDR", "localhost:9080")

	db, err := inventorypg.OpenSchemaDB(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("inventory: ping db: %w", err)
	}

	// EnsureSchema is explicit (schema-qualified — always resolves
	// correctly regardless of search_path), run once before migrations so
	// migrate.Apply's own bookkeeping table, and every unqualified
	// CREATE TABLE in migrations/inventory/*.up.sql, land inside the
	// "inventory" schema rather than silently falling through to "public"
	// (see the report's migration-decision writeup).
	if err := inventorypg.EnsureSchema(context.Background(), db); err != nil {
		return fmt.Errorf("inventory: create schema: %w", err)
	}
	if err := migrate.Apply(context.Background(), db, []migrate.Source{
		{Name: "inventory", FS: migrations.FS, Dir: "inventory"},
	}); err != nil {
		return err
	}

	store := inventorypg.NewStore(db)
	app := inventoryapp.New(store, salesreader.New(db))

	ledgerConn, err := grpc.NewClient(ledgerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("inventory: dial ledger at %s: %w", ledgerAddr, err)
	}
	defer ledgerConn.Close()

	poller := &outbox.Poller{DB: db, Deliver: ledgerclient.New(ledgerConn)}
	pollerCtx, stopPoller := context.WithCancel(context.Background())
	defer stopPoller()
	go poller.Run(pollerCtx)

	grpcServer := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(grpcServer, grpcserver.New(app))
	grpcLis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		return fmt.Errorf("inventory: grpc listen: %w", err)
	}
	go func() {
		log.Printf("inventory: grpc listening on :%s", grpcPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Printf("inventory: grpc server: %v", err)
		}
	}()
	defer grpcServer.GracefulStop()

	mux := inventoryhttp.NewMux(app, verifier)
	addr := ":" + restPort
	log.Printf("inventory: rest listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// authPublicKey reads the aivo-auth Ed25519 public key from AUTH_PUBLIC_KEY
// (base64, inline) or AUTH_PUBLIC_KEY_PATH (a file containing the same),
// mirroring cmd/aivo-server's TOKEN_ENCRYPTION_KEY base64 convention.
func authPublicKey() ([]byte, error) {
	raw := os.Getenv("AUTH_PUBLIC_KEY")
	if raw == "" {
		path := os.Getenv("AUTH_PUBLIC_KEY_PATH")
		if path == "" {
			return nil, fmt.Errorf("inventory: AUTH_PUBLIC_KEY or AUTH_PUBLIC_KEY_PATH is required")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("inventory: read AUTH_PUBLIC_KEY_PATH: %w", err)
		}
		raw = strings.TrimSpace(string(b))
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("inventory: AUTH_PUBLIC_KEY: decode base64: %w", err)
	}
	return key, nil
}

func envDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
