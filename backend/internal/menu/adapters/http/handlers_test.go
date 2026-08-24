package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/adapters/memory"
	"aivo/internal/menu/adapters/telegram"
	"aivo/internal/menu/app"

	"uuid"
)

func testKey() []byte { return make([]byte, 32) }

// newMux wires a fresh Application (against st and a real, but never
// actually invoked in these tests, Telegram Notifier — no restaurant
// seeds a NotificationChannel) and mounts it exactly like cmd/menu-server
// does, so these tests exercise the real routing/adapter wiring.
func newMux(st *memory.MemoryStore) http.Handler {
	return NewMux(app.NewApplication(st, telegram.New(), testKey(), "http://localhost:8080"))
}

func seedRestaurant(t *testing.T) (*memory.MemoryStore, domain.Restaurant, domain.Table, domain.MenuItem) {
	t.Helper()
	st := memory.NewMemoryStore()
	r := st.SeedRestaurant(domain.Restaurant{Slug: "acme", Name: "Acme Diner"})
	tbl := st.SeedTable(domain.Table{RestaurantID: r.ID, Label: "Table 1", Token: "tok123"})
	item := st.SeedMenuItem(domain.MenuItem{
		RestaurantID: r.ID,
		Name:         "Burger",
		PriceCents:   1000,
		Available:    true,
	})
	return st, r, tbl, item
}

// TestEndToEnd walks the full diner flow — landing, menu, order,
// service-request, QR — over the real mux, and checks cross-tenant/other
// restaurant tokens are rejected with the same generic 404 the tests use
// to prove no slug/token enumeration signal leaks.
func TestEndToEnd(t *testing.T) {
	st, _, _, item := seedRestaurant(t)
	mux := newMux(st)

	// landing
	req := httptest.NewRequest(http.MethodGet, "/api/landing/acme/tok123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("landing: status = %d, body = %s", w.Code, w.Body)
	}
	if w.Header().Get("Set-Cookie") == "" {
		t.Fatal("landing: expected a session cookie to be set")
	}

	// cross-tenant: real token, wrong slug -> generic 404, not e.g. a 400
	st.SeedRestaurant(domain.Restaurant{Slug: "other", Name: "Other"})
	req = httptest.NewRequest(http.MethodGet, "/api/landing/other/tok123", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant landing: status = %d, want 404", w.Code)
	}

	// menu
	req = httptest.NewRequest(http.MethodGet, "/api/menu/acme", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("menu: status = %d, body = %s", w.Code, w.Body)
	}
	var menuResp menuResponse
	if err := json.Unmarshal(w.Body.Bytes(), &menuResp); err != nil {
		t.Fatalf("menu: decode: %v", err)
	}
	if len(menuResp.Items) != 1 || menuResp.Items[0].Name != "Burger" {
		t.Fatalf("menu: items = %+v", menuResp.Items)
	}

	// order
	orderBody, _ := json.Marshal(createOrderRequest{
		RestaurantSlug: "acme",
		TableToken:     "tok123",
		Lines:          []orderLineRequest{{MenuItemID: item.ID, Qty: 2}},
		Comment:        "no onions",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(orderBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("order: status = %d, body = %s", w.Code, w.Body)
	}
	var order orderView
	if err := json.Unmarshal(w.Body.Bytes(), &order); err != nil {
		t.Fatalf("order: decode: %v", err)
	}
	if len(order.Lines) != 1 || order.Lines[0].Qty != 2 || order.Lines[0].UnitPriceCents != 1000 {
		t.Fatalf("order: lines = %+v", order.Lines)
	}

	// second immediate order from the same session is rate-limited
	req = httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(orderBody))
	req.Header.Set("Cookie", w.Result().Cookies()[0].Name+"="+w.Result().Cookies()[0].Value)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second order: status = %d, want 429", w2.Code)
	}

	// service request
	srBody, _ := json.Marshal(createServiceRequestRequest{
		RestaurantSlug: "acme", TableToken: "tok123", Kind: domain.CallWaiter,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/service-requests", bytes.NewReader(srBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("service request: status = %d, body = %s", w.Code, w.Body)
	}

	// duplicate open service request of the same kind is rate-limited
	req = httptest.NewRequest(http.MethodPost, "/api/service-requests", bytes.NewReader(srBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("duplicate service request: status = %d, want 429", w.Code)
	}

	// invalid kind
	badBody, _ := json.Marshal(createServiceRequestRequest{RestaurantSlug: "acme", TableToken: "tok123", Kind: "espresso"})
	req = httptest.NewRequest(http.MethodPost, "/api/service-requests", bytes.NewReader(badBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind: status = %d, want 400", w.Code)
	}

	// QR
	req = httptest.NewRequest(http.MethodGet, "/api/qr/acme/tok123", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("qr: status = %d, body = %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("qr: content-type = %q, want image/png", ct)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("qr: response body missing PNG magic header")
	}
}

// TestCreateOrder_UnknownMenuItem proves a client can't smuggle an item
// from a different restaurant (or a bogus ID) past validation into an
// Order snapshot.
func TestCreateOrder_UnknownMenuItem(t *testing.T) {
	st, _, _, _ := seedRestaurant(t)
	mux := newMux(st)

	body, _ := json.Marshal(createOrderRequest{
		RestaurantSlug: "acme",
		TableToken:     "tok123",
		Lines:          []orderLineRequest{{MenuItemID: uuid.New(), Qty: 1}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body)
	}
}

// TestCreateOrder_LineSnapshotSurvivesItemMutation proves an OrderLine is a
// snapshot: mutating the source MenuItem after an Order is placed must not
// retroactively change the persisted OrderLine (order-line-is-a-snapshot,
// issue #13).
func TestCreateOrder_LineSnapshotSurvivesItemMutation(t *testing.T) {
	st, r, _, item := seedRestaurant(t)
	mux := newMux(st)

	body, _ := json.Marshal(createOrderRequest{
		RestaurantSlug: "acme",
		TableToken:     "tok123",
		Lines:          []orderLineRequest{{MenuItemID: item.ID, Qty: 1}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("order: status = %d, body = %s", w.Code, w.Body)
	}
	var created orderView
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("order: decode: %v", err)
	}

	// Mutate the underlying MenuItem after the order was placed.
	st.SeedMenuItem(domain.MenuItem{
		ID:           item.ID,
		RestaurantID: r.ID,
		Name:         "Deluxe Burger",
		PriceCents:   2000,
		Available:    true,
	})

	// Re-fetch the persisted order (not the earlier response) and check
	// its line still reflects the item as it was at submission time.
	persisted, ok := st.Order(created.ID)
	if !ok {
		t.Fatalf("order %s not found in store", created.ID)
	}
	if len(persisted.Lines) != 1 {
		t.Fatalf("persisted order: lines = %+v", persisted.Lines)
	}
	line := persisted.Lines[0]
	if line.Name != "Burger" || line.UnitPriceCents != 1000 {
		t.Fatalf("persisted order line = %+v, want snapshot of original item (Burger, 1000)", line)
	}
}

// TestResolveTable_CrossTenantRejection proves a table_token scoped to one
// Restaurant is rejected — with the same generic 404 and no leaked data —
// both when claimed under a different Restaurant's slug and when the token
// is simply wrong.
func TestResolveTable_CrossTenantRejection(t *testing.T) {
	st, _, _, _ := seedRestaurant(t) // restaurant "acme", token "tok123"
	st.SeedRestaurant(domain.Restaurant{Slug: "other", Name: "Other Diner"})
	mux := newMux(st)

	cases := []struct {
		name  string
		slug  string
		token string
	}{
		{"acme's token claimed under other's slug", "other", "tok123"},
		{"bare wrong token under acme's own slug", "acme", "not-a-real-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/landing/"+tc.slug+"/"+tc.token, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body)
			}
			body := w.Body.String()
			for _, leaked := range []string{"Acme Diner", "acme", "Burger", "Table 1", "tok123"} {
				if strings.Contains(body, leaked) {
					t.Fatalf("response body leaked %q: %s", leaked, body)
				}
			}
		})
	}
}

// TestIPRateLimit proves the global middleware actually rejects once the
// fixed-window IP limit (20/min, see internal/session) is exceeded.
func TestIPRateLimit(t *testing.T) {
	st, _, _, _ := seedRestaurant(t)
	mux := newMux(st)

	var last *httptest.ResponseRecorder
	for i := 0; i < 25; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/menu/acme", nil)
		req.RemoteAddr = "203.0.113.9:12345"
		last = httptest.NewRecorder()
		mux.ServeHTTP(last, req)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("25th request from same IP: status = %d, want 429", last.Code)
	}
}
