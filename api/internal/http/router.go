package http

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
)

func NewRouter(d handlers.Deps) http.Handler {
	mw.CookieSecure = d.Config != nil && d.Config.CookieSecure
	var origins []string
	var trusted []netip.Prefix
	if d.Config != nil {
		origins = d.Config.AppOrigins
		trusted = d.Config.TrustedProxies
	}
	if d.AuthLimits == nil {
		d.AuthLimits = mw.NewAuthLimits()
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(mw.RealIP(trusted))
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware(origins))
	r.Use(longRequests(longRequestTimeout))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		mw.WriteError(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		mw.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	authH := &handlers.AuthHandler{Deps: d}
	oidcH := &handlers.OIDCHandler{Deps: d}
	userH := &handlers.UserHandler{Deps: d}
	adminH := &handlers.AdminHandler{Deps: d}
	spaceH := &handlers.SpaceHandler{Deps: d}
	pageH := &handlers.PageHandler{Deps: d}
	versionH := &handlers.VersionHandler{Deps: d}
	commentH := &handlers.CommentHandler{Deps: d}
	labelH := &handlers.LabelHandler{Deps: d}
	searchH := &handlers.SearchHandler{Deps: d}
	attachmentH := &handlers.AttachmentHandler{Deps: d}
	templateH := &handlers.TemplateHandler{Deps: d}
	watchH := &handlers.WatchHandler{Deps: d}
	notificationH := &handlers.NotificationHandler{Deps: d}
	activityH := &handlers.ActivityHandler{Deps: d}
	dashboardH := &handlers.DashboardHandler{Deps: d}
	exportH := &handlers.ExportHandler{Deps: d}

	notificationH.Subscribe()

	version := d.Version
	if version == "" {
		version = "dev"
	}
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": version})
	})

	r.Route("/api/v1", func(r chi.Router) {
		authH.Routes(r)
		oidcH.Routes(r)
		userH.Routes(r)
		adminH.Routes(r)
		spaceH.Routes(r)
		pageH.Routes(r)
		versionH.Routes(r)
		commentH.Routes(r)
		labelH.Routes(r)
		searchH.Routes(r)
		attachmentH.Routes(r)
		templateH.Routes(r)
		watchH.Routes(r)
		notificationH.Routes(r)
		activityH.Routes(r)
		dashboardH.Routes(r)
		exportH.Routes(r)
	})

	return r
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allow[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allow[origin]; ok {
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
					w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

const longRequestTimeout = 5 * time.Minute

func longRequests(d time.Duration) func(http.Handler) http.Handler {
	extend := mw.LongRequest(d)
	return func(next http.Handler) http.Handler {
		long := extend(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/attachments") || strings.HasSuffix(r.URL.Path, "/import")) {
				long.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
