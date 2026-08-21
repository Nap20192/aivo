// Package app is the Platform context's use-case layer: registration,
// auth/sessions, org + subscription management, restaurant provisioning,
// themes, staff. Methods on one App struct (smaller surface than the menu
// context's per-handler CQRS types; same hexagonal position).
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// SessionTTL is how long an aivo_session login lives.
const SessionTTL = 30 * 24 * time.Hour

// ErrInvalid is returned for caller-fixable input problems; the HTTP
// adapter maps it (and anything wrapping it) to 422.
var ErrInvalid = errors.New("invalid input")

// ErrUnauthorized is returned for bad credentials / missing session (401).
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden is returned when the session user lacks the role or
// restaurant scope for the action (403).
var ErrForbidden = errors.New("forbidden")

// ErrPlanLimit is returned when a plan limit blocks the action (422).
var ErrPlanLimit = errors.New("plan limit reached")

type App struct {
	store    ports.Store
	billing  ports.BillingProvider
	themeGen ports.ThemeGenerator // nil = generation not configured
}

func New(store ports.Store, billing ports.BillingProvider, themeGen ports.ThemeGenerator) *App {
	return &App{store: store, billing: billing, themeGen: themeGen}
}

// Theme-generation errors (HTTP: 503 / 409).
var (
	ErrGeneratorUnavailable = errors.New("theme generator not configured")
	ErrNoDesignMD           = errors.New("design_md is empty; paste a design brief first")
)

// GenerateTheme proposes a theme from the restaurant's stored design_md.
// Never saves — applying is the explicit PUT .../theme. The proposal is
// logged (AGENTS.md: log AI-generated recommendations).
func (a *App) GenerateTheme(ctx context.Context, restaurantID uuid.UUID) (domain.Theme, error) {
	if a.themeGen == nil {
		return domain.Theme{}, ErrGeneratorUnavailable
	}
	current, err := a.store.Theme(ctx, restaurantID)
	if err != nil {
		return domain.Theme{}, err
	}
	if strings.TrimSpace(current.DesignMD) == "" {
		return domain.Theme{}, ErrNoDesignMD
	}
	proposal, err := a.themeGen.Generate(ctx, current.DesignMD, current)
	if err != nil {
		return domain.Theme{}, err
	}
	slog.Info("ai theme proposal generated",
		"restaurant_id", restaurantID,
		"based_on", "design_md",
		"proposal", string(proposal.ThemeJSON))
	return proposal, nil
}

// hashToken is the storage form of a session token: SHA-256, so a DB
// leak doesn't leak usable cookies.
func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("app: token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validEmail(e string) bool {
	if len(e) > 254 {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}

// Slugify turns a display name into a slug candidate: lowercase, spaces
// and punctuation to hyphens, "&" to "and".
func Slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "&", " and ")
	var b strings.Builder
	lastHyphen := true // suppress leading hyphen
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// --- Auth -------------------------------------------------------------

// Register creates org + owner + free subscription + first restaurant,
// then logs the owner in. Returns the user and the raw session token for
// the cookie.
func (a *App) Register(ctx context.Context, orgName, restaurantName, email, password string) (domain.User, string, error) {
	orgName, restaurantName = strings.TrimSpace(orgName), strings.TrimSpace(restaurantName)
	if orgName == "" || restaurantName == "" {
		return domain.User{}, "", fmt.Errorf("%w: org_name and restaurant_name are required", ErrInvalid)
	}
	if !validEmail(email) {
		return domain.User{}, "", fmt.Errorf("%w: invalid email", ErrInvalid)
	}
	if len(password) < 8 {
		return domain.User{}, "", fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}
	slug := Slugify(restaurantName)
	if !domain.ValidSlug(slug) {
		return domain.User{}, "", fmt.Errorf("%w: restaurant_name does not produce a usable slug", ErrInvalid)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("app: register: hash: %w", err)
	}

	org := domain.Organization{ID: uuid.New(), Name: orgName}
	owner := domain.User{
		ID: uuid.New(), OrgID: org.ID, Email: strings.ToLower(email),
		PasswordHash: hash, Role: domain.RoleOwner,
	}
	sub := domain.Subscription{OrgID: org.ID, Plan: domain.PlanFree, Status: domain.SubActive}
	rest := domain.Restaurant{ID: uuid.New(), OrgID: org.ID, Slug: slug, Name: restaurantName}

	// On a taken slug, retry once with a short random suffix before
	// giving up — "Ember & Bone" in two cities shouldn't 409 on signup.
	err = a.store.CreateOrgWithOwner(ctx, org, owner, sub, rest)
	if errors.Is(err, ports.ErrConflict) && strings.Contains(err.Error(), "slug") {
		rest.Slug = slug + "-" + randomSuffix()
		err = a.store.CreateOrgWithOwner(ctx, org, owner, sub, rest)
	}
	if err != nil {
		return domain.User{}, "", err
	}

	token, err := a.startSession(ctx, owner.ID)
	if err != nil {
		return domain.User{}, "", err
	}
	return owner, token, nil
}

func randomSuffix() string {
	b := make([]byte, 3)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b) // 4 chars
}

func (a *App) startSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	err = a.store.CreateSession(ctx, domain.Session{
		TokenHash: hashToken(token),
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(SessionTTL),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Login checks credentials and starts a session. Bad email and bad
// password collapse to the same ErrUnauthorized.
func (a *App) Login(ctx context.Context, email, password string) (domain.User, string, error) {
	u, err := a.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, ports.ErrNotFound) {
		// Burn a comparable amount of time so an attacker can't tell a
		// wrong email from a wrong password by latency.
		bcrypt.CompareHashAndPassword([]byte("$2a$10$0123456789012345678901uCsB6zwzoiZ9BvbVHGwFzLB1p9PY0P2"), []byte(password))
		return domain.User{}, "", ErrUnauthorized
	}
	if err != nil {
		return domain.User{}, "", err
	}
	if bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)) != nil {
		return domain.User{}, "", ErrUnauthorized
	}
	token, err := a.startSession(ctx, u.ID)
	if err != nil {
		return domain.User{}, "", err
	}
	return u, token, nil
}

func (a *App) Logout(ctx context.Context, token string) error {
	return a.store.DeleteSession(ctx, hashToken(token))
}

// UserByToken resolves the aivo_session cookie value to its user.
// Returns ErrUnauthorized for unknown/expired tokens.
func (a *App) UserByToken(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, ErrUnauthorized
	}
	u, err := a.store.SessionUser(ctx, hashToken(token))
	if errors.Is(err, ports.ErrNotFound) {
		return domain.User{}, ErrUnauthorized
	}
	return u, err
}

// --- Org & subscription ------------------------------------------------

func (a *App) Organization(ctx context.Context, orgID uuid.UUID) (domain.Organization, error) {
	return a.store.Organization(ctx, orgID)
}

func (a *App) RenameOrganization(ctx context.Context, orgID uuid.UUID, name string) (domain.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Organization{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	org := domain.Organization{ID: orgID, Name: name}
	if err := a.store.UpdateOrganization(ctx, org); err != nil {
		return domain.Organization{}, err
	}
	return a.store.Organization(ctx, orgID)
}

func (a *App) Subscription(ctx context.Context, orgID uuid.UUID) (domain.Subscription, error) {
	return a.store.Subscription(ctx, orgID)
}

// ChangePlan switches the org's plan through the billing provider and
// the subscription state machine. Switching to free cancels paid billing
// (free is always "active"); switching to a paid plan starts at trialing
// and transitions to whatever the provider reports.
func (a *App) ChangePlan(ctx context.Context, orgID uuid.UUID, plan domain.Plan) (domain.Subscription, error) {
	if !domain.ValidPlan(plan) {
		return domain.Subscription{}, fmt.Errorf("%w: unknown plan", ErrInvalid)
	}
	sub, err := a.store.Subscription(ctx, orgID)
	if err != nil {
		return domain.Subscription{}, err
	}
	if sub.Plan == plan {
		return sub, nil
	}

	if plan == domain.PlanFree {
		if err := a.billing.Cancel(ctx, orgID); err != nil {
			return domain.Subscription{}, fmt.Errorf("app: billing cancel: %w", err)
		}
		sub = domain.Subscription{OrgID: orgID, Plan: domain.PlanFree, Status: domain.SubActive}
	} else {
		status, err := a.billing.Subscribe(ctx, orgID, plan)
		if err != nil {
			return domain.Subscription{}, fmt.Errorf("app: billing subscribe: %w", err)
		}
		// A new paid subscription starts at trialing and moves to what
		// the provider reports (the fake reports active immediately).
		next := domain.Subscription{OrgID: orgID, Plan: plan, Status: domain.SubTrialing}
		if status != domain.SubTrialing {
			if err := next.Transition(status); err != nil {
				return domain.Subscription{}, err
			}
		}
		sub = next
	}
	if err := a.store.SaveSubscription(ctx, sub); err != nil {
		return domain.Subscription{}, err
	}
	// Re-read so UpdatedAt reflects the store's now().
	return a.store.Subscription(ctx, orgID)
}

// --- Restaurants -------------------------------------------------------

func (a *App) Restaurants(ctx context.Context, orgID uuid.UUID) ([]domain.Restaurant, error) {
	return a.store.Restaurants(ctx, orgID)
}

func (a *App) Restaurant(ctx context.Context, orgID, id uuid.UUID) (domain.Restaurant, error) {
	return a.store.Restaurant(ctx, orgID, id)
}

// CreateRestaurant provisions a new restaurant under the org, enforcing
// the plan's restaurant limit.
func (a *App) CreateRestaurant(ctx context.Context, orgID uuid.UUID, name, slug string) (domain.Restaurant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Restaurant{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if slug == "" {
		slug = Slugify(name)
	}
	if !domain.ValidSlug(slug) {
		return domain.Restaurant{}, fmt.Errorf("%w: invalid slug", ErrInvalid)
	}

	sub, err := a.store.Subscription(ctx, orgID)
	if err != nil {
		return domain.Restaurant{}, err
	}
	if max := sub.Plan.MaxRestaurants(); max > 0 {
		n, err := a.store.CountRestaurants(ctx, orgID)
		if err != nil {
			return domain.Restaurant{}, err
		}
		if n >= max {
			return domain.Restaurant{}, fmt.Errorf("%w: plan %s allows %d restaurant(s)", ErrPlanLimit, sub.Plan, max)
		}
	}

	r := domain.Restaurant{ID: uuid.New(), OrgID: orgID, Slug: slug, Name: name}
	if err := a.store.CreateRestaurant(ctx, r); err != nil {
		return domain.Restaurant{}, err
	}
	return a.store.Restaurant(ctx, orgID, r.ID)
}

// RestaurantPatch carries a partial update; nil fields keep the current
// value (the HTTP adapter passes only what the client sent). Phone and
// Instagram live in the contacts map but patch as flat fields, matching
// the admin client's Restaurant shape.
type RestaurantPatch struct {
	Slug         *string
	Name         *string
	Address      *string
	Hours        *[]domain.HoursRow
	Phone        *string
	Instagram    *string
	Contacts     map[string]string
	CustomDomain *string
}

func (a *App) UpdateRestaurant(ctx context.Context, orgID, id uuid.UUID, patch RestaurantPatch) (domain.Restaurant, error) {
	r, err := a.store.Restaurant(ctx, orgID, id)
	if err != nil {
		return domain.Restaurant{}, err
	}
	if patch.Slug != nil {
		if !domain.ValidSlug(*patch.Slug) {
			return domain.Restaurant{}, fmt.Errorf("%w: invalid slug", ErrInvalid)
		}
		r.Slug = *patch.Slug
	}
	if patch.Name != nil {
		if strings.TrimSpace(*patch.Name) == "" {
			return domain.Restaurant{}, fmt.Errorf("%w: name cannot be empty", ErrInvalid)
		}
		r.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Address != nil {
		r.Address = *patch.Address
	}
	if patch.Hours != nil {
		if len(*patch.Hours) > 20 {
			return domain.Restaurant{}, fmt.Errorf("%w: too many hours rows", ErrInvalid)
		}
		r.Hours = *patch.Hours
	}
	if patch.Contacts != nil {
		r.Contacts = patch.Contacts
	}
	if r.Contacts == nil {
		r.Contacts = map[string]string{}
	}
	if patch.Phone != nil {
		r.Contacts["phone"] = *patch.Phone
	}
	if patch.Instagram != nil {
		r.Contacts["instagram"] = *patch.Instagram
	}
	if err := a.store.UpdateRestaurant(ctx, r); err != nil {
		return domain.Restaurant{}, err
	}
	if patch.CustomDomain != nil {
		host := strings.ToLower(strings.TrimSpace(*patch.CustomDomain))
		if host != "" && !validHostname(host) {
			return domain.Restaurant{}, fmt.Errorf("%w: invalid custom domain", ErrInvalid)
		}
		if err := a.store.SetCustomDomain(ctx, r.ID, host); err != nil {
			return domain.Restaurant{}, err
		}
	}
	return r, nil
}

var hostnameRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

func validHostname(h string) bool { return len(h) <= 253 && hostnameRe.MatchString(h) }

// CustomDomain returns the restaurant's claimed custom domain, "" if none.
func (a *App) CustomDomain(ctx context.Context, restaurantID uuid.UUID) (string, error) {
	return a.store.CustomDomainForRestaurant(ctx, restaurantID)
}

// RestaurantPublic is the org-unscoped lookup for public composition
// (diner entry). Never use it on org-authenticated paths.
func (a *App) RestaurantPublic(ctx context.Context, id uuid.UUID) (domain.Restaurant, error) {
	return a.store.RestaurantByID(ctx, id)
}

// User resolves a user by ID (POS cashier display).
func (a *App) User(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return a.store.UserByID(ctx, id)
}

// --- Theme -------------------------------------------------------------

func (a *App) Theme(ctx context.Context, restaurantID uuid.UUID) (domain.Theme, error) {
	return a.store.Theme(ctx, restaurantID)
}

func (a *App) SaveTheme(ctx context.Context, t domain.Theme) (domain.Theme, error) {
	if len(t.ThemeJSON) == 0 {
		t.ThemeJSON = []byte(`{}`)
	}
	if len(t.ThemeJSON) > 64<<10 || len(t.DesignMD) > 256<<10 {
		return domain.Theme{}, fmt.Errorf("%w: theme or design_md too large", ErrInvalid)
	}
	if err := a.store.SaveTheme(ctx, t); err != nil {
		return domain.Theme{}, err
	}
	return a.store.Theme(ctx, t.RestaurantID)
}

// --- Staff -------------------------------------------------------------

func (a *App) Staff(ctx context.Context, orgID, restaurantID uuid.UUID) ([]domain.User, error) {
	return a.store.StaffForRestaurant(ctx, orgID, restaurantID)
}

// AddStaff creates a manager/waiter account scoped to the restaurant.
// (Owners are created only via Register.) An empty password creates an
// "invited" account with an unguessable random password — no invite
// email exists yet, so the owner shares credentials out of band or the
// account stays unusable until a future invite/reset flow.
func (a *App) AddStaff(ctx context.Context, orgID, restaurantID uuid.UUID, email, password string, role domain.Role) (domain.User, error) {
	if role != domain.RoleManager && role != domain.RoleWaiter {
		return domain.User{}, fmt.Errorf("%w: role must be manager or waiter", ErrInvalid)
	}
	if !validEmail(email) {
		return domain.User{}, fmt.Errorf("%w: invalid email", ErrInvalid)
	}
	if password == "" {
		random, err := newToken()
		if err != nil {
			return domain.User{}, err
		}
		password = random
	}
	if len(password) < 8 {
		return domain.User{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("app: add staff: hash: %w", err)
	}
	u := domain.User{
		ID: uuid.New(), OrgID: orgID, Email: strings.ToLower(email),
		PasswordHash: hash, Role: role, RestaurantID: &restaurantID,
	}
	if err := a.store.CreateUser(ctx, u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// ItemLimitFor returns the org's per-restaurant menu item limit
// (0 = unlimited), for the admin item-create path.
func (a *App) ItemLimitFor(ctx context.Context, orgID uuid.UUID) (int, error) {
	sub, err := a.store.Subscription(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return sub.Plan.MaxMenuItems(), nil
}
