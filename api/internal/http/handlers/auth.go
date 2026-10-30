package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type AuthHandler struct {
	Deps
}

func (h *AuthHandler) limits() *mw.AuthLimits {
	if h.AuthLimits == nil {
		return mw.NewAuthLimits()
	}
	return h.AuthLimits
}

func (h *AuthHandler) Routes(r chi.Router) {
	limits := h.limits()
	h.AuthLimits = limits
	r.With(limits.IP()).Post("/auth/login", h.Login)
	r.With(mw.OptionalAuth(h.Store)).Post("/auth/logout", h.Logout)
	r.With(mw.RequireAuth(h.Store)).Get("/auth/me", h.Me)
	r.With(mw.RequireAuth(h.Store)).Patch("/auth/me", h.UpdateMe)
	r.With(mw.RequireAuth(h.Store), limits.IP()).Post("/auth/change-password", h.ChangePassword)
}

func (h *AuthHandler) startSession(w http.ResponseWriter, r *http.Request, u domain.User) error {
	expires := time.Now().Add(domain.SessionTTL).UTC()
	token, err := h.Store.CreateSession(r.Context(), u.ID, expires)
	if err != nil {
		return err
	}
	http.SetCookie(w, mw.NewSessionCookie(token, expires))
	return nil
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	email, ok := normalizeEmail(body.Email)
	if !ok || body.Password == "" {
		auth.DummyVerify(body.Password)
		httpErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !h.AuthLimits.AllowEmail(email) {
		tooManyRequests(w)
		return
	}
	u, err := h.Store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			auth.DummyVerify(body.Password)
			httpErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		internalErr(w, r, err, "login failed")
		return
	}
	if u.PasswordHash == nil {
		auth.DummyVerify(body.Password)
		httpErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	valid, err := auth.VerifyPassword(body.Password, *u.PasswordHash)
	if err != nil || !valid {
		httpErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.DisabledAt != nil {
		httpErr(w, http.StatusUnauthorized, "account disabled")
		return
	}
	if err := h.startSession(w, r, u); err != nil {
		internalErr(w, r, err, "session create failed")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := mw.SessionTokenFrom(r.Context()); ok {
		if err := h.Store.DeleteSession(r.Context(), token); err != nil {
			log.Printf("logout: delete session: %v", err)
		}
	}
	http.SetCookie(w, mw.ClearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, mw.UserFrom(r.Context()))
}

func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	var body struct {
		Email       *string `json:"email"`
		DisplayName *string `json:"display_name"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if body.Email != nil {
		email, ok := normalizeEmail(*body.Email)
		if !ok {
			httpErr(w, http.StatusBadRequest, "invalid email")
			return
		}
		body.Email = &email
	}
	if body.DisplayName != nil {
		name := cleanName(*body.DisplayName)
		if name == "" || len(name) > 200 {
			httpErr(w, http.StatusBadRequest, "invalid display name")
			return
		}
		body.DisplayName = &name
	}
	nu, err := h.Store.UpdateUserProfile(r.Context(), u.ID, body.Email, body.DisplayName)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpErr(w, http.StatusConflict, "email already in use")
			return
		}
		internalErr(w, r, err, "failed to update profile")
		return
	}
	writeJSON(w, http.StatusOK, nu)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if !h.AuthLimits.AllowEmail(strings.ToLower(u.Email)) {
		tooManyRequests(w)
		return
	}
	if err := auth.ValidatePassword(body.NewPassword); err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := h.Store.GetUserWithHash(r.Context(), u.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load user")
		return
	}
	if full.PasswordHash == nil {
		httpErr(w, http.StatusBadRequest, "this account has no local password")
		return
	}
	valid, err := auth.VerifyPassword(body.CurrentPassword, *full.PasswordHash)
	if err != nil || !valid {
		httpErr(w, http.StatusUnauthorized, "current password incorrect")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Store.SetUserPassword(r.Context(), u.ID, hash); err != nil {
		internalErr(w, r, err, "failed to set password")
		return
	}
	token, _ := mw.SessionTokenFrom(r.Context())
	if err := h.Store.DeleteOtherUserSessions(r.Context(), u.ID, token); err != nil {
		internalErr(w, r, err, "failed to revoke sessions")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func tooManyRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	httpErr(w, http.StatusTooManyRequests, "too many requests")
}
