package http

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"aivo/internal/domain/platform"

	"uuid"
)

// fakeTokenMinter is a platformports.TokenMinter test double — no real
// aivo-auth service or network involved.
type fakeTokenMinter struct {
	token string
	err   error

	calledUserID   uuid.UUID
	calledTenantID uuid.UUID
	calledRoles    []string
	calledAppID    string
	calls          int
}

func (f *fakeTokenMinter) Mint(_ context.Context, userID, tenantID uuid.UUID, roles []string, appID string) (string, error) {
	f.calls++
	f.calledUserID = userID
	f.calledTenantID = tenantID
	f.calledRoles = roles
	f.calledAppID = appID
	return f.token, f.err
}

func restaurantScopedUser() domain.User {
	restID := uuid.New()
	return domain.User{
		ID:           uuid.New(),
		OrgID:        uuid.New(),
		Role:         domain.RoleManager,
		RestaurantID: &restID,
	}
}

func TestMintToken_NilTokenMinterDisablesMinting(t *testing.T) {
	h := &handler{Deps: Deps{}} // TokenMinter left nil
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	token, ok := h.mintToken(r, restaurantScopedUser(), "admin")
	if ok || token != "" {
		t.Fatalf("mintToken with nil TokenMinter = (%q, %v), want (\"\", false)", token, ok)
	}
}

func TestMintToken_RestaurantScopedUser(t *testing.T) {
	fake := &fakeTokenMinter{token: "signed.jwt.token"}
	h := &handler{Deps: Deps{TokenMinter: fake}}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	u := restaurantScopedUser()

	token, ok := h.mintToken(r, u, "pos")
	if !ok || token != "signed.jwt.token" {
		t.Fatalf("mintToken = (%q, %v), want (\"signed.jwt.token\", true)", token, ok)
	}
	if fake.calls != 1 {
		t.Fatalf("expected exactly 1 Mint call, got %d", fake.calls)
	}
	if fake.calledUserID != u.ID {
		t.Errorf("Mint called with userID %v, want %v", fake.calledUserID, u.ID)
	}
	if fake.calledTenantID != *u.RestaurantID {
		t.Errorf("Mint called with tenantID %v, want the user's restaurant %v", fake.calledTenantID, *u.RestaurantID)
	}
	if len(fake.calledRoles) != 1 || fake.calledRoles[0] != string(u.Role) {
		t.Errorf("Mint called with roles %v, want [%s]", fake.calledRoles, u.Role)
	}
	if fake.calledAppID != "pos" {
		t.Errorf("Mint called with app_id %q, want %q", fake.calledAppID, "pos")
	}
}

func TestMintToken_EmptyAppIDDefaultsToAdmin(t *testing.T) {
	fake := &fakeTokenMinter{token: "t"}
	h := &handler{Deps: Deps{TokenMinter: fake}}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	if _, ok := h.mintToken(r, restaurantScopedUser(), ""); !ok {
		t.Fatal("expected mintToken to succeed")
	}
	if fake.calledAppID != defaultAppID {
		t.Errorf("Mint called with app_id %q, want default %q", fake.calledAppID, defaultAppID)
	}
}

func TestMintToken_MintErrorDoesNotBreakLogin(t *testing.T) {
	fake := &fakeTokenMinter{err: errors.New("aivo-auth unreachable")}
	h := &handler{Deps: Deps{TokenMinter: fake}}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	token, ok := h.mintToken(r, restaurantScopedUser(), "admin")
	if ok || token != "" {
		t.Fatalf("mintToken with a failing minter = (%q, %v), want (\"\", false)", token, ok)
	}
}
