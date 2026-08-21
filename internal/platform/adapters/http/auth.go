package http

import (
	"net/http"
	"time"

	"aivo/internal/platform/app"
	"aivo/internal/platform/domain"

	"github.com/google/uuid"
)

// authedFunc is a handler that runs with a resolved session user.
type authedFunc func(w http.ResponseWriter, r *http.Request, u domain.User)

// restaurantFunc additionally runs with a tenant-checked restaurant.
type restaurantFunc func(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant)

// auth resolves the aivo_session cookie to a user or 401s.
func (h *handler) auth(next authedFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := h.sessionUser(r)
		if writeAppErr(w, err) {
			return
		}
		next(w, r, u)
	}
}

// manage is auth + owner/manager role.
func (h *handler) manage(next authedFunc) http.HandlerFunc {
	return h.auth(func(w http.ResponseWriter, r *http.Request, u domain.User) {
		if !u.CanManage() {
			writeAppErr(w, app.ErrForbidden)
			return
		}
		next(w, r, u)
	})
}

// restaurant is auth + tenant scope on the {id} path segment: the
// restaurant must belong to the user's org (store-checked, 404 on
// foreign IDs) and to the user's restaurant scope (403 for staff of a
// sibling restaurant). needManage additionally requires owner/manager.
func (h *handler) restaurant(needManage bool, next restaurantFunc) http.HandlerFunc {
	return h.auth(func(w http.ResponseWriter, r *http.Request, u domain.User) {
		if needManage && !u.CanManage() {
			writeAppErr(w, app.ErrForbidden)
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		rest, err := h.Platform.Restaurant(r.Context(), u.OrgID, id)
		if writeAppErr(w, err) {
			return
		}
		if !u.CanAccessRestaurant(rest.ID) {
			writeAppErr(w, app.ErrForbidden)
			return
		}
		next(w, r, u, rest)
	})
}

func (h *handler) sessionUser(r *http.Request) (domain.User, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return domain.User{}, app.ErrUnauthorized
	}
	return h.Platform.UserByToken(r.Context(), c.Value)
}

func setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- Views -------------------------------------------------------------

type userView struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	RestaurantID *uuid.UUID `json:"restaurant_id"`
}

func toUserView(u domain.User) userView {
	return userView{ID: u.ID, OrgID: u.OrgID, Email: u.Email, Role: string(u.Role), RestaurantID: u.RestaurantID}
}

// --- Handlers ----------------------------------------------------------

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgName        string `json:"org_name"`
		RestaurantName string `json:"restaurant_name"`
		Email          string `json:"email"`
		Password       string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, token, err := h.Platform.Register(r.Context(), req.OrgName, req.RestaurantName, req.Email, req.Password)
	if writeAppErr(w, err) {
		return
	}
	setSessionCookie(w, token, app.SessionTTL)
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserView(u)})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, token, err := h.Platform.Login(r.Context(), req.Email, req.Password)
	if writeAppErr(w, err) {
		return
	}
	setSessionCookie(w, token, app.SessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(u)})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if err := h.Platform.Logout(r.Context(), c.Value); err != nil {
			writeAppErr(w, err)
			return
		}
	}
	setSessionCookie(w, "", -time.Hour) // negative MaxAge deletes the cookie
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *handler) me(w http.ResponseWriter, r *http.Request, u domain.User) {
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(u)})
}
