package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type AdminHandler struct {
	Deps
}

func (h *AdminHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Get("/admin/users", h.ListUsers)
	r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Post("/admin/users", h.CreateUser)
	r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Patch("/admin/users/{userID}", h.UpdateUser)
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.ListUsers(r.Context())
	if err != nil {
		internalErr(w, r, err, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		DisplayName   string `json:"display_name"`
		IsSystemAdmin bool   `json:"is_system_admin"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	email, ok := normalizeEmail(body.Email)
	if !ok {
		httpErr(w, http.StatusBadRequest, "invalid email")
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name := cleanName(body.DisplayName)
	if name == "" {
		name = email
	}
	if len(name) > 200 {
		httpErr(w, http.StatusBadRequest, "invalid display name")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := uuid.Must(uuid.NewV7())
	u := domain.User{
		ID:            id,
		Email:         email,
		DisplayName:   name,
		IsSystemAdmin: body.IsSystemAdmin,
	}
	if err := h.Store.CreateUser(r.Context(), u, hash); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpErr(w, http.StatusConflict, "email already in use")
			return
		}
		internalErr(w, r, err, "failed to create user")
		return
	}
	created, err := h.Store.GetUser(r.Context(), id)
	if err != nil {
		internalErr(w, r, err, "failed to load user")
		return
	}
	if actor := mw.UserFrom(r.Context()); actor != nil {
		log.Printf("audit admin.create_user actor=%s target=%s admin=%t", actor.ID, created.ID, created.IsSystemAdmin)
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := urlUUID(r, "userID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var body struct {
		Disabled      *bool   `json:"disabled"`
		Password      *string `json:"password"`
		Email         *string `json:"email"`
		DisplayName   *string `json:"display_name"`
		IsSystemAdmin *bool   `json:"is_system_admin"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	upd := store.UserUpdate{
		IsSystemAdmin: body.IsSystemAdmin,
		Disabled:      body.Disabled,
	}
	if body.Email != nil {
		email, ok := normalizeEmail(*body.Email)
		if !ok {
			httpErr(w, http.StatusBadRequest, "invalid email")
			return
		}
		upd.Email = &email
	}
	if body.DisplayName != nil {
		name := cleanName(*body.DisplayName)
		if name == "" || len(name) > 200 {
			httpErr(w, http.StatusBadRequest, "invalid display name")
			return
		}
		upd.DisplayName = &name
	}
	var newHash string
	if body.Password != nil {
		if err := auth.ValidatePassword(*body.Password); err != nil {
			httpErr(w, http.StatusBadRequest, err.Error())
			return
		}
		newHash, err = auth.HashPassword(*body.Password)
		if err != nil {
			httpErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if _, err := h.Store.AdminUpdateUser(r.Context(), id, upd); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			httpErr(w, http.StatusNotFound, "not found")
		case errors.Is(err, store.ErrLastAdmin):
			httpErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, store.ErrConflict):
			httpErr(w, http.StatusConflict, "email already in use")
		default:
			internalErr(w, r, err, "failed to update user")
		}
		return
	}
	if body.Password != nil {
		if err := h.Store.SetUserPassword(r.Context(), id, newHash); err != nil {
			internalErr(w, r, err, "failed to set password")
			return
		}
		if err := h.Store.DeleteUserSessions(r.Context(), id); err != nil {
			internalErr(w, r, err, "failed to revoke sessions")
			return
		}
	}
	u, err := h.Store.GetUser(r.Context(), id)
	if err != nil {
		internalErr(w, r, err, "failed to load user")
		return
	}
	if actor := mw.UserFrom(r.Context()); actor != nil {
		log.Printf("audit admin.update_user actor=%s target=%s email=%t display=%t admin=%t disabled=%t password=%t",
			actor.ID, id,
			body.Email != nil, body.DisplayName != nil, body.IsSystemAdmin != nil,
			body.Disabled != nil, body.Password != nil)
	}
	writeJSON(w, http.StatusOK, u)
}
