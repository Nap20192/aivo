package domain

import (
	"errors"
	"testing"
)

func TestSubscriptionTransitions(t *testing.T) {
	s := Subscription{Status: SubTrialing}

	// Happy path through the whole machine.
	for _, next := range []SubscriptionStatus{SubActive, SubPastDue, SubActive, SubCanceled} {
		if err := s.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}

	// canceled is terminal.
	if err := s.Transition(SubActive); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("canceled -> active: got %v, want ErrInvalidTransition", err)
	}

	// Skipping states is not allowed where no edge exists.
	s = Subscription{Status: SubActive}
	if err := s.Transition(SubTrialing); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("active -> trialing: got %v, want ErrInvalidTransition", err)
	}
}

func TestValidSlug(t *testing.T) {
	for slug, want := range map[string]bool{
		"ember-and-bone": true,
		"a":              true,
		"table-3":        true,
		"":               false,
		"-lead":          false,
		"trail-":         false,
		"double--hyphen": false,
		"UPPER":          false,
		"has space":      false,
		"api":            false, // reserved
		"admin":          false, // reserved
		"pos":            false, // reserved
	} {
		if got := ValidSlug(slug); got != want {
			t.Errorf("ValidSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestPlanLimits(t *testing.T) {
	if got := PlanFree.MaxMenuItems(); got != 30 {
		t.Errorf("free item limit = %d, want 30", got)
	}
	if got := PlanPro.MaxMenuItems(); got != 0 {
		t.Errorf("pro item limit = %d, want 0 (unlimited)", got)
	}
	if got := PlanFree.MaxRestaurants(); got != 1 {
		t.Errorf("free restaurant limit = %d, want 1", got)
	}
	if got := PlanBusiness.MaxRestaurants(); got != 0 {
		t.Errorf("business restaurant limit = %d, want 0 (unlimited)", got)
	}
}
