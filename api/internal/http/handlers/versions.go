package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type VersionHandler struct {
	Deps
}

func (h *VersionHandler) Routes(r chi.Router) {
	view := mw.SpaceByPage(h.Store, domain.PermViewSpace)
	edit := mw.SpaceByPage(h.Store, domain.PermEditPages)

	r.With(view).Get("/pages/{pageID}/versions", h.List)
	r.With(view).Get("/pages/{pageID}/versions/{version}", h.Get)
	r.With(edit).Post("/pages/{pageID}/versions/{version}/restore", h.Restore)
}

func pageVersionParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n < 1 || n > math.MaxInt32 {
		httpErr(w, http.StatusBadRequest, "invalid version")
		return 0, false
	}
	return n, true
}

func (h *VersionHandler) List(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	versions, err := h.Store.ListPageVersions(r.Context(), req.page.ID)
	if err != nil {
		internalErr(w, r, err, "failed to list versions")
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (h *VersionHandler) Get(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	n, ok := pageVersionParam(w, r)
	if !ok {
		return
	}
	v, err := h.Store.GetPageVersion(r.Context(), req.page.ID, n)
	if err != nil {
		pageStoreErr(w, r, err, "failed to load version")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *VersionHandler) Restore(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	n, ok := pageVersionParam(w, r)
	if !ok {
		return
	}
	v, err := h.Store.GetPageVersion(r.Context(), req.page.ID, n)
	if err != nil {
		pageStoreErr(w, r, err, "failed to load version")
		return
	}
	message := fmt.Sprintf("Restored version %d", n)
	html := content.SanitizeHTML(v.ContentHTML)
	page, err := pagePublishNow(r.Context(), h.Deps, req.user, store.PublishParams{
		PageID:      req.page.ID,
		ActorID:     req.user.ID,
		Title:       v.Title,
		ContentJSON: v.ContentJSON,
		ContentHTML: html,
		ContentText: pageContentText(v.ContentJSON, html),
		Force:       true,
		Message:     &message,
	})
	if err != nil {
		pageStoreErr(w, r, err, "failed to restore version")
		return
	}
	writeJSON(w, http.StatusOK, pageOut{Page: page, SpaceKey: req.space.Key})
}
