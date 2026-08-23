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
