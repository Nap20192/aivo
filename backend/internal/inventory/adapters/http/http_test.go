package http

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	inv "aivo/internal/domain/inventory"
	"aivo/internal/inventory/adapters/jwtauth"
	inventorypg "aivo/internal/inventory/adapters/postgres"
	inventoryapp "aivo/internal/inventory/app"
	"aivo/internal/inventory/ports"
	"aivo/internal/pos/adapters/salesreader"
	"aivo/migrations"
	"aivo/pkg/migrate"

	_ "github.com/jackc/pgx/v5/stdlib"
	"uuid"
)

// mint builds a compact EdDSA JWT for tests — mirrors
// jwtauth_test.go's helper; svc-auth's real Mint is out of this
// worktree's scope, and this package tests the REST surface's
// consumption of a token, not minting.
func mint(t *testing.T, priv ed25519.PrivateKey, sub, tenant string, roles []string, exp time.Time) string {
	t.Helper()
	type header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	type payload struct {
		Sub      string   `json:"sub"`
		TenantID string   `json:"tenant_id"`
		Roles    []string `json:"roles"`
		AppID    string   `json:"app_id"`
		Exp      int64    `json:"exp"`
		Iss      string   `json:"iss"`
	}
	h, _ := json.Marshal(header{Alg: "EdDSA", Typ: "JWT"})
	b, _ := json.Marshal(payload{Sub: sub, TenantID: tenant, Roles: roles, AppID: "admin", Exp: exp.Unix(), Iss: "aivo-auth"})
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(b)
	sig := ed25519.Sign(priv, []byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func testVerifier(t *testing.T) (jwtauth.Verifier, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jwtauth.Verifier{PublicKey: pub, Issuer: "aivo-auth"}, priv
}

func TestRestaurantMiddleware_MissingToken(t *testing.T) {
	verifier, _ := testVerifier(t)
	mux := NewMux(nil, verifier)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+uuid.New().String()+"/inventory/products", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401", rec.Code)
	}
}

func TestRestaurantMiddleware_InvalidToken(t *testing.T) {
	verifier, _ := testVerifier(t)
	mux := NewMux(nil, verifier)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+uuid.New().String()+"/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: status = %d, want 401", rec.Code)
	}
}

func TestRestaurantMiddleware_ExpiredToken(t *testing.T) {
	verifier, priv := testVerifier(t)
	mux := NewMux(nil, verifier)
	restID := uuid.New()
	token := mint(t, priv, uuid.New().String(), restID.String(), []string{"manager"}, time.Now().Add(-time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+restID.String()+"/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expired token: status = %d, want 401", rec.Code)
	}
}

func TestRestaurantMiddleware_WrongTenant(t *testing.T) {
	verifier, priv := testVerifier(t)
	mux := NewMux(nil, verifier)
	tokenTenant := uuid.New()
	pathTenant := uuid.New() // different restaurant than the token's claim
	token := mint(t, priv, uuid.New().String(), tokenTenant.String(), []string{"manager"}, time.Now().Add(time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+pathTenant.String()+"/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("wrong tenant: status = %d, want 403", rec.Code)
	}
}

func TestRestaurantMiddleware_InsufficientRole(t *testing.T) {
	verifier, priv := testVerifier(t)
	mux := NewMux(nil, verifier)
	restID := uuid.New()
	token := mint(t, priv, uuid.New().String(), restID.String(), []string{"waiter"}, time.Now().Add(time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+restID.String()+"/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("waiter role: status = %d, want 403", rec.Code)
	}
}

func TestRestaurantMiddleware_BadPathID(t *testing.T) {
	verifier, priv := testVerifier(t)
	mux := NewMux(nil, verifier)
	token := mint(t, priv, uuid.New().String(), uuid.New().String(), []string{"manager"}, time.Now().Add(time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/not-a-uuid/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("bad path id: status = %d, want 404", rec.Code)
	}
}

func TestWriteAppErr(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0}, // handled separately below
		{ports.ErrNotFound, http.StatusNotFound},
		{inv.ErrUnitIncompatible, http.StatusUnprocessableEntity},
		{inv.ErrRecipeCycle, http.StatusUnprocessableEntity},
		{inv.ErrEmptyRecipe, http.StatusUnprocessableEntity},
		{inv.ErrDuplicateIngredient, http.StatusUnprocessableEntity},
		{inv.ErrEmptyDocument, http.StatusUnprocessableEntity},
		{inv.ErrInvalidType, http.StatusUnprocessableEntity},
		{inventoryapp.ErrSKUTaken, http.StatusUnprocessableEntity},
		{inventoryapp.ErrBackdated, http.StatusUnprocessableEntity},
		{inventoryapp.ErrMenuItemTaken, http.StatusConflict},
		{inventoryapp.ErrVersionExists, http.StatusConflict},
		{inventoryapp.ErrStocktakeOpen, http.StatusConflict},
		{inventoryapp.ErrSupplierNameTaken, http.StatusConflict},
		{inventoryapp.ErrAlreadyPosted, http.StatusConflict},
		{inventoryapp.ErrAlreadyCancelled, http.StatusConflict},
		{inventoryapp.ErrNotDraft, http.StatusConflict},
		{ports.ErrConflict, http.StatusConflict},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		if c.err == nil {
			if wrote := writeAppErr(rec, nil); wrote {
				t.Errorf("writeAppErr(nil) wrote a response, want none")
			}
			continue
		}
		if !writeAppErr(rec, c.err) {
			t.Errorf("writeAppErr(%v) = false, want true", c.err)
			continue
		}
		if rec.Code != c.want {
			t.Errorf("writeAppErr(%v) status = %d, want %d", c.err, rec.Code, c.want)
		}
	}
}

func TestDecodeJSON_BadBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("not json"))
	var v struct{}
	if decodeJSON(rec, req, &v) {
		t.Error("decodeJSON(bad json) = true, want false")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: status = %d, want 400", rec.Code)
	}
}

// --- DB-gated: a valid token reaches the handler and behaves like the
// pre-split REST surface ---------------------------------------------

func setupMux(t *testing.T) (mux http.Handler, priv ed25519.PrivateKey, restID, userID uuid.UUID, db *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	database, err := inventorypg.OpenSchemaDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()
	if err := inventorypg.EnsureSchema(ctx, database); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Apply(ctx, database, []migrate.Source{{Name: "inventory", FS: migrations.FS, Dir: "inventory"}}); err != nil {
		t.Fatal(err)
	}

	orgID, rest, user := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		if _, err := database.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'http-test-org')`, orgID)
	exec(`INSERT INTO users (id, org_id, email, password_hash, role) VALUES ($1, $2, $3, $4, 'owner')`,
		user, orgID, "u-"+uuid.New().String()[:8]+"@t", []byte("x"))
	exec(`INSERT INTO restaurants (id, org_id, slug, name) VALUES ($1, $2, $3, 'T')`, rest, orgID, "t-"+uuid.New().String()[:8])
	t.Cleanup(func() {
		bg := context.Background()
		database.ExecContext(bg, `DELETE FROM restaurants WHERE id = $1`, rest)
		database.ExecContext(bg, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	verifier, key := testVerifier(t)
	app := inventoryapp.New(inventorypg.NewStore(database), salesreader.New(database))
	return NewMux(app, verifier), key, rest, user, database
}

func TestValidToken_ListProducts(t *testing.T) {
	mux, priv, restID, _, _ := setupMux(t)
	token := mint(t, priv, uuid.New().String(), restID.String(), []string{"manager"}, time.Now().Add(time.Hour))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+restID.String()+"/inventory/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("fresh restaurant products = %v, want empty list", got)
	}
}

func TestValidToken_CreateAndGetProduct(t *testing.T) {
	mux, priv, restID, _, _ := setupMux(t)
	token := mint(t, priv, uuid.New().String(), restID.String(), []string{"owner"}, time.Now().Add(time.Hour))

	body := `{"sku":"FLOUR","name":"Flour","type":"goods","stock_unit":"g"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restaurants/"+restID.String()+"/inventory/products", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	pid, _ := created["id"].(string)
	if pid == "" {
		t.Fatalf("created product has no id: %v", created)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/restaurants/"+restID.String()+"/inventory/products/"+pid, nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get product: status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
}
