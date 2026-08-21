package http

import (
	"encoding/json"
	"net/http"
	"time"

	"aivo/internal/platform/app"
	"aivo/internal/platform/domain"

	"github.com/google/uuid"
)

type orgView struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type subscriptionView struct {
	Plan      string    `json:"plan"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type restaurantView struct {
	ID        uuid.UUID         `json:"id"`
	Slug      string            `json:"slug"`
	Name      string            `json:"name"`
	Address   string            `json:"address"`
	Hours     string            `json:"hours"`
	Contacts  map[string]string `json:"contacts"`
	CreatedAt time.Time         `json:"created_at"`
}

func toRestaurantView(r domain.Restaurant) restaurantView {
	if r.Contacts == nil {
		r.Contacts = map[string]string{}
	}
	return restaurantView{ID: r.ID, Slug: r.Slug, Name: r.Name, Address: r.Address, Hours: r.Hours, Contacts: r.Contacts, CreatedAt: r.CreatedAt}
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
	writeJSON(w, http.StatusOK, subscriptionView{Plan: string(sub.Plan), Status: string(sub.Status), UpdatedAt: sub.UpdatedAt})
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
	writeJSON(w, http.StatusOK, subscriptionView{Plan: string(sub.Plan), Status: string(sub.Status), UpdatedAt: sub.UpdatedAt})
}

func (h *handler) listRestaurants(w http.ResponseWriter, r *http.Request, u domain.User) {
	rests, err := h.Platform.Restaurants(r.Context(), u.OrgID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]restaurantView, 0, len(rests))
	for _, rest := range rests {
		if u.CanAccessRestaurant(rest.ID) {
			views = append(views, toRestaurantView(rest))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"restaurants": views})
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
	writeJSON(w, http.StatusCreated, toRestaurantView(rest))
}

func (h *handler) getRestaurant(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	writeJSON(w, http.StatusOK, toRestaurantView(rest))
}

func (h *handler) patchRestaurant(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req struct {
		Slug     *string           `json:"slug"`
		Name     *string           `json:"name"`
		Address  *string           `json:"address"`
		Hours    *string           `json:"hours"`
		Contacts map[string]string `json:"contacts"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	updated, err := h.Platform.UpdateRestaurant(r.Context(), u.OrgID, rest.ID, app.RestaurantPatch{
		Slug: req.Slug, Name: req.Name, Address: req.Address, Hours: req.Hours, Contacts: req.Contacts,
	})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, toRestaurantView(updated))
}

func (h *handler) getTheme(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	t, err := h.Platform.Theme(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme": json.RawMessage(t.ThemeJSON), "design_md": t.DesignMD})
}

func (h *handler) putTheme(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	var req struct {
		Theme    json.RawMessage `json:"theme"`
		DesignMD string          `json:"design_md"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.Platform.SaveTheme(r.Context(), domain.Theme{RestaurantID: rest.ID, ThemeJSON: req.Theme, DesignMD: req.DesignMD})
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme": json.RawMessage(t.ThemeJSON), "design_md": t.DesignMD})
}

func (h *handler) listStaff(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	staff, err := h.Platform.Staff(r.Context(), u.OrgID, rest.ID)
	if writeAppErr(w, err) {
		return
	}
	views := make([]userView, len(staff))
	for i, s := range staff {
		views[i] = toUserView(s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": views})
}

func (h *handler) addStaff(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	var req struct {
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	staff, err := h.Platform.AddStaff(r.Context(), u.OrgID, rest.ID, req.Email, req.Password, domain.Role(req.Role))
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toUserView(staff))
}

func (h *handler) uploadImage(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	if h.Images == nil {
		writeErr(w, http.StatusServiceUnavailable, "images_unconfigured", "image storage is not configured")
		return
	}
	// 10MB cap for the whole multipart body.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_upload", `multipart field "file" is required`)
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	switch ct {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "content type must be an image (jpeg, png, webp, gif)")
		return
	}

	url, err := h.Images.Put(r.Context(), rest.ID, header.Filename, ct, file, header.Size)
	if writeAppErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}
