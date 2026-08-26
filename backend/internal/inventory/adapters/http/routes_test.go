package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"
)

// TestValidToken_FullRouteSurface exercises every route NewMux registers
// once each with a valid token, end to end against a real store — the
// "behaves identically to today given a valid token" regression check
// (specs/inventory-service), covering the full endpoint surface the
// admin frontend depends on, not just products (already covered by
// TestValidToken_ListProducts / TestValidToken_CreateAndGetProduct).
func TestValidToken_FullRouteSurface(t *testing.T) {
	mux, priv, restID, userID, _ := setupMux(t)
	token := mint(t, priv, userID.String(), restID.String(), []string{"owner"}, time.Now().Add(time.Hour))

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
		}
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var v map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		return v
	}
	base := "/api/v1/restaurants/" + restID.String() + "/inventory"

	// --- products + tech cards -----------------------------------------
	rec := do(http.MethodPost, base+"/products", `{"sku":"FLOUR","name":"Flour","type":"goods","stock_unit":"g","min_stock":2}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create flour: %d %s", rec.Code, rec.Body)
	}
	flourID := decode(rec)["id"].(string)

	menuItem := uuid.New().String()
	rec = do(http.MethodPost, base+"/products", `{"sku":"SOUP","name":"Soup","type":"dish","stock_unit":"pcs","menu_item_id":"`+menuItem+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create soup: %d %s", rec.Code, rec.Body)
	}
	soupID := decode(rec)["id"].(string)

	if rec := do(http.MethodPatch, base+"/products/"+soupID, `{"name":"Soup v2","min_stock":3}`); rec.Code != http.StatusOK {
		t.Fatalf("patch product: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/products/not-a-uuid", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get product bad id: %d %s", rec.Code, rec.Body)
	}

	rec = do(http.MethodPost, base+"/products/"+soupID+"/tech-cards",
		`{"valid_from":"2026-01-01","consumption":"assemble","lines":[{"ingredient_product_id":"`+flourID+`","qty":200,"unit":"g"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tech card: %d %s", rec.Code, rec.Body)
	}
	tcID := decode(rec)["id"].(string)

	if rec := do(http.MethodGet, base+"/products/"+soupID+"/tech-cards", ""); rec.Code != http.StatusOK {
		t.Fatalf("list tech cards: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/products/"+soupID+"/tech-cards/active", ""); rec.Code != http.StatusOK {
		t.Fatalf("active tech card: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/products/"+soupID+"/tech-cards/active?on=2026-01-15", ""); rec.Code != http.StatusOK {
		t.Fatalf("active tech card with ?on=: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/tech-cards/"+tcID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get tech card: %d %s", rec.Code, rec.Body)
	}

	// --- suppliers -------------------------------------------------------
	rec = do(http.MethodPost, base+"/suppliers", `{"name":"Acme"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create supplier: %d %s", rec.Code, rec.Body)
	}
	supplierID := decode(rec)["id"].(string)
	if rec := do(http.MethodGet, base+"/suppliers", ""); rec.Code != http.StatusOK {
		t.Fatalf("list suppliers: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPatch, base+"/suppliers/"+supplierID, `{"name":"Acme Foods"}`); rec.Code != http.StatusOK {
		t.Fatalf("patch supplier: %d %s", rec.Code, rec.Body)
	}

	// --- receipt: create, list, get, post, cancel -------------------------
	rec = do(http.MethodPost, base+"/receipts",
		`{"supplier_id":"`+supplierID+`","business_date":"2026-01-01","lines":[{"product_id":"`+flourID+`","qty":5000,"unit":"g","unit_price_cents":6}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create receipt: %d %s", rec.Code, rec.Body)
	}
	receiptID := decode(rec)["id"].(string)
	if rec := do(http.MethodGet, base+"/receipts", ""); rec.Code != http.StatusOK {
		t.Fatalf("list receipts: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/receipts/"+receiptID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get receipt: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/receipts/"+receiptID+"/post", ""); rec.Code != http.StatusOK {
		t.Fatalf("post receipt: %d %s", rec.Code, rec.Body)
	}

	// --- write-off: create, list, get, post, cancel -----------------------
	rec = do(http.MethodPost, base+"/write-offs",
		`{"reason":"spoilage","business_date":"2026-01-02","lines":[{"product_id":"`+flourID+`","qty":100,"unit":"g"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create write-off: %d %s", rec.Code, rec.Body)
	}
	writeOffID := decode(rec)["id"].(string)
	if rec := do(http.MethodGet, base+"/write-offs", ""); rec.Code != http.StatusOK {
		t.Fatalf("list write-offs: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/write-offs/"+writeOffID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get write-off: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/write-offs/"+writeOffID+"/post", ""); rec.Code != http.StatusOK {
		t.Fatalf("post write-off: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/write-offs/"+writeOffID+"/cancel", ""); rec.Code != http.StatusOK {
		t.Fatalf("cancel write-off: %d %s", rec.Code, rec.Body)
	}

	// --- stocktake: create, list, get, enter counts, dry-run, post, cancel -
	rec = do(http.MethodPost, base+"/stocktakes", `{"note":"weekly count"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create stocktake: %d %s", rec.Code, rec.Body)
	}
	stocktakeID := decode(rec)["id"].(string)
	if rec := do(http.MethodGet, base+"/stocktakes", ""); rec.Code != http.StatusOK {
		t.Fatalf("list stocktakes: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/stocktakes/"+stocktakeID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get stocktake: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPatch, base+"/stocktakes/"+stocktakeID,
		`{"lines":[{"product_id":"`+flourID+`","counted_qty":4000,"unit":"g"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("enter counts: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/stocktakes/"+stocktakeID+"/dry-run", ""); rec.Code != http.StatusOK {
		t.Fatalf("dry-run stocktake: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/stocktakes/"+stocktakeID+"/post", ""); rec.Code != http.StatusOK {
		t.Fatalf("post stocktake: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/stocktakes/"+stocktakeID+"/cancel", ""); rec.Code != http.StatusOK {
		t.Fatalf("cancel stocktake: %d %s", rec.Code, rec.Body)
	}

	// --- reads: on-hand, stock-moves, recost, food-cost, receipt cancel ---
	if rec := do(http.MethodGet, base+"/on-hand", ""); rec.Code != http.StatusOK {
		t.Fatalf("on-hand: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/on-hand?low_stock=1", ""); rec.Code != http.StatusOK {
		t.Fatalf("on-hand low_stock: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/stock-moves?product="+flourID, ""); rec.Code != http.StatusOK {
		t.Fatalf("stock-moves: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/stock-moves?product=not-a-uuid", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("stock-moves bad product filter: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/tech-cards/"+tcID+"/recost", ""); rec.Code != http.StatusOK {
		t.Fatalf("recost: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, base+"/reports/food-cost?from=2026-01-01&to=2026-01-31", ""); rec.Code != http.StatusOK {
		t.Fatalf("food-cost: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/receipts/"+receiptID+"/cancel", ""); rec.Code != http.StatusOK {
		t.Fatalf("cancel receipt: %d %s", rec.Code, rec.Body)
	}
}
