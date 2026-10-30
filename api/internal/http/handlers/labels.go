package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	labelMaxLen   = 50
	labelMaxCount = 20
)

type LabelHandler struct {
	Deps
}

func (h *LabelHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/labels", h.List)
	r.With(mw.SpaceByPage(h.Store, domain.PermViewSpace)).Get("/pages/{pageID}/labels", h.PageLabels)
	r.With(mw.SpaceByPage(h.Store, domain.PermManageLabels)).Put("/pages/{pageID}/labels", h.SetPageLabels)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/labels", h.SpaceLabels)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/labels/{name}/pages", h.LabelPages)
}

var errLabelInvalid = errors.New("invalid label")

func labelNormalize(raw string) (string, error) {
	fields := strings.Fields(strings.ToLower(raw))
	name := strings.Join(fields, "-")
	if name == "" || len(name) > labelMaxLen {
		return "", errLabelInvalid
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return "", errLabelInvalid
		}
	}
	return name, nil
}

func labelNormalizeAll(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		name, err := labelNormalize(r)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) > labelMaxCount {
		return nil, errLabelInvalid
	}
	sort.Strings(out)
	return out, nil
}

func (h *LabelHandler) labelGate(w http.ResponseWriter, r *http.Request, write bool) bool {
	pageID, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return false
	}
	status, createdBy, err := h.Store.GetPageLabelGate(r.Context(), pageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return false
		}
		internalErr(w, r, err, "failed to load page")
		return false
	}
	if status == domain.PageStatusDraft {
		u := mw.UserFrom(r.Context())
		sc := mw.SpaceFrom(r.Context())
		if u == nil || sc == nil || (u.ID != createdBy && sc.Role != domain.RoleAdmin) {
			httpErr(w, http.StatusNotFound, "not found")
			return false
		}
	}
	if status == domain.PageStatusTrashed {
		if write {
			httpErrCode(w, http.StatusConflict, "page_trashed", "page is trashed")
			return false
		}
		sc := mw.SpaceFrom(r.Context())
		if sc == nil || !sc.Role.AtLeast(domain.RoleEditor) {
			httpErr(w, http.StatusNotFound, "not found")
			return false
		}
	}
	return true
}

func (h *LabelHandler) List(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 20, 1, 50)
	labels, err := h.Store.ListLabels(r.Context(), u.ID, u.IsSystemAdmin, q, limit)
	if err != nil {
		internalErr(w, r, err, "failed to list labels")
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) PageLabels(w http.ResponseWriter, r *http.Request) {
	pageID, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !h.labelGate(w, r, false) {
		return
	}
	labels, err := h.Store.ListPageLabels(r.Context(), pageID)
	if err != nil {
		internalErr(w, r, err, "failed to list labels")
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) SetPageLabels(w http.ResponseWriter, r *http.Request) {
	pageID, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Labels *[]string `json:"labels"`
	}
	if err := readJSON(r, &body); err != nil || body.Labels == nil {
		httpErr(w, http.StatusBadRequest, "labels array is required")
		return
	}
	names, err := labelNormalizeAll(*body.Labels)
	if err != nil {
		httpErr(w, http.StatusBadRequest, "labels must be 1-50 characters of letters a-z, digits, '-', '_' or '.' and at most 20 per page")
		return
	}
	if !h.labelGate(w, r, true) {
		return
	}
	labels, err := h.Store.SetPageLabels(r.Context(), pageID, names)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to set labels")
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) SpaceLabels(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	labels, err := h.Store.ListSpaceLabels(r.Context(), sc.ID)
	if err != nil {
		internalErr(w, r, err, "failed to list labels")
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) LabelPages(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	rawName, err := url.PathUnescape(chi.URLParam(r, "name"))
	if err != nil {
		rawName = ""
	}
	name, err := labelNormalize(rawName)
	if err != nil {
		writeJSON(w, http.StatusOK, []store.LabelPageSummary{})
		return
	}
	pages, err := h.Store.ListSpaceLabelPages(r.Context(), sc.ID, name)
	if err != nil {
		internalErr(w, r, err, "failed to list pages")
		return
	}
	writeJSON(w, http.StatusOK, pages)
}
