package http

import "testing"

func TestValidateActionIndexes(t *testing.T) {
	if err := validateActionIndexes([]int{0, 2, 1}, 3); err != nil {
		t.Errorf("valid selection rejected: %v", err)
	}
	for name, sel := range map[string][]int{
		"out of range high": {0, 3},
		"negative":          {-1},
		"duplicate":         {1, 1},
	} {
		if err := validateActionIndexes(sel, 3); err == nil {
			t.Errorf("%s: accepted %v", name, sel)
		}
	}
	// Empty selection over zero actions is fine (caller guards non-empty
	// actions separately).
	if err := validateActionIndexes(nil, 0); err != nil {
		t.Errorf("empty: %v", err)
	}
}
