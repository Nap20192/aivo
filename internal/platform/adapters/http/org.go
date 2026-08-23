package http

import (
	"encoding/json"
	"net/http"
	"time"

	"aivo/internal/domain/platform"
	"aivo/internal/platform/app"

	"github.com/google/uuid"
)

type orgView struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// subscriptionView matches the admin client's Subscription. renews_at is
// fake-billing fiction: 30 days after the last change.
type subscriptionView struct {
	Plan     string `json:"plan"`
	Status   string `json:"status"`
	RenewsAt string `json:"renews_at"`
}

func toSubscriptionView(s domain.Subscription) subscriptionView {
	return subscriptionView{
		Plan:     string(s.Plan),
		Status:   string(s.Status),
		RenewsAt: s.UpdatedAt.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}
}

// restaurantView matches the admin client's Restaurant shape
// (web/admin/src/api/types.ts): structured hours, flat phone/instagram,
// custom_domain inline.
type restaurantView struct {
	ID           uuid.UUID         `json:"id"`
	OrgID        uuid.UUID         `json:"org_id"`
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Hours        []domain.HoursRow `json:"hours"`
	Address      string            `json:"address"`
	Phone        string            `json:"phone"`
	Instagram    string            `json:"instagram"`
	CustomDomain string            `json:"custom_domain"`
	CreatedAt    time.Time         `json:"created_at"`
}

func (h *handler) toRestaurantView(r *http.Request, rest domain.Restaurant) restaurantView {
	hours := rest.Hours
	if hours == nil {
		hours = []domain.HoursRow{}
	}
	// ponytail: one extra query per restaurant for the domain; orgs have
	// a handful of restaurants, join it into the store if that changes.
	cd, err := h.Platform.CustomDomain(r.Context(), rest.ID)
	if err != nil {
		cd = ""
	}
	return restaurantView{
		ID: rest.ID, OrgID: rest.OrgID, Slug: rest.Slug, Name: rest.Name,
		Hours: hours, Address: rest.Address,
		Phone: rest.Contacts["phone"], Instagram: rest.Contacts["instagram"],
		CustomDomain: cd, CreatedAt: rest.CreatedAt,
	}
}

func (h *handler) getOrg(w http.ResponseWriter, r *http.Request, u domain.User) {
	org, err := h.Platform.Organization(r.Context(), u.OrgID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, orgView{ID: org.ID, Name: org.Name, CreatedAt: org.CreatedAt})
}

func (h *handler) patchOrg(w http.ResponseWriter, r *http.Request, u domain.User) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	org, err := h.Platform.RenameOrganization(r.Context(), u.OrgID, req.Name)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, orgView{ID: org.ID, Name: org.Name, CreatedAt: org.CreatedAt})
}

func (h *handler) getSubscription(w http.ResponseWriter, r *http.Request, u domain.User) {
	sub, err := h.Platform.Subscription(r.Context(), u.OrgID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionView(sub))
}

func (h *handler) changePlan(w http.ResponseWriter, r *http.Request, u domain.User) {
	var req struct {
		Plan string `json:"plan"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := h.Platform.ChangePlan(r.Context(), u.OrgID, domain.Plan(req.Plan))
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionView(sub))
}

func (h *handler) listRestaurants(w http.ResponseWriter, r *http.Request, u domain.User) {
	views, err := h.restaurantViews(r, u)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, views)
}

// restaurantViews is the user's visible restaurants as admin-shaped
// views (also embedded in the auth Me responses).
func (h *handler) restaurantViews(r *http.Request, u domain.User) ([]restaurantView, error) {
	rests, err := h.Platform.Restaurants(r.Context(), u.OrgID)
	if err != nil {
		return nil, err
	}
	views := []restaurantView{}
	for _, rest := range rests {
		if u.CanAccessRestaurant(rest.ID) {
			views = append(views, h.toRestaurantView(r, rest))
		}
	}
	return views, nil
}

func (h *handler) createRestaurant(w http.ResponseWriter, r *http.Request, u domain.User) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	rest, err := h.Platform.CreateRestaurant(r.Context(), u.OrgID, req.Name, req.Slug)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, h.toRestaurantView(r, rest))
}

func (h *handler) getRestaurant(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	writeJSON(w, http.StatusOK, h.toRestaurantView(r, rest))
}

func (h *handler) patchRestaurant(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req struct {
		Slug         *string            `json:"slug"`
		Name         *string            `json:"name"`
		Address      *string            `json:"address"`
		Hours        *[]domain.HoursRow `json:"hours"`
		Phone        *string            `json:"phone"`
		Instagram    *string            `json:"instagram"`
		Contacts     map[string]string  `json:"contacts"`
		CustomDomain *string            `json:"custom_domain"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	updated, err := h.Platform.UpdateRestaurant(r.Context(), u.OrgID, rest.ID, app.RestaurantPatch{
		Slug: req.Slug, Name: req.Name, Address: req.Address, Hours: req.Hours,
		Phone: req.Phone, Instagram: req.Instagram, Contacts: req.Contacts,
		CustomDomain: req.CustomDomain,
	})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, h.toRestaurantView(r, updated))
}

// --- Theme -------------------------------------------------------------

// themeView is the admin client's flat Theme: structured keys + design_md
// in one object. Stored as theme JSON (everything but design_md) plus the
// design_md text column.
type themeView struct {
	BrandName string            `json:"brand_name"`
	Accent    string            `json:"accent"`
	Bold      bool              `json:"bold"`
	BannerURL string            `json:"banner_url"`
	CSSVars   map[string]string `json:"css_vars"`
	DesignMD  string            `json:"design_md"`
}

func validAccent(a string) bool {
	switch a {
	case "Blood red", "Olive", "Wine", "Fire":
		return true
	}
	return false
}

func toThemeView(t domain.Theme, restaurantName string) themeView {
	v := themeView{BrandName: restaurantName, Accent: "Blood red", CSSVars: map[string]string{}, DesignMD: t.DesignMD}
	var stored struct {
		BrandName *string           `json:"brand_name"`
		Accent    *string           `json:"accent"`
		Bold      *bool             `json:"bold"`
		BannerURL *string           `json:"banner_url"`
		CSSVars   map[string]string `json:"css_vars"`
	}
	if err := json.Unmarshal(t.ThemeJSON, &stored); err == nil {
		if stored.BrandName != nil {
			v.BrandName = *stored.BrandName
		}
		if stored.Accent != nil {
			v.Accent = *stored.Accent
		}
		if stored.Bold != nil {
			v.Bold = *stored.Bold
		}
		if stored.BannerURL != nil {
			v.BannerURL = *stored.BannerURL
		}
		if stored.CSSVars != nil {
			v.CSSVars = stored.CSSVars
		}
	}
	return v
}

func (h *handler) getTheme(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	t, err := h.Platform.Theme(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toThemeView(t, rest.Name))
}

func (h *handler) putTheme(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req themeView
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validAccent(req.Accent) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "accent must be one of: Blood red, Olive, Wine, Fire")
		return
	}
	if req.CSSVars == nil {
		req.CSSVars = map[string]string{}
	}
	themeJSON, err := json.Marshal(map[string]any{
		"brand_name": req.BrandName,
		"accent":     req.Accent,
		"bold":       req.Bold,
		"banner_url": req.BannerURL,
		"css_vars":   req.CSSVars,
	})
	if err != nil {
		writeAppErr(w, err)
		return
	}
	t, err := h.Platform.SaveTheme(r.Context(), domain.Theme{RestaurantID: rest.ID, ThemeJSON: themeJSON, DesignMD: req.DesignMD})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toThemeView(t, rest.Name))
}

// generateTheme proposes a theme from the stored design_md WITHOUT
// saving it — applying stays the explicit PUT above (AI must not
// silently control, per AGENTS.md).
func (h *handler) generateTheme(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	proposal, err := h.Platform.GenerateTheme(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"proposal": toThemeView(proposal, rest.Name),
		"based_on": "design_md",
	})
}

// --- Staff -------------------------------------------------------------

// staffView matches the admin client's StaffMember.
type staffView struct {
	ID     uuid.UUID `json:"id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	Status string    `json:"status"` // "active" | "invited"
}

func (h *handler) listStaff(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	staff, err := h.Platform.Staff(r.Context(), u.OrgID, rest.ID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]staffView, len(staff))
	for i, s := range staff {
		views[i] = staffView{ID: s.ID, Email: s.Email, Role: string(s.Role), Status: "active"}
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *handler) addStaff(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req struct {
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"` // optional; empty = invited with random password
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	staff, err := h.Platform.AddStaff(r.Context(), u.OrgID, rest.ID, req.Email, req.Password, domain.Role(req.Role))
	if writeAppErr(w, err) {
		return
	}
	status := "active"
	if req.Password == "" {
		status = "invited"
	}
	writeJSON(w, http.StatusCreated, staffView{ID: staff.ID, Email: staff.Email, Role: string(staff.Role), Status: status})
}

// --- Images ------------------------------------------------------------

func (h *handler) uploadImage(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	if h.Images == nil {
		writeErr(w, http.StatusServiceUnavailable, "images_unconfigured", "image storage is not configured")
		return
	}
	// 10MB cap for the whole multipart body.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	file, header, err := r.FormFile("image")
	if err != nil {
		// Accept "file" as a fallback field name.
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_upload", `multipart field "image" is required`)
		return
	}
	defer file.Close()

	// Never trust the declared multipart type: sniff the bytes, require
	// a real image, store the sniffed type.
	reader, sniffed, err := sniffUpload(file, header.Header.Get("Content-Type"))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}
	switch sniffed {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "content must be an image (jpeg, png, webp, gif)")
		return
	}

	url, err := h.Images.Put(r.Context(), rest.ID, header.Filename, sniffed, reader, header.Size)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}
