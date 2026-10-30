package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
)

type DashboardHandler struct {
	Deps
}

func (h *DashboardHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/me/dashboard", h.Get)
}

func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	d, err := h.Store.GetDashboard(r.Context(), u.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load dashboard")
		return
	}
	writeJSON(w, http.StatusOK, d)
}
