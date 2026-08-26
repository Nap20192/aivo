package domain

import (
	"errors"
	"testing"

	"aivo/internal/sharedkernel"
)

func ptr[T any](v T) *T { return &v }

func TestValidateActionAllowlist(t *testing.T) {
	id := sharedkernel.NewID()
	valid := []AssistantAction{
		{Type: ActionCreateCategory, MenuID: &id, Name: "Starters"},
		{Type: ActionRenameCategory, ID: &id, Name: "Snacks"},
		{Type: ActionDeleteCategory, ID: &id},
		{Type: ActionCreateItem, CategoryID: &id, Name: "Caesar", PriceCents: ptr(1200)},
		{Type: ActionUpdateItem, ID: &id, PriceCents: ptr(900)},
		{Type: ActionDeleteItem, ID: &id},
		{Type: ActionSetItemAvailable, ID: &id, Available: ptr(false)},
		{Type: ActionCreateMenu, Name: "Lunch", Slug: "lunch"},
		{Type: ActionUpdateTheme, Theme: &ThemePayload{BrandName: "E", Accent: "Wine"}},
	}
	for _, a := range valid {
		if err := ValidateAction(a); err != nil {
			t.Errorf("%s: unexpected reject: %v", a.Type, err)
		}
	}

	invalid := []AssistantAction{
		{Type: "drop_database"},                                                   // unknown type
		{Type: ActionCreateItem, CategoryID: &id, Name: "X"},                      // missing price
		{Type: ActionCreateItem, CategoryID: &id, Name: "X", PriceCents: ptr(-1)}, // negative price
		{Type: ActionCreateCategory, Name: "X"},                                   // missing menu_id
		{Type: ActionSetItemAvailable, ID: &id},                                   // missing available
		{Type: ActionCreateMenu, Name: "X", Slug: "Bad Slug"},                     // bad slug
		{Type: ActionUpdateTheme, Theme: &ThemePayload{BrandName: "E", Accent: "Hot pink"}},
		{Type: ActionUpdateTheme, Theme: &ThemePayload{BrandName: "E", Accent: "Wine",
			CSSVars: map[string]string{"--x": "url(http://evil)"}}},
		{Type: ActionUpdateTheme, Theme: &ThemePayload{BrandName: "E", Accent: "Wine",
			CSSVars: map[string]string{"background": "red"}}},
	}
	for _, a := range invalid {
		if err := ValidateAction(a); !errors.Is(err, ErrInvalidAction) {
			t.Errorf("%s: got %v, want ErrInvalidAction", a.Type, err)
		}
	}
}

func TestValidCSSVarRejectsBackslashEscapes(t *testing.T) {
	// "\75rl(" spells url( via CSS character escapes — any backslash is
	// rejected outright.
	for _, v := range []string{`\75rl(http://evil)`, `red\`, `a\9`} {
		if err := ValidCSSVar("--x", v); err == nil {
			t.Errorf("value %q accepted, want reject", v)
		}
	}
	if err := ValidCSSVar("--x", "#556b2f"); err != nil {
		t.Errorf("plain value rejected: %v", err)
	}
}

func TestValidateActionRefsTenantScoping(t *testing.T) {
	ours, foreign := sharedkernel.NewID(), sharedkernel.NewID()
	refs := ActionRefs{
		MenuIDs:     map[sharedkernel.ID]bool{ours: true},
		CategoryIDs: map[sharedkernel.ID]bool{ours: true},
		ItemIDs:     map[sharedkernel.ID]bool{ours: true},
		ImagePrefix: "http://localhost:9000/aivo-menu-images/",
	}

	ok := []AssistantAction{
		{Type: ActionCreateCategory, MenuID: &ours, Name: "X"},
		{Type: ActionDeleteItem, ID: &ours},
		{Type: ActionCreateItem, CategoryID: &ours, Name: "X", PriceCents: ptr(1),
			ImageURL: ptr("http://localhost:9000/aivo-menu-images/r/img.jpg")},
	}
	for _, a := range ok {
		if err := ValidateActionRefs(a, refs); err != nil {
			t.Errorf("%s: unexpected reject: %v", a.Type, err)
		}
	}

	bad := []AssistantAction{
		{Type: ActionCreateCategory, MenuID: &foreign, Name: "X"}, // foreign menu
		{Type: ActionDeleteItem, ID: &foreign},                    // foreign item
		{Type: ActionRenameCategory, ID: &foreign, Name: "X"},     // foreign category
		{Type: ActionCreateItem, CategoryID: &ours, Name: "X", PriceCents: ptr(1),
			ImageURL: ptr("http://evil.example/x.jpg")}, // off-host image
	}
	for _, a := range bad {
		if err := ValidateActionRefs(a, refs); !errors.Is(err, ErrInvalidAction) {
			t.Errorf("%s: got %v, want ErrInvalidAction", a.Type, err)
		}
	}

	// No image storage configured: every image_url is rejected.
	refs.ImagePrefix = ""
	a := AssistantAction{Type: ActionCreateItem, CategoryID: &ours, Name: "X", PriceCents: ptr(1),
		ImageURL: ptr("http://localhost:9000/aivo-menu-images/r/img.jpg")}
	if err := ValidateActionRefs(a, refs); !errors.Is(err, ErrInvalidAction) {
		t.Errorf("no image prefix: got %v, want ErrInvalidAction", err)
	}
}

func TestAssistantRoleValid(t *testing.T) {
	cases := []struct {
		r    AssistantRole
		want bool
	}{
		{AssistantRoleUser, true}, {AssistantRoleAssistant, true}, {"", false}, {"bogus", false},
	}
	for _, c := range cases {
		if got := c.r.Valid(); got != c.want {
			t.Errorf("AssistantRole(%q).Valid() = %v, want %v", c.r, got, c.want)
		}
	}
	if AssistantRole("").Default() != AssistantRoleUser {
		t.Errorf("AssistantRole default = %q, want %q", AssistantRole("").Default(), AssistantRoleUser)
	}
}

func TestActionStatusValid(t *testing.T) {
	cases := []struct {
		s    ActionStatus
		want bool
	}{
		{ActionStatusApplied, true}, {ActionStatusDiscarded, true}, {"", false}, {"bogus", false},
	}
	for _, c := range cases {
		if got := c.s.Valid(); got != c.want {
			t.Errorf("ActionStatus(%q).Valid() = %v, want %v", c.s, got, c.want)
		}
	}
	if ActionStatus("").Default() != "" {
		t.Errorf("ActionStatus default = %q, want empty (pending)", ActionStatus("").Default())
	}
}

func TestActionTypeValid(t *testing.T) {
	cases := []struct {
		a    ActionType
		want bool
	}{
		{ActionCreateCategory, true}, {ActionRenameCategory, true}, {ActionDeleteCategory, true},
		{ActionCreateItem, true}, {ActionUpdateItem, true}, {ActionDeleteItem, true},
		{ActionSetItemAvailable, true}, {ActionUpdateTheme, true}, {ActionCreateMenu, true},
		{"", false}, {"bogus", false},
	}
	for _, c := range cases {
		if got := c.a.Valid(); got != c.want {
			t.Errorf("ActionType(%q).Valid() = %v, want %v", c.a, got, c.want)
		}
	}
	if ActionType("").Default() != ActionCreateCategory {
		t.Errorf("ActionType default = %q, want %q", ActionType("").Default(), ActionCreateCategory)
	}
}
