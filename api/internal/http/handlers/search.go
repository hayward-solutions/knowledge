package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/search"
)

type SearchHandler struct {
	Deps
}

func (h *SearchHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/search", h.Search)
	r.With(mw.RequireAuth(h.Store)).Get("/search/quick", h.Quick)
}

func searchQueryError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, search.ErrEmptyQuery):
		httpErr(w, http.StatusBadRequest, "q is required")
	case errors.Is(err, search.ErrQueryTooLong):
		httpErr(w, http.StatusBadRequest, "q is too long")
	default:
		return false
	}
	return true
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	qp := r.URL.Query()
	params := search.Params{
		Q:        qp.Get("q"),
		SpaceKey: qp.Get("space"),
		Label:    qp.Get("label"),
		Page:     queryInt(r, "page", 1, 1, search.MaxPage),
		Limit:    queryInt(r, "limit", search.DefaultLimit, 1, search.MaxLimit),
		UserID:   u.ID,
		IsAdmin:  u.IsSystemAdmin,
	}
	if a := strings.TrimSpace(qp.Get("author")); a != "" {
		id, err := uuid.Parse(a)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "invalid author")
			return
		}
		params.AuthorID = id
	}
	if _, err := params.Normalize(); err != nil {
		if searchQueryError(w, err) {
			return
		}
		internalErr(w, r, err, "search failed")
		return
	}
	res, err := h.Store.SearchPages(r.Context(), params)
	if err != nil {
		internalErr(w, r, err, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *SearchHandler) Quick(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	params := search.QuickParams{
		Q:       r.URL.Query().Get("q"),
		Limit:   queryInt(r, "limit", search.QuickDefaultLimit, 1, search.QuickMaxLimit),
		UserID:  u.ID,
		IsAdmin: u.IsSystemAdmin,
	}
	hits, err := h.Store.QuickSearchPages(r.Context(), params)
	if err != nil {
		if searchQueryError(w, err) {
			return
		}
		internalErr(w, r, err, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, hits)
}
