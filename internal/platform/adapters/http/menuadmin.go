package http

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	menudomain "aivo/internal/menu/domain"
	"aivo/internal/platform/app"
	"aivo/internal/platform/domain"
	"aivo/pkg/qrcode"

	"github.com/google/uuid"
)

// Shapes here match the admin client (web/admin/src/api/types.ts): bare
// arrays for lists, option groups as {name, type: single|multi, choices}.

// --- Categories --------------------------------------------------------

type categoryView struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
}

func (h *handler) listCategories(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	cats, _, err := h.Menu.Menu(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]categoryView, len(cats))
	for i, c := range cats {
		views[i] = categoryView{ID: c.ID, Name: c.Name, Position: c.Position}
	}
	writeJSON(w, http.StatusOK, views)
}

type categoryRequest struct {
	Name     string `json:"name"`
	Position *int   `json:"position"`
}

func (h *handler) createCategory(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req categoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "name is required")
		return
	}
	position := 0
	if req.Position != nil {
		position = *req.Position
	} else {
		// Default: append after the existing categories.
		cats, _, err := h.Menu.Menu(r.Context(), rest.ID)
		if writeAppErr(w, err) {
			return
		}
		for _, c := range cats {
			if c.Position >= position {
				position = c.Position + 1
			}
		}
	}
	c := menudomain.Category{ID: uuid.New(), RestaurantID: rest.ID, Name: strings.TrimSpace(req.Name), Position: position}
	if writeAppErr(w, h.MenuAdmin.CreateCategory(r.Context(), c)) {
		return
	}
	writeJSON(w, http.StatusCreated, categoryView{ID: c.ID, Name: c.Name, Position: c.Position})
}

func (h *handler) updateCategory(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("catID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	// Partial patch: start from the current row.
	cats, _, err := h.Menu.Menu(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	var current *menudomain.Category
	for i := range cats {
		if cats[i].ID == id {
			current = &cats[i]
			break
		}
	}
	if current == nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Position *int    `json:"position"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", "name cannot be empty")
			return
		}
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Position != nil {
		current.Position = *req.Position
	}
	if writeAppErr(w, h.MenuAdmin.UpdateCategory(r.Context(), *current)) {
		return
	}
	writeJSON(w, http.StatusOK, categoryView{ID: current.ID, Name: current.Name, Position: current.Position})
}

func (h *handler) deleteCategory(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("catID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if writeAppErr(w, h.MenuAdmin.DeleteCategory(r.Context(), rest.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Items -------------------------------------------------------------

type choiceView struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	PriceDeltaCents int       `json:"price_delta_cents"`
}

type optionGroupView struct {
	ID      uuid.UUID    `json:"id"`
	Name    string       `json:"name"`
	Type    string       `json:"type"` // "single" | "multi"
	Choices []choiceView `json:"choices"`
}

type itemView struct {
	ID           uuid.UUID         `json:"id"`
	CategoryID   uuid.UUID         `json:"category_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	PriceCents   int               `json:"price_cents"`
	ImageURL     string            `json:"image_url"`
	Allergens    []string          `json:"allergens"`
	OptionGroups []optionGroupView `json:"option_groups"`
	Available    bool              `json:"available"`
}

func toItemView(it menudomain.MenuItem) itemView {
	allergens := make([]string, len(it.Allergens))
	for i, a := range it.Allergens {
		allergens[i] = string(a)
	}
	groups := make([]optionGroupView, len(it.OptionGroups))
	for i, g := range it.OptionGroups {
		typ := "single"
		if g.Multi {
			typ = "multi"
		}
		choices := make([]choiceView, len(g.Options))
		for j, o := range g.Options {
			choices[j] = choiceView{ID: o.ID, Name: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		groups[i] = optionGroupView{ID: g.ID, Name: g.Name, Type: typ, Choices: choices}
	}
	return itemView{
		ID: it.ID, CategoryID: it.CategoryID, Name: it.Name, Description: it.Description,
		PriceCents: it.PriceCents, ImageURL: it.ImageURL, Allergens: allergens,
		OptionGroups: groups, Available: it.Available,
	}
}

// itemPatch is Partial<MenuItem> from the admin client; nil = keep.
type itemPatch struct {
	CategoryID   *uuid.UUID `json:"category_id"`
	Name         *string    `json:"name"`
	Description  *string    `json:"description"`
	PriceCents   *int       `json:"price_cents"`
	ImageURL     *string    `json:"image_url"`
	Allergens    *[]string  `json:"allergens"`
	OptionGroups *[]struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Choices []struct {
			Name            string `json:"name"`
			PriceDeltaCents int    `json:"price_delta_cents"`
		} `json:"choices"`
	} `json:"option_groups"`
	Available *bool `json:"available"`
}

// apply merges the patch into it, validating as it goes.
func (p itemPatch) apply(it *menudomain.MenuItem) error {
	if p.CategoryID != nil {
		if *p.CategoryID == uuid.Nil {
			return fmt.Errorf("category_id is required")
		}
		it.CategoryID = *p.CategoryID
	}
	if p.Name != nil {
		if strings.TrimSpace(*p.Name) == "" {
			return fmt.Errorf("name cannot be empty")
		}
		it.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		it.Description = *p.Description
	}
	if p.PriceCents != nil {
		if *p.PriceCents < 0 {
			return fmt.Errorf("price_cents must be >= 0")
		}
		it.PriceCents = *p.PriceCents
	}
	if p.ImageURL != nil {
		it.ImageURL = *p.ImageURL
	}
	if p.Allergens != nil {
		allergens := make([]menudomain.Allergen, 0, len(*p.Allergens))
		for _, a := range *p.Allergens {
			al := menudomain.Allergen(a)
			if !menudomain.ValidAllergen(al) {
				return fmt.Errorf("unknown allergen %q", a)
			}
			allergens = append(allergens, al)
		}
		it.Allergens = allergens
	}
	if p.OptionGroups != nil {
		groups := make([]menudomain.OptionGroup, 0, len(*p.OptionGroups))
		for _, g := range *p.OptionGroups {
			if strings.TrimSpace(g.Name) == "" {
				return fmt.Errorf("option group name is required")
			}
			opts := make([]menudomain.Option, 0, len(g.Choices))
			for _, c := range g.Choices {
				if strings.TrimSpace(c.Name) == "" {
					return fmt.Errorf("option choice name is required")
				}
				opts = append(opts, menudomain.Option{ID: uuid.New(), Label: c.Name, PriceDeltaCents: c.PriceDeltaCents})
			}
			groups = append(groups, menudomain.OptionGroup{
				ID: uuid.New(), Name: g.Name, Multi: g.Type == "multi", Options: opts,
			})
		}
		it.OptionGroups = groups
	}
	if p.Available != nil {
		it.Available = *p.Available
	}
	return nil
}

func (h *handler) listItems(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	_, items, err := h.Menu.Menu(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]itemView, len(items))
	for i, it := range items {
		views[i] = toItemView(it)
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *handler) createItem(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var patch itemPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	it := menudomain.MenuItem{ID: uuid.New(), RestaurantID: rest.ID, Available: true}
	if err := patch.apply(&it); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}
	if it.Name == "" || it.CategoryID == uuid.Nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "name and category_id are required")
		return
	}

	// Plan limit (free: 30 items per restaurant).
	limit, err := h.Platform.ItemLimitFor(r.Context(), u.OrgID)
	if writeAppErr(w, err) {
		return
	}
	if limit > 0 {
		n, err := h.MenuAdmin.CountMenuItems(r.Context(), rest.ID)
		if writeAppErr(w, err) {
			return
		}
		if n >= limit {
			writeAppErr(w, fmt.Errorf("%w: plan allows %d menu items", app.ErrPlanLimit, limit))
			return
		}
	}

	if writeAppErr(w, h.MenuAdmin.CreateMenuItem(r.Context(), it)) {
		return
	}
	writeJSON(w, http.StatusCreated, toItemView(it))
}

func (h *handler) updateItem(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("itemID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	it, err := h.MenuAdmin.MenuItemByID(r.Context(), rest.ID, id)
	if writeAppErr(w, err) {
		return
	}
	var patch itemPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	if err := patch.apply(&it); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}
	if writeAppErr(w, h.MenuAdmin.UpdateMenuItem(r.Context(), it)) {
		return
	}
	writeJSON(w, http.StatusOK, toItemView(it))
}

func (h *handler) deleteItem(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("itemID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if writeAppErr(w, h.MenuAdmin.DeleteMenuItem(r.Context(), rest.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tables ------------------------------------------------------------

// newTableToken returns a ~128-bit random URL-safe table token, per
// internal/menu/CONTEXT.md "Table link".
func newTableToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("table token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type tableView struct {
	ID        uuid.UUID `json:"id"`
	Label     string    `json:"label"`
	Token     string    `json:"token"`
	Link      string    `json:"link"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *handler) tableLink(slug, token string) string {
	return h.BaseURL + "/" + slug + "/t/" + token
}

func (h *handler) toTableView(rest domain.Restaurant, t menudomain.Table) tableView {
	return tableView{ID: t.ID, Label: t.Label, Token: t.Token, Link: h.tableLink(rest.Slug, t.Token), CreatedAt: t.CreatedAt}
}

func (h *handler) listTables(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	tables, err := h.MenuAdmin.Tables(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]tableView, len(tables))
	for i, t := range tables {
		views[i] = h.toTableView(rest, t)
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *handler) createTable(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req struct {
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "label is required")
		return
	}
	token, err := newTableToken()
	if writeAppErr(w, err) {
		return
	}
	t := menudomain.Table{ID: uuid.New(), RestaurantID: rest.ID, Label: strings.TrimSpace(req.Label), Token: token}
	if writeAppErr(w, h.MenuAdmin.CreateTable(r.Context(), t)) {
		return
	}
	t.CreatedAt = time.Now().UTC()
	writeJSON(w, http.StatusCreated, h.toTableView(rest, t))
}

func (h *handler) regenerateTable(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("tableID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	token, err := newTableToken()
	if writeAppErr(w, err) {
		return
	}
	if writeAppErr(w, h.MenuAdmin.RegenerateTableToken(r.Context(), rest.ID, id, token)) {
		return
	}
	t, err := h.MenuAdmin.TableByID(r.Context(), rest.ID, id)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, h.toTableView(rest, t))
}

func (h *handler) tableQR(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	id, err := uuid.Parse(r.PathValue("tableID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	t, err := h.MenuAdmin.TableByID(r.Context(), rest.ID, id)
	if writeAppErr(w, err) {
		return
	}
	png, err := qrcode.PNG(h.tableLink(rest.Slug, t.Token), 512)
	if writeAppErr(w, err) {
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}
