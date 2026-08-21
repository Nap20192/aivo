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
	writeJSON(w, http.StatusOK, map[string]any{"categories": views})
}

type categoryRequest struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
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
	c := menudomain.Category{ID: uuid.New(), RestaurantID: rest.ID, Name: strings.TrimSpace(req.Name), Position: req.Position}
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
	var req categoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "name is required")
		return
	}
	c := menudomain.Category{ID: id, RestaurantID: rest.ID, Name: strings.TrimSpace(req.Name), Position: req.Position}
	if writeAppErr(w, h.MenuAdmin.UpdateCategory(r.Context(), c)) {
		return
	}
	writeJSON(w, http.StatusOK, categoryView{ID: c.ID, Name: c.Name, Position: c.Position})
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Items -------------------------------------------------------------

type optionRequest struct {
	Label           string `json:"label"`
	PriceDeltaCents int    `json:"price_delta_cents"`
}

type optionGroupRequest struct {
	Name    string          `json:"name"`
	Multi   bool            `json:"multi"`
	Options []optionRequest `json:"options"`
}

type itemRequest struct {
	CategoryID   uuid.UUID            `json:"category_id"`
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	PriceCents   int                  `json:"price_cents"`
	ImageURL     string               `json:"image_url"`
	Allergens    []string             `json:"allergens"`
	OptionGroups []optionGroupRequest `json:"option_groups"`
	Available    *bool                `json:"available"`
}

func (req itemRequest) toDomain(restaurantID, id uuid.UUID) (menudomain.MenuItem, error) {
	if strings.TrimSpace(req.Name) == "" {
		return menudomain.MenuItem{}, fmt.Errorf("name is required")
	}
	if req.CategoryID == uuid.Nil {
		return menudomain.MenuItem{}, fmt.Errorf("category_id is required")
	}
	if req.PriceCents < 0 {
		return menudomain.MenuItem{}, fmt.Errorf("price_cents must be >= 0")
	}
	allergens := make([]menudomain.Allergen, 0, len(req.Allergens))
	for _, a := range req.Allergens {
		al := menudomain.Allergen(a)
		if !menudomain.ValidAllergen(al) {
			return menudomain.MenuItem{}, fmt.Errorf("unknown allergen %q", a)
		}
		allergens = append(allergens, al)
	}
	groups := make([]menudomain.OptionGroup, 0, len(req.OptionGroups))
	for _, g := range req.OptionGroups {
		if strings.TrimSpace(g.Name) == "" {
			return menudomain.MenuItem{}, fmt.Errorf("option group name is required")
		}
		opts := make([]menudomain.Option, 0, len(g.Options))
		for _, o := range g.Options {
			if strings.TrimSpace(o.Label) == "" {
				return menudomain.MenuItem{}, fmt.Errorf("option label is required")
			}
			opts = append(opts, menudomain.Option{ID: uuid.New(), Label: o.Label, PriceDeltaCents: o.PriceDeltaCents})
		}
		groups = append(groups, menudomain.OptionGroup{ID: uuid.New(), Name: g.Name, Multi: g.Multi, Options: opts})
	}
	available := true
	if req.Available != nil {
		available = *req.Available
	}
	return menudomain.MenuItem{
		ID: id, RestaurantID: restaurantID, CategoryID: req.CategoryID,
		Name: strings.TrimSpace(req.Name), Description: req.Description,
		PriceCents: req.PriceCents, ImageURL: req.ImageURL,
		Allergens: allergens, Available: available, OptionGroups: groups,
	}, nil
}

type optionView struct {
	ID              uuid.UUID `json:"id"`
	Label           string    `json:"label"`
	PriceDeltaCents int       `json:"price_delta_cents"`
}

type optionGroupView struct {
	ID      uuid.UUID    `json:"id"`
	Name    string       `json:"name"`
	Multi   bool         `json:"multi"`
	Options []optionView `json:"options"`
}

type itemView struct {
	ID           uuid.UUID         `json:"id"`
	CategoryID   uuid.UUID         `json:"category_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	PriceCents   int               `json:"price_cents"`
	ImageURL     string            `json:"image_url"`
	Allergens    []string          `json:"allergens"`
	Available    bool              `json:"available"`
	OptionGroups []optionGroupView `json:"option_groups"`
}

func toItemView(it menudomain.MenuItem) itemView {
	allergens := make([]string, len(it.Allergens))
	for i, a := range it.Allergens {
		allergens[i] = string(a)
	}
	groups := make([]optionGroupView, len(it.OptionGroups))
	for i, g := range it.OptionGroups {
		opts := make([]optionView, len(g.Options))
		for j, o := range g.Options {
			opts[j] = optionView{ID: o.ID, Label: o.Label, PriceDeltaCents: o.PriceDeltaCents}
		}
		groups[i] = optionGroupView{ID: g.ID, Name: g.Name, Multi: g.Multi, Options: opts}
	}
	return itemView{
		ID: it.ID, CategoryID: it.CategoryID, Name: it.Name, Description: it.Description,
		PriceCents: it.PriceCents, ImageURL: it.ImageURL, Allergens: allergens,
		Available: it.Available, OptionGroups: groups,
	}
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
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (h *handler) createItem(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req itemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	it, err := req.toDomain(rest.ID, uuid.New())
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
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
	var req itemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	it, err := req.toDomain(rest.ID, id)
	if err != nil {
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	writeJSON(w, http.StatusOK, map[string]any{"tables": views})
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
