package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
)

type ActivityHandler struct {
	Deps
}

func (h *ActivityHandler) Routes(r chi.Router) {
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/activity", h.List)
}

func activityParseCursor(raw string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(raw, " ", "+"))
	if err != nil {
		return nil, false
	}
	return &t, true
}

func (h *ActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	before, ok := activityParseCursor(r.URL.Query().Get("before"))
	if !ok {
		httpErr(w, http.StatusBadRequest, "invalid before")
		return
	}
	limit := queryInt(r, "limit", 30, 1, 100)
	items, err := h.Store.ListSpaceActivity(r.Context(), sc.ID, u.ID, sc.Role == domain.RoleAdmin, before, limit)
	if err != nil {
		internalErr(w, r, err, "failed to load activity")
		return
	}
	writeJSON(w, http.StatusOK, items)
}
