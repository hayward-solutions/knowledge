package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
)

const (
	oidcStateCookie = "oidc_state"
	oidcNonceCookie = "oidc_nonce"
	oidcNextCookie  = "oidc_next"
)

type OIDCHandler struct {
	Deps
}

func (h *OIDCHandler) Routes(r chi.Router) {
	r.Get("/auth/oidc/config", h.ConfigInfo)
	r.Get("/auth/oidc/login", h.Login)
	r.Get("/auth/oidc/callback", h.Callback)
}

func (h *OIDCHandler) publicURL() string {
	if h.Config == nil {
		return ""
	}
	return h.Config.PublicBaseURL
}

func (h *OIDCHandler) ConfigInfo(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"provider_name": h.OIDC.ProviderName,
	})
}

func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		http.NotFound(w, r)
		return
	}
	state := randString(24)
	nonce := randString(24)
	next := r.URL.Query().Get("next")

	setShortCookie(w, oidcStateCookie, state)
	setShortCookie(w, oidcNonceCookie, nonce)
	if safeNext(next) {
		setShortCookie(w, oidcNextCookie, next)
	}

	authURL := h.OIDC.OAuth2.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("nonce", nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	cfg := h.OIDC

	sc, err := r.Cookie(oidcStateCookie)
	if err != nil || sc.Value == "" || sc.Value != r.URL.Query().Get("state") {
		h.fail(w, r, "invalid state")
		return
	}
	clearCookie(w, oidcStateCookie)

	nc, err := r.Cookie(oidcNonceCookie)
	if err != nil || nc.Value == "" {
		h.fail(w, r, "missing nonce")
		return
	}
	clearCookie(w, oidcNonceCookie)

	code := r.URL.Query().Get("code")
	if code == "" {
		h.fail(w, r, "missing code")
		return
	}

	token, err := cfg.OAuth2.Exchange(ctx, code)
	if err != nil {
		h.fail(w, r, "token exchange failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		h.fail(w, r, "id_token missing")
		return
	}
	idToken, err := cfg.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		h.fail(w, r, "id_token invalid")
		return
	}
	if idToken.Nonce != nc.Value {
		h.fail(w, r, "nonce mismatch")
		return
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		h.fail(w, r, "claims parse failed")
		return
	}

	email := strings.TrimSpace(asString(claims[cfg.EmailClaim]))
	if email == "" {
		if ui, err := cfg.Provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
			var uiClaims map[string]any
			_ = ui.Claims(&uiClaims)
			email = strings.TrimSpace(asString(uiClaims[cfg.EmailClaim]))
			if email == "" {
				email = strings.TrimSpace(ui.Email)
			}
			if _, ok := claims[cfg.GroupsClaim]; !ok {
				if g, ok2 := uiClaims[cfg.GroupsClaim]; ok2 {
					claims[cfg.GroupsClaim] = g
				}
			}
		}
	}
	if email == "" {
		h.fail(w, r, "email claim missing")
		return
	}
	displayName := strings.TrimSpace(asString(claims[cfg.NameClaim]))
	if displayName == "" {
		displayName = email
	}

	var isAdmin *bool
	if cfg.AdminGroup != "" {
		v := auth.HasGroup(auth.GroupsFrom(claims[cfg.GroupsClaim]), cfg.AdminGroup)
		isAdmin = &v
	}

	u, err := h.Store.UpsertOIDCUser(ctx, cfg.Issuer, idToken.Subject, email, displayName, isAdmin)
	if err != nil {
		h.fail(w, r, "user provisioning failed")
		return
	}
	if u.DisabledAt != nil {
		h.fail(w, r, "account disabled")
		return
	}

	expires := time.Now().Add(domain.SessionTTL).UTC()
	sessionToken, err := h.Store.CreateSession(ctx, u.ID, expires)
	if err != nil {
		h.fail(w, r, "session create failed")
		return
	}
	http.SetCookie(w, mw.NewSessionCookie(sessionToken, expires))

	next := "/"
	if nextC, err := r.Cookie(oidcNextCookie); err == nil && safeNext(nextC.Value) {
		next = nextC.Value
	}
	clearCookie(w, oidcNextCookie)

	http.Redirect(w, r, h.publicURL()+next, http.StatusFound)
}

func (h *OIDCHandler) fail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, h.publicURL()+"/login?error="+url.QueryEscape(reason), http.StatusFound)
}

func setShortCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   mw.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   mw.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func safeNext(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") {
		return false
	}
	if strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return false
	}
	if strings.ContainsAny(p, "\r\n") {
		return false
	}
	return true
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	}
	return ""
}
