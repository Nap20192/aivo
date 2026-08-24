package http

import (
	"testing"

	menudomain "aivo/internal/domain/menu"

	"uuid"
)

func fixtures() ([]menudomain.Menu, []menudomain.Category, []menudomain.MenuItem) {
	dinner := menudomain.Menu{ID: uuid.New(), Slug: "menu", Name: "Dinner", IsDefault: true}
	bar := menudomain.Menu{ID: uuid.New(), Slug: "bar", Name: "Bar", Position: 1}
	starters := menudomain.Category{ID: uuid.New(), MenuID: dinner.ID, Name: "Starters"}
	wine := menudomain.Category{ID: uuid.New(), MenuID: bar.ID, Name: "Wine"}
	items := []menudomain.MenuItem{
		{ID: uuid.New(), CategoryID: starters.ID, Name: "Marrow", PriceCents: 1400, Available: true},
		{ID: uuid.New(), CategoryID: wine.ID, Name: "Malbec", PriceCents: 1400, Available: true},
		{ID: uuid.New(), CategoryID: wine.ID, Name: "86'd wine", PriceCents: 1200, Available: false},
	}
	return []menudomain.Menu{dinner, bar}, []menudomain.Category{starters, wine}, items
}

func TestBuildDinerMenusGroupsByMenu(t *testing.T) {
	menus, cats, items := fixtures()
	got := buildDinerMenus(menus, cats, items)
	if len(got) != 2 {
		t.Fatalf("menus = %d, want 2", len(got))
	}
	if !got[0].IsDefault || got[0].Name != "Dinner" {
		t.Errorf("default menu not first: %+v", got[0])
	}
	if len(got[0].Categories) != 1 || got[0].Categories[0].Name != "Starters" {
		t.Errorf("dinner categories = %+v", got[0].Categories)
	}
	if len(got[1].Categories) != 1 || len(got[1].Categories[0].Items) != 2 {
		t.Errorf("bar categories = %+v", got[1].Categories)
	}
}

func TestPosMenuPrefixesWhenMultipleMenus(t *testing.T) {
	menus, cats, items := fixtures()

	got := posMenu(menus, cats, items)
	if len(got) != 2 {
		t.Fatalf("categories = %d, want 2", len(got))
	}
	if got[0].Name != "Dinner · Starters" || got[1].Name != "Bar · Wine" {
		t.Errorf("labels = %q, %q", got[0].Name, got[1].Name)
	}
	// 86'd item excluded from POS sheet.
	if len(got[1].Items) != 1 || got[1].Items[0].Name != "Malbec" {
		t.Errorf("bar items = %+v", got[1].Items)
	}

	// Single menu: no prefix.
	solo := posMenu(menus[:1], cats, items)
	if len(solo) != 1 || solo[0].Name != "Starters" {
		t.Errorf("single-menu labels = %+v", solo)
	}
}
