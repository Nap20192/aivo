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
	"os"
	"path/filepath"
	"strings"

	menuhttp "aivo/internal/menu/adapters/http"
	menupg "aivo/internal/menu/adapters/postgres"
	"aivo/internal/menu/adapters/telegram"
	menuapp "aivo/internal/menu/app"
	"aivo/internal/platform/adapters/billing"
	platformhttp "aivo/internal/platform/adapters/http"
	platformpg "aivo/internal/platform/adapters/postgres"
	"aivo/internal/platform/adapters/s3"
	platformapp "aivo/internal/platform/app"
	platformports "aivo/internal/platform/ports"
	"aivo/internal/pos/adapters/menubridge"
	pospg "aivo/internal/pos/adapters/postgres"
	posapp "aivo/internal/pos/app"
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
		{Name: "menu", FS: menupg.MigrationsFS, Dir: "migrations"},
		{Name: "platform", FS: platformpg.MigrationsFS, Dir: "migrations"},
		{Name: "pos", FS: pospg.MigrationsFS, Dir: "migrations"},
	})
	if err != nil {
		return err
	}

	// Stores + apps, one shared pool.
	menuStore := menupg.NewPostgresStoreFromDB(db)
	platformStore := platformpg.NewStore(db)
	posStore := pospg.NewStore(db)

	menuApplication := menuapp.NewApplication(menuStore, telegram.New(), key, baseURL)
	platformApplication := platformapp.New(platformStore, billing.NewFake())
	posApplication := posapp.New(posStore, menubridge.New(menuStore))

	var images platformports.ImageStore
	if ep := os.Getenv("S3_ENDPOINT"); ep != "" {
		images, err = s3.New(ep,
			os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"),
			envDefault("S3_BUCKET", "aivo-menu-images"),
			envDefault("S3_PUBLIC_URL", "http://localhost:9000"),
			os.Getenv("S3_USE_SSL") == "true")
		if err != nil {
			return err
		}
	} else {
		log.Print("server: S3_ENDPOINT not set, image uploads disabled")
	}

	apiV1 := platformhttp.NewMux(platformhttp.Deps{
		Platform:  platformApplication,
		Pos:       posApplication,
		Menu:      menuStore,
		MenuAdmin: menuStore,
		MenuApp:   menuApplication,
		Images:    images,
		BaseURL:   baseURL,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", apiV1)
	// Legacy menu API paths (existing diner SPA still calls these).
	mux.Handle("/api/", menuhttp.NewMux(menuApplication))
	mux.Handle("/admin/", http.StripPrefix("/admin", spaFileServer("web/admin/dist")))
	mux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("/pos/", http.StripPrefix("/pos", spaFileServer("web/pos/dist")))
	mux.Handle("/pos", http.RedirectHandler("/pos/", http.StatusMovedPermanently))
	// Tenant routes (/{slug}, /{slug}/menu, /{slug}/t/{token}) are
	// client-side routes of the menu SPA.
	mux.Handle("/", spaFileServer(menuSPADir()))

	root := customDomainMiddleware(platformStore, menuStore, mux)

	addr := ":" + port
	log.Printf("server: listening on %s", addr)
	return http.ListenAndServe(addr, root)
}

// customDomainMiddleware resolves a verified custom domain from the Host
// header to its restaurant and rewrites the path onto the slug route, so
// menu.example.com/ serves what aivo.example/{slug} serves. Falls through
// to slug routing for unknown hosts. Certificate automation is out of
// scope for v1 — this only handles routing.
func customDomainMiddleware(platformStore *platformpg.Store, menuStore *menupg.PostgresStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host != "" && !strings.HasPrefix(r.URL.Path, "/api/") &&
			!strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/pos") {
			if restaurantID, err := platformStore.RestaurantIDByDomain(r.Context(), host); err == nil {
				if rest, err := menuStore.RestaurantByID(r.Context(), restaurantID); err == nil {
					r2 := r.Clone(r.Context())
					r2.URL.Path = "/" + rest.Slug + r.URL.Path
					next.ServeHTTP(w, r2)
					return
				}
			} else if !errors.Is(err, platformports.ErrNotFound) {
				log.Printf("server: custom domain lookup: %v", err)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// menuSPADir picks the diner menu SPA directory: the menu teammate's
// Vite build if present, else the legacy static site.
func menuSPADir() string {
	for _, dir := range []string{"web/menu-app/dist", "web/menu/dist", "web/menu"} {
		if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
			return dir
		}
	}
	return "web/menu"
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
