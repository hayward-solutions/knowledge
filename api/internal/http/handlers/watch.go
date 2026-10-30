package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type WatchHandler struct {
	Deps
}

func (h *WatchHandler) Routes(r chi.Router) {
	auth := mw.RequireAuth(h.Store)
	r.With(auth).Get("/watch/{type}/{id}", h.Get)
	r.With(auth).Put("/watch/{type}/{id}", h.Put)
	r.With(auth).Delete("/watch/{type}/{id}", h.Delete)
}

type watchState struct {
	Watching bool `json:"watching"`
}

func watchEffectiveRole(u *domain.User, a store.SpaceAccess) (domain.Role, bool) {
	switch {
	case u.IsSystemAdmin:
		return domain.RoleAdmin, true
	case a.MemberRole != nil && a.MemberRole.Valid():
		return *a.MemberRole, true
	case a.Visibility == domain.VisibilityInternal:
		return domain.RoleViewer, true
	}
	return "", false
}

func (h *WatchHandler) target(w http.ResponseWriter, r *http.Request) (*domain.User, domain.WatchTarget, uuid.UUID, bool) {
	u := mw.UserFrom(r.Context())
	if u == nil {
		httpErr(w, http.StatusUnauthorized, "unauthenticated")
		return nil, "", uuid.Nil, false
	}
	kind := domain.WatchTarget(chi.URLParam(r, "type"))
	if !kind.Valid() {
		httpErr(w, http.StatusBadRequest, "invalid watch type")
		return nil, "", uuid.Nil, false
	}
	id, err := urlUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return nil, "", uuid.Nil, false
	}
	var access store.SpaceAccess
	if kind == domain.WatchTargetPage {
		access, err = h.Store.GetSpaceAccessByPage(r.Context(), id, u.ID)
	} else {
		access, err = h.Store.GetSpaceAccessByID(r.Context(), id, u.ID)
	}
	if err != nil {
		watchStoreErr(w, r, err)
		return nil, "", uuid.Nil, false
	}
	if access.DeletedAt != nil && !u.IsSystemAdmin {
		httpErr(w, http.StatusNotFound, "not found")
		return nil, "", uuid.Nil, false
	}
	role, ok := watchEffectiveRole(u, access)
	if !ok {
		httpErr(w, http.StatusNotFound, "not found")
		return nil, "", uuid.Nil, false
	}
	if kind == domain.WatchTargetPage {
		page, err := h.Store.GetPage(r.Context(), id)
		if err != nil {
			watchStoreErr(w, r, err)
			return nil, "", uuid.Nil, false
		}
		if !store.IsPageVisible(page, u.ID, role) {
			httpErr(w, http.StatusNotFound, "not found")
			return nil, "", uuid.Nil, false
		}
	}
	return u, kind, id, true
}

func watchStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	internalErr(w, r, err, "failed to resolve watch target")
}

func (h *WatchHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, kind, id, ok := h.target(w, r)
	if !ok {
		return
	}
	watching, err := h.Store.IsWatching(r.Context(), u.ID, kind, id)
	if err != nil {
		internalErr(w, r, err, "failed to load watch state")
		return
	}
	writeJSON(w, http.StatusOK, watchState{Watching: watching})
}

func (h *WatchHandler) Put(w http.ResponseWriter, r *http.Request) {
	u, kind, id, ok := h.target(w, r)
	if !ok {
		return
	}
	if err := h.Store.SetWatch(r.Context(), u.ID, kind, id); err != nil {
		internalErr(w, r, err, "failed to watch")
		return
	}
	writeJSON(w, http.StatusOK, watchState{Watching: true})
}

func (h *WatchHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u, kind, id, ok := h.target(w, r)
	if !ok {
		return
	}
	if err := h.Store.RemoveWatch(r.Context(), u.ID, kind, id); err != nil {
		internalErr(w, r, err, "failed to unwatch")
		return
	}
	writeJSON(w, http.StatusOK, watchState{Watching: false})
}
