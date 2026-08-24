// Command aivo-server is the AIVO platform binary: the /api/v1 platform,
// POS, and diner APIs, the legacy menu API, and the static SPAs
// (/admin, /pos, tenant menu routes), one process. Applies embedded SQL
// migrations on startup.
//
// Env: DATABASE_URL (required), TOKEN_ENCRYPTION_KEY (required, base64
// 32 bytes), BASE_URL, PORT, and optionally S3_ENDPOINT / S3_ACCESS_KEY /
// S3_SECRET_KEY / S3_BUCKET / S3_PUBLIC_URL for image uploads (upload
// endpoint answers 503 when unset).
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	inventoryledgerbridge "aivo/internal/inventory/adapters/ledgerbridge"
	inventorypg "aivo/internal/inventory/adapters/postgres"
	inventoryapp "aivo/internal/inventory/app"
	ledgerpg "aivo/internal/ledger/adapters/postgres"
	ledgerapp "aivo/internal/ledger/app"
	menupg "aivo/internal/menu/adapters/postgres"
	"aivo/internal/menu/adapters/telegram"
	menuapp "aivo/internal/menu/app"
	"aivo/internal/platform/adapters/billing"
	"aivo/internal/platform/adapters/claudecli"
	platformhttp "aivo/internal/platform/adapters/http"
	platformpg "aivo/internal/platform/adapters/postgres"
	"aivo/internal/platform/adapters/s3"
	platformapp "aivo/internal/platform/app"
	platformports "aivo/internal/platform/ports"
	"aivo/internal/pos/adapters/inventorybridge"
	"aivo/internal/pos/adapters/ledgerbridge"
	"aivo/internal/pos/adapters/menubridge"
	pospg "aivo/internal/pos/adapters/postgres"
	"aivo/internal/pos/adapters/salesreader"
	posapp "aivo/internal/pos/app"
	"aivo/internal/provisioning"
	"aivo/migrations"
	"aivo/pkg/migrate"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
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
	baseURL := envDefault("BASE_URL", "http://localhost:8080")
	port := envDefault("PORT", "8080")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("server: open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("server: ping db: %w", err)
	}

	// Migrations: menu first (owns restaurants), then platform (extends
	// it), then pos (references both).
	err = migrate.Apply(context.Background(), db, []migrate.Source{
		{Name: "menu", FS: migrations.FS, Dir: "menu"},
		{Name: "platform", FS: migrations.FS, Dir: "platform"},
		{Name: "ledger", FS: migrations.FS, Dir: "ledger"},
		{Name: "pos", FS: migrations.FS, Dir: "pos"},
		{Name: "inventory", FS: migrations.FS, Dir: "inventory"},
	})
	if err != nil {
		return err
	}

	// Stores + apps, one shared pool.
	menuStore := menupg.NewPostgresStoreFromDB(db)
	platformStore := platformpg.NewStore(db)
	posStore := pospg.NewStore(db)
	ledgerStore := ledgerpg.NewStore(db)

	menuApplication := menuapp.NewApplication(menuStore, telegram.New(), key, baseURL)

	// AI theme generation: opt-in via THEME_GENERATOR=claudecli. Off by
	// default so prod without the CLI fails clean (endpoint answers 503).
	var themeGen platformports.ThemeGenerator
	switch gen := os.Getenv("THEME_GENERATOR"); gen {
	case "claudecli":
		themeGen = claudecli.New(os.Getenv("CLAUDE_BIN"))
		log.Print("server: theme generator: claudecli")
	case "":
		log.Print("server: THEME_GENERATOR not set, theme generation disabled")
	default:
		return fmt.Errorf("server: unknown THEME_GENERATOR %q (want claudecli)", gen)
	}

	platformApplication := platformapp.New(platformStore, billing.NewFake(), themeGen)
	ledgerApplication := ledgerapp.New(ledgerStore)
	// Inventory posts to the GL via the ledger bridge and reads pos sales
	// for the food-cost report (pos → inventory → ledger; no cycles).
	inventoryApplication := inventoryapp.New(
		inventorypg.NewStore(db),
		inventoryledgerbridge.New(ledgerApplication),
		salesreader.New(db),
	)
	posApplication := posapp.New(posStore, menubridge.New(menuStore), ledgerbridge.New(ledgerApplication), inventorybridge.New(inventoryApplication))
	// Live restaurant provisioning seeds the GL + payment methods in the
	// same transaction as the restaurant row (M3 / BUG-1).
	platformStore.OnProvisionRestaurant = provisioning.RestaurantProvisioner(ledgerApplication)

	var images platformports.ImageStore
	imagePrefix := ""
	if ep := os.Getenv("S3_ENDPOINT"); ep != "" {
		bucket := envDefault("S3_BUCKET", "aivo-menu-images")
		publicURL := envDefault("S3_PUBLIC_URL", "http://localhost:9000")
		images, err = s3.New(ep,
			os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"),
			bucket, publicURL, os.Getenv("S3_USE_SSL") == "true")
		if err != nil {
			return err
		}
		imagePrefix = strings.TrimRight(publicURL, "/") + "/" + bucket + "/"
	} else {
		log.Print("server: S3_ENDPOINT not set, image uploads disabled")
	}

	// Admin AI assistant: opt-in via ASSISTANT=claudecli (same CLI and
	// CLAUDE_BIN as the theme generator).
	var assistant platformports.Assistant
	switch os.Getenv("ASSISTANT") {
	case "claudecli":
		assistant = claudecli.NewAssistant(os.Getenv("CLAUDE_BIN"))
		log.Print("server: assistant: claudecli")
	case "":
		log.Print("server: ASSISTANT not set, admin assistant disabled")
	default:
		return fmt.Errorf("server: unknown ASSISTANT %q (want claudecli)", os.Getenv("ASSISTANT"))
	}

	// POS display timezone (RESTAURANT_TZ, e.g. "Europe/Brussels");
	// unset = server-local.
	var posLocation *time.Location
	if tz := os.Getenv("RESTAURANT_TZ"); tz != "" {
		posLocation, err = time.LoadLocation(tz)
		if err != nil {
			return fmt.Errorf("server: RESTAURANT_TZ: %w", err)
		}
	}

	apiV1 := platformhttp.NewMux(platformhttp.Deps{
		Platform:       platformApplication,
		Pos:            posApplication,
		Ledger:         ledgerApplication,
		Inventory:      inventoryApplication,
		Menu:           menuStore,
		MenuAdmin:      menuStore,
		MenuApp:        menuApplication,
		Images:         images,
		Assistant:      assistant,
		AssistantStore: platformStore,
		ImagePrefix:    imagePrefix,
		BaseURL:        baseURL,
		POSLocation:    posLocation,
	})

	// The frontend tree sits next to the binary's cwd in Docker
	// (WORKDIR /, /frontend) and one level up when running natively from
	// backend/ (go run ./cmd/aivo-server).
	frontendDir := "frontend"
	if _, err := os.Stat(frontendDir); err != nil {
		if _, err := os.Stat("../frontend"); err == nil {
			frontendDir = "../frontend"
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", apiV1)
	// Unknown /api/* must 404, not fall through to the tenant SPA catch-all.
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/admin/", http.StripPrefix("/admin", spaFileServer(frontendDir+"/admin/dist")))
	mux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("/pos/", http.StripPrefix("/pos", spaFileServer(frontendDir+"/pos/dist")))
	mux.Handle("/pos", http.RedirectHandler("/pos/", http.StatusMovedPermanently))
	// Tenant routes (/{slug}, /{slug}/menu, /{slug}/t/{token},
	// /{slug}/m/{menu}) are client-side routes of the menu SPA — served
	// from its Vite build only (the Dockerfile builds it; 503 until then).
	mux.Handle("/", spaFileServer(frontendDir+"/menu/dist"))

	root := customDomainMiddleware(platformStore, baseURL, mux)

	addr := ":" + port
	log.Printf("server: listening on %s", addr)
	return http.ListenAndServe(addr, root)
}

// customDomainMiddleware maps a verified custom domain to a 302 redirect
// onto the canonical BASE_URL/{slug}{path} — a server-side path rewrite
// is invisible to the SPA's location.pathname parsing (and mangled Vite
// /assets/* paths); one canonical URL space avoids all of that.
// Lookups are cached in-memory for a minute so assets don't hammer the
// DB. Certificate automation stays out of scope for v1.
func customDomainMiddleware(platformStore *platformpg.Store, baseURL string, next http.Handler) http.Handler {
	baseHost := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		baseHost = u.Hostname()
	}

	type cacheEntry struct {
		slug  string // "" = no verified domain
		until time.Time
	}
	var mu sync.Mutex
	cache := map[string]cacheEntry{}
	const cacheTTL = time.Minute

	slugFor := func(r *http.Request, host string) string {
		mu.Lock()
		if e, ok := cache[host]; ok && time.Now().Before(e.until) {
			mu.Unlock()
			return e.slug
		}
		mu.Unlock()

		slug := ""
		if restaurantID, err := platformStore.RestaurantIDByDomain(r.Context(), host); err == nil {
			if rest, err := platformStore.RestaurantByID(r.Context(), restaurantID); err == nil {
				slug = rest.Slug
			}
		} else if !errors.Is(err, platformports.ErrNotFound) {
			log.Printf("server: custom domain lookup: %v", err)
			return "" // don't cache transient DB errors
		}
		mu.Lock()
		cache[host] = cacheEntry{slug: slug, until: time.Now().Add(cacheTTL)}
		mu.Unlock()
		return slug
	}

	reserved := func(path string) bool {
		return strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/admin") ||
			strings.HasPrefix(path, "/pos") || strings.HasPrefix(path, "/assets/")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host != "" && host != baseHost &&
			(r.Method == http.MethodGet || r.Method == http.MethodHead) && !reserved(r.URL.Path) {
			if slug := slugFor(r, host); slug != "" {
				http.Redirect(w, r, strings.TrimRight(baseURL, "/")+"/"+slug+r.URL.Path, http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// spaFileServer serves static files under dir, falling back to
// dir/index.html for client-side routes. If the dir (or its index.html)
// is missing — SPA not built yet — it answers 503 instead of a confusing
// 404, checked per request so a later build is picked up without a
// restart.
func spaFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "frontend not built: missing "+index, http.StatusServiceUnavailable)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.Clean(r.URL.Path))); err != nil {
			http.ServeFile(w, r, index)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func envDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// encryptionKey reads TOKEN_ENCRYPTION_KEY as base64-encoded bytes, same
// convention as cmd/aivo-seed.
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
