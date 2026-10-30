package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type UserHandler struct {
	Deps
}

func (h *UserHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/users", h.Search)
	r.With(mw.RequireAuth(h.Store)).Get("/users/{userID}", h.Get)
}

func (h *UserHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 20, 1, 50)
	refs, err := h.Store.SearchUserRefs(r.Context(), q, limit)
	if err != nil {
		internalErr(w, r, err, "failed to search users")
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := urlUUID(r, "userID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	ref, err := h.Store.GetUserRef(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to load user")
		return
	}
	writeJSON(w, http.StatusOK, ref)
}
