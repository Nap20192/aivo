package app

import (
	"context"
	"errors"
	"testing"

	"aivo/internal/platform/adapters/billing"
	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
)

// fakeStore is a minimal in-memory ports.Store for app-level tests.
type fakeStore struct {
	users       map[string]domain.User // by email
	sessions    map[string]uuid.UUID   // token hash -> user id
	subs        map[uuid.UUID]domain.Subscription
	restaurants map[uuid.UUID]domain.Restaurant
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:       map[string]domain.User{},
		sessions:    map[string]uuid.UUID{},
		subs:        map[uuid.UUID]domain.Subscription{},
		restaurants: map[uuid.UUID]domain.Restaurant{},
	}
}

func (f *fakeStore) CreateOrgWithOwner(_ context.Context, org domain.Organization, owner domain.User, sub domain.Subscription, rest domain.Restaurant) error {
	if _, ok := f.users[owner.Email]; ok {
		return ports.ErrConflict
	}
	for _, r := range f.restaurants {
		if r.Slug == rest.Slug {
			return errors.New("slug taken: " + ports.ErrConflict.Error())
		}
	}
	f.users[owner.Email] = owner
	f.subs[org.ID] = sub
	f.restaurants[rest.ID] = rest
	return nil
}

func (f *fakeStore) Organization(context.Context, uuid.UUID) (domain.Organization, error) {
	return domain.Organization{}, nil
}
func (f *fakeStore) UpdateOrganization(context.Context, domain.Organization) error { return nil }

func (f *fakeStore) UserByEmail(_ context.Context, email string) (domain.User, error) {
	u, ok := f.users[email]
	if !ok {
		return domain.User{}, ports.ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) UserByID(context.Context, uuid.UUID) (domain.User, error) {
	return domain.User{}, ports.ErrNotFound
}
func (f *fakeStore) CreateUser(_ context.Context, u domain.User) error {
	if _, ok := f.users[u.Email]; ok {
		return ports.ErrConflict
	}
	f.users[u.Email] = u
	return nil
}
func (f *fakeStore) StaffForRestaurant(context.Context, uuid.UUID, uuid.UUID) ([]domain.User, error) {
	return nil, nil
}

func (f *fakeStore) CreateSession(_ context.Context, s domain.Session) error {
	f.sessions[string(s.TokenHash)] = s.UserID
	return nil
}
func (f *fakeStore) SessionUser(_ context.Context, tokenHash []byte) (domain.User, error) {
	id, ok := f.sessions[string(tokenHash)]
	if !ok {
		return domain.User{}, ports.ErrNotFound
	}
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, ports.ErrNotFound
}
func (f *fakeStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	delete(f.sessions, string(tokenHash))
	return nil
}

func (f *fakeStore) Subscription(_ context.Context, orgID uuid.UUID) (domain.Subscription, error) {
	s, ok := f.subs[orgID]
	if !ok {
		return domain.Subscription{}, ports.ErrNotFound
	}
	return s, nil
}
func (f *fakeStore) SaveSubscription(_ context.Context, s domain.Subscription) error {
	f.subs[s.OrgID] = s
	return nil
}

func (f *fakeStore) CreateRestaurant(_ context.Context, r domain.Restaurant) error {
	f.restaurants[r.ID] = r
	return nil
}
func (f *fakeStore) Restaurants(_ context.Context, orgID uuid.UUID) ([]domain.Restaurant, error) {
	out := []domain.Restaurant{}
	for _, r := range f.restaurants {
		if r.OrgID == orgID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeStore) Restaurant(_ context.Context, orgID, id uuid.UUID) (domain.Restaurant, error) {
	r, ok := f.restaurants[id]
	if !ok || r.OrgID != orgID {
		// Foreign-org lookups 404, same as missing rows: tenant scoping.
		return domain.Restaurant{}, ports.ErrNotFound
	}
	return r, nil
}
func (f *fakeStore) UpdateRestaurant(_ context.Context, r domain.Restaurant) error {
	f.restaurants[r.ID] = r
	return nil
}
func (f *fakeStore) CountRestaurants(_ context.Context, orgID uuid.UUID) (int, error) {
	n := 0
	for _, r := range f.restaurants {
		if r.OrgID == orgID {
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) Theme(_ context.Context, id uuid.UUID) (domain.Theme, error) {
	return domain.Theme{RestaurantID: id}, nil
}
func (f *fakeStore) SaveTheme(context.Context, domain.Theme) error { return nil }
func (f *fakeStore) RestaurantByID(_ context.Context, id uuid.UUID) (domain.Restaurant, error) {
	r, ok := f.restaurants[id]
	if !ok {
		return domain.Restaurant{}, ports.ErrNotFound
	}
	return r, nil
}
func (f *fakeStore) CustomDomainForRestaurant(context.Context, uuid.UUID) (string, error) {
	return "", nil
}
func (f *fakeStore) SetCustomDomain(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeStore) RestaurantIDByDomain(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, ports.ErrNotFound
}

var _ ports.Store = (*fakeStore)(nil)

func newTestApp() (*App, *fakeStore) {
	st := newFakeStore()
	return New(st, billing.NewFake()), st
}

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp()

	owner, token, err := a.Register(ctx, "Ember & Bone", "Ember & Bone", "owner@ember.test", "embertest1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if owner.Role != domain.RoleOwner {
		t.Errorf("role = %s, want owner", owner.Role)
	}
	if token == "" {
		t.Fatal("register returned empty session token")
	}

	// Session token resolves back to the user; raw password never stored.
	u, err := a.UserByToken(ctx, token)
	if err != nil || u.ID != owner.ID {
		t.Fatalf("UserByToken: %v (user %v)", err, u.ID)
	}
	if string(u.PasswordHash) == "embertest1" {
		t.Fatal("password stored in plaintext")
	}

	// Wrong password and unknown email both fail closed.
	if _, _, err := a.Login(ctx, "owner@ember.test", "wrong-password"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("wrong password: got %v, want ErrUnauthorized", err)
	}
	if _, _, err := a.Login(ctx, "nobody@ember.test", "embertest1"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("unknown email: got %v, want ErrUnauthorized", err)
	}

	// Right password logs in; logout invalidates the session.
	_, token2, err := a.Login(ctx, "owner@ember.test", "embertest1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := a.Logout(ctx, token2); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := a.UserByToken(ctx, token2); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("after logout: got %v, want ErrUnauthorized", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp()
	if _, _, err := a.Register(ctx, "Org", "Rest", "not-an-email", "embertest1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad email: got %v, want ErrInvalid", err)
	}
	if _, _, err := a.Register(ctx, "Org", "Rest", "a@b.test", "short"); !errors.Is(err, ErrInvalid) {
		t.Errorf("short password: got %v, want ErrInvalid", err)
	}
}

func TestChangePlanStateMachine(t *testing.T) {
	ctx := context.Background()
	a, st := newTestApp()
	owner, _, err := a.Register(ctx, "Org", "Rest One", "o@x.test", "embertest1")
	if err != nil {
		t.Fatal(err)
	}

	// free -> pro: fake billing approves immediately, trialing -> active.
	sub, err := a.ChangePlan(ctx, owner.OrgID, domain.PlanPro)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if sub.Plan != domain.PlanPro || sub.Status != domain.SubActive {
		t.Errorf("after upgrade: %s/%s, want pro/active", sub.Plan, sub.Status)
	}

	// pro -> free: cancel billing, free plan is active.
	sub, err = a.ChangePlan(ctx, owner.OrgID, domain.PlanFree)
	if err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	if sub.Plan != domain.PlanFree || sub.Status != domain.SubActive {
		t.Errorf("after downgrade: %s/%s, want free/active", sub.Plan, sub.Status)
	}

	if _, err := a.ChangePlan(ctx, owner.OrgID, "gold"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown plan: got %v, want ErrInvalid", err)
	}
	_ = st
}

func TestCreateRestaurantPlanLimit(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp()
	owner, _, err := a.Register(ctx, "Org", "Rest One", "o@x.test", "embertest1")
	if err != nil {
		t.Fatal(err)
	}

	// Free plan: 1 restaurant max, the registration one used it up.
	if _, err := a.CreateRestaurant(ctx, owner.OrgID, "Second", ""); !errors.Is(err, ErrPlanLimit) {
		t.Fatalf("second restaurant on free: got %v, want ErrPlanLimit", err)
	}

	// Business plan: unlimited.
	if _, err := a.ChangePlan(ctx, owner.OrgID, domain.PlanBusiness); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateRestaurant(ctx, owner.OrgID, "Second", ""); err != nil {
		t.Fatalf("second restaurant on business: %v", err)
	}
}

func TestTenantScopingOnRestaurantLookup(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp()
	ownerA, _, err := a.Register(ctx, "Org A", "Rest A", "a@x.test", "embertest1")
	if err != nil {
		t.Fatal(err)
	}
	ownerB, _, err := a.Register(ctx, "Org B", "Rest B", "b@x.test", "embertest1")
	if err != nil {
		t.Fatal(err)
	}
	restsB, _ := a.Restaurants(ctx, ownerB.OrgID)
	if len(restsB) != 1 {
		t.Fatalf("org B restaurants: %d", len(restsB))
	}

	// Org A asking for org B's restaurant by ID gets a plain 404-shaped
	// error — same as a nonexistent ID.
	if _, err := a.Restaurant(ctx, ownerA.OrgID, restsB[0].ID); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("cross-tenant lookup: got %v, want ErrNotFound", err)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Ember & Bone":  "ember-and-bone",
		"  Café  24/7 ": "caf-24-7",
		"UPPER case":    "upper-case",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
