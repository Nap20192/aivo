package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func ptr[T any](v T) *T { return &v }

func TestValidateActionAllowlist(t *testing.T) {
	id := uuid.New()
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
	ours, foreign := uuid.New(), uuid.New()
	refs := ActionRefs{
		MenuIDs:     map[uuid.UUID]bool{ours: true},
		CategoryIDs: map[uuid.UUID]bool{ours: true},
		ItemIDs:     map[uuid.UUID]bool{ours: true},
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
