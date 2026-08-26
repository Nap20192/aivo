package domain

import (
	"errors"
	"testing"
)

func TestCanDeleteMenu(t *testing.T) {
	def := Menu{IsDefault: true}
	extra := Menu{}

	if err := CanDeleteMenu(def, 2, 0, false); !errors.Is(err, ErrDefaultMenuDelete) {
		t.Errorf("default: got %v", err)
	}
	if err := CanDeleteMenu(extra, 1, 0, false); !errors.Is(err, ErrLastMenuDelete) {
		t.Errorf("last: got %v", err)
	}
	if err := CanDeleteMenu(extra, 2, 3, false); !errors.Is(err, ErrMenuNotEmpty) {
		t.Errorf("non-empty without force: got %v", err)
	}
	if err := CanDeleteMenu(extra, 2, 3, true); err != nil {
		t.Errorf("non-empty with force: got %v", err)
	}
	if err := CanDeleteMenu(extra, 2, 0, false); err != nil {
		t.Errorf("empty extra menu: got %v", err)
	}
}

func TestServiceRequestStatusValid(t *testing.T) {
	cases := []struct {
		s    ServiceRequestStatus
		want bool
	}{
		{ServiceRequestPending, true}, {ServiceRequestAcknowledged, true}, {ServiceRequestDismissed, true},
		{"", false}, {"bogus", false},
	}
	for _, c := range cases {
		if got := c.s.Valid(); got != c.want {
			t.Errorf("ServiceRequestStatus(%q).Valid() = %v, want %v", c.s, got, c.want)
		}
	}
	if ServiceRequestStatus("").Default() != ServiceRequestPending {
		t.Errorf("ServiceRequestStatus default = %q, want %q", ServiceRequestStatus("").Default(), ServiceRequestPending)
	}
}
