package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// CustomerSessionTTL — diner accounts stay signed in long-term.
const CustomerSessionTTL = 90 * 24 * time.Hour

// Customer auth mirrors staff auth but on entirely separate tables and
// sessions: a staff cookie never resolves a customer and vice versa.

func (a *App) RegisterCustomer(ctx context.Context, email, password, name string) (domain.Customer, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return domain.Customer{}, "", fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if !validEmail(email) {
		return domain.Customer{}, "", fmt.Errorf("%w: invalid email", ErrInvalid)
	}
	if len(password) < 8 {
		return domain.Customer{}, "", fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.Customer{}, "", fmt.Errorf("app: register customer: hash: %w", err)
	}
	c := domain.Customer{ID: uuid.New(), Email: strings.ToLower(email), PasswordHash: hash, Name: name}
	if err := a.store.CreateCustomer(ctx, c); err != nil {
		return domain.Customer{}, "", err
	}
	token, err := a.startCustomerSession(ctx, c.ID)
	if err != nil {
		return domain.Customer{}, "", err
	}
	return c, token, nil
}

func (a *App) startCustomerSession(ctx context.Context, customerID uuid.UUID) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	err = a.store.CreateCustomerSession(ctx, domain.Session{
		TokenHash: hashToken(token),
		UserID:    customerID,
		ExpiresAt: time.Now().UTC().Add(CustomerSessionTTL),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (a *App) LoginCustomer(ctx context.Context, email, password string) (domain.Customer, string, error) {
	c, err := a.store.CustomerByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, ports.ErrNotFound) {
		bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
		return domain.Customer{}, "", ErrUnauthorized
	}
	if err != nil {
		return domain.Customer{}, "", err
	}
	if bcrypt.CompareHashAndPassword(c.PasswordHash, []byte(password)) != nil {
		return domain.Customer{}, "", ErrUnauthorized
	}
	token, err := a.startCustomerSession(ctx, c.ID)
	if err != nil {
		return domain.Customer{}, "", err
	}
	return c, token, nil
}

func (a *App) LogoutCustomer(ctx context.Context, token string) error {
	return a.store.DeleteCustomerSession(ctx, hashToken(token))
}

// CustomerByToken resolves the aivo_customer cookie value.
func (a *App) CustomerByToken(ctx context.Context, token string) (domain.Customer, error) {
	if token == "" {
		return domain.Customer{}, ErrUnauthorized
	}
	c, err := a.store.CustomerSession(ctx, hashToken(token))
	if errors.Is(err, ports.ErrNotFound) {
		return domain.Customer{}, ErrUnauthorized
	}
	return c, err
}

func (a *App) CustomerHistory(ctx context.Context, customerID uuid.UUID) ([]domain.CustomerOrder, error) {
	return a.store.CustomerOrders(ctx, customerID, 50)
}

func (a *App) Customer(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	return a.store.CustomerByID(ctx, id)
}

// TouchGuest records a customer's activity at a restaurant (lazy CRM row).
func (a *App) TouchGuest(ctx context.Context, restaurantID, customerID uuid.UUID) error {
	return a.store.TouchGuestProfile(ctx, restaurantID, customerID)
}

// Guests / GuestDetail / UpdateGuest are the CRM surface (manager+
// enforcement is the HTTP layer's job; scoping here is by restaurantID
// coming from the tenant-checked path).
func (a *App) Guests(ctx context.Context, restaurantID uuid.UUID, query string, limit int) ([]domain.GuestSummary, error) {
	return a.store.Guests(ctx, restaurantID, query, limit)
}

func (a *App) GuestDetail(ctx context.Context, restaurantID, customerID uuid.UUID) (domain.GuestProfile, domain.GuestSummary, []domain.GuestOrder, error) {
	p, sum, err := a.store.GuestProfile(ctx, restaurantID, customerID)
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, nil, err
	}
	orders, err := a.store.GuestOrders(ctx, restaurantID, customerID)
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, nil, err
	}
	return p, sum, orders, nil
}

func (a *App) GuestOrdersFor(ctx context.Context, restaurantID, customerID uuid.UUID) ([]domain.GuestOrder, error) {
	return a.store.GuestOrders(ctx, restaurantID, customerID)
}

func (a *App) UpdateGuest(ctx context.Context, restaurantID, customerID uuid.UUID, notes string, tags []string) (domain.GuestProfile, domain.GuestSummary, error) {
	if len(notes) > 10_000 {
		return domain.GuestProfile{}, domain.GuestSummary{}, fmt.Errorf("%w: notes too long", ErrInvalid)
	}
	if len(tags) > 20 {
		return domain.GuestProfile{}, domain.GuestSummary{}, fmt.Errorf("%w: too many tags", ErrInvalid)
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if len(t) > 40 {
			return domain.GuestProfile{}, domain.GuestSummary{}, fmt.Errorf("%w: tag too long", ErrInvalid)
		}
		clean = append(clean, t)
	}
	err := a.store.UpdateGuestProfile(ctx, domain.GuestProfile{
		RestaurantID: restaurantID, CustomerID: customerID, Notes: notes, Tags: clean,
	})
	if err != nil {
		return domain.GuestProfile{}, domain.GuestSummary{}, err
	}
	p, sum, err := a.store.GuestProfile(ctx, restaurantID, customerID)
	return p, sum, err
}
