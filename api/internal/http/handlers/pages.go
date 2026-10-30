package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	pageMaxTitleRunes   = 500
	pageMaxMessageRunes = 1000
	pageDefaultTitle    = "Untitled"
)

type PageHandler struct {
	Deps
}

func (h *PageHandler) Routes(r chi.Router) {
	view := mw.SpaceByPage(h.Store, domain.PermViewSpace)
	edit := mw.SpaceByPage(h.Store, domain.PermEditPages)
	purge := mw.SpaceByPage(h.Store, domain.PermPurgeTrash)

	r.With(mw.SpaceByKey(h.Store, domain.PermEditPages)).Post("/spaces/{key}/pages", h.Create)
	r.With(view).Get("/pages/{pageID}", h.Get)
	r.With(edit).Patch("/pages/{pageID}", h.Update)
	r.With(purge).Delete("/pages/{pageID}", h.Purge)
	r.With(edit).Put("/pages/{pageID}/draft", h.SaveDraft)
	r.With(edit).Get("/pages/{pageID}/draft", h.GetDraft)
	r.With(edit).Delete("/pages/{pageID}/draft", h.DiscardDraft)
	r.With(edit).Post("/pages/{pageID}/publish", h.Publish)
	r.With(edit).Post("/pages/{pageID}/move", h.Move)
	r.With(edit).Post("/pages/{pageID}/trash", h.Trash)
	r.With(edit).Post("/pages/{pageID}/restore", h.Restore)
	r.With(edit).Post("/pages/{pageID}/copy", h.Copy)
	r.With(view).Get("/pages/{pageID}/children", h.Children)
	r.With(view).Post("/pages/{pageID}/star", h.Star)
	r.With(view).Delete("/pages/{pageID}/star", h.Unstar)
	r.With(edit).Post("/pages/{pageID}/editing", h.Editing)
}

type pageOut struct {
	domain.Page
	SpaceKey string `json:"space_key"`
}

type pageDetailOut struct {
	domain.Page
	SpaceKey      string              `json:"space_key"`
	CreatedByUser *domain.UserRef     `json:"created_by_user"`
	UpdatedByUser *domain.UserRef     `json:"updated_by_user"`
	Labels        []domain.Label      `json:"labels"`
	Breadcrumbs   []store.PageSummary `json:"breadcrumbs"`
	Children      []store.PageSummary `json:"children"`
	MyDraft       *store.DraftRef     `json:"my_draft"`
	Editors       []domain.UserRef    `json:"editors"`
	Starred       bool                `json:"starred"`
	Watching      bool                `json:"watching"`
	MyRole        domain.Role         `json:"my_role"`
}

type pageReq struct {
	user  *domain.User
	space *mw.SpaceCtx
	page  domain.Page
}

func pageDecode(w http.ResponseWriter, r *http.Request, v any, allowEmpty bool) bool {
	err := readJSON(r, v)
	if err == nil || (allowEmpty && errors.Is(err, io.EOF)) {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	httpErr(w, http.StatusBadRequest, "bad body")
	return false
}

func pageLoad(w http.ResponseWriter, r *http.Request, s *store.Store, editable bool) (pageReq, bool) {
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	if u == nil || sc == nil {
		httpErr(w, http.StatusUnauthorized, "unauthenticated")
		return pageReq{}, false
	}
	id, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return pageReq{}, false
	}
	page, err := s.GetPage(r.Context(), id)
	if err != nil {
		pageStoreErr(w, r, err, "failed to load page")
		return pageReq{}, false
	}
	if page.SpaceID != sc.ID || !store.IsPageVisible(page, u.ID, sc.Role) {
		httpErr(w, http.StatusNotFound, "not found")
		return pageReq{}, false
	}
	if editable && page.Status == domain.PageStatusTrashed {
		httpErrCode(w, http.StatusConflict, "page_trashed", "page is trashed")
		return pageReq{}, false
	}
	return pageReq{user: u, space: sc, page: page}, true
}

func pageStoreErr(w http.ResponseWriter, r *http.Request, err error, msg string) {
	var conflict *store.VersionConflictError
	switch {
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":           conflict.Error(),
			"code":            "version_conflict",
			"current_version": conflict.Current,
		})
	case errors.Is(err, store.ErrNotFound):
		httpErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrInvalidParent), errors.Is(err, store.ErrPageCycle),
		errors.Is(err, store.ErrInvalidAnchor), errors.Is(err, store.ErrHomePage),
		errors.Is(err, store.ErrNotTrashed):
		httpErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrPageTrashed):
		httpErrCode(w, http.StatusConflict, "page_trashed", "page is trashed")
	default:
		internalErr(w, r, err, msg)
	}
}

func pageValidTitle(title string) bool {
	return utf8.RuneCountInString(title) <= pageMaxTitleRunes
}

func pageContentJSON(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, true
	}
	if trimmed[0] != '{' {
		return nil, false
	}
	return json.RawMessage(trimmed), true
}

func pageContentText(contentJSON json.RawMessage, html string) string {
	text := content.ExtractText(contentJSON)
	if text == "" {
		text = content.ExtractTextFromHTML(html)
	}
	return text
}

func pageOthers(editors []domain.UserRef, self uuid.UUID) []domain.UserRef {
	out := make([]domain.UserRef, 0, len(editors))
	for _, e := range editors {
		if e.ID != self {
			out = append(out, e)
		}
	}
	return out
}

func pagePublishNow(ctx context.Context, d Deps, actor *domain.User, p store.PublishParams) (domain.Page, error) {
	page, err := d.Store.PublishPage(ctx, p)
	if err != nil {
		return page, err
	}
	d.Bus.Publish(domain.PagePublished{
		PageID:           page.ID,
		SpaceID:          page.SpaceID,
		ActorID:          actor.ID,
		Version:          page.Version,
		MentionedUserIDs: content.MentionedUserIDs(page.ContentJSON),
	})
	return page, nil
}

func (h *PageHandler) pageLabels(ctx context.Context, pageID uuid.UUID) ([]domain.Label, error) {
	return h.Store.ListPageLabels(ctx, pageID)
}

func (h *PageHandler) pageWatching(ctx context.Context, pageID, userID uuid.UUID) (bool, error) {
	return h.Store.IsWatching(ctx, userID, domain.WatchTargetPage, pageID)
}

func (h *PageHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	var body struct {
		Title      string     `json:"title"`
		ParentID   *uuid.UUID `json:"parent_id"`
		TemplateID *uuid.UUID `json:"template_id"`
	}
	if !pageDecode(w, r, &body, false) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if !pageValidTitle(title) {
		httpErr(w, http.StatusBadRequest, "title too long")
		return
	}
	in := store.NewPage{SpaceID: sc.ID, ParentID: body.ParentID, CreatedBy: u.ID}
	if in.ParentID == nil {
		home, err := h.Store.PageSpaceHomeID(r.Context(), sc.ID)
		if err != nil {
			pageStoreErr(w, r, err, "failed to load space")
			return
		}
		in.ParentID = home
	}
	if body.TemplateID != nil {
		tpl, err := h.Store.GetTemplateContent(r.Context(), *body.TemplateID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpErr(w, http.StatusNotFound, "template not found")
				return
			}
			internalErr(w, r, err, "failed to load template")
			return
		}
		if tpl.SpaceID != uuid.Nil && tpl.SpaceID != sc.ID {
			httpErr(w, http.StatusNotFound, "template not found")
			return
		}
		raw, ok := pageContentJSON(tpl.ContentJSON)
		if !ok {
			internalErr(w, r, errors.New("template content is not an object"), "failed to load template")
			return
		}
		in.ContentJSON = raw
		in.ContentHTML = content.SanitizeHTML(tpl.ContentHTML)
		in.ContentText = pageContentText(raw, in.ContentHTML)
		if title == "" {
			title = strings.TrimSpace(tpl.Title)
		}
	} else {
		in.ContentJSON = json.RawMessage(store.PageEmptyDocJSON)
	}
	if title == "" {
		title = pageDefaultTitle
	}
	in.Title = title
	page, err := h.Store.CreatePage(r.Context(), in)
	if err != nil {
		pageStoreErr(w, r, err, "failed to create page")
		return
	}
	writeJSON(w, http.StatusCreated, pageOut{Page: page, SpaceKey: sc.Key})
}

func (h *PageHandler) Get(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	ctx := r.Context()
	page := req.page
	isAdmin := req.space.Role == domain.RoleAdmin

	if err := h.Store.RecordPageView(ctx, req.user.ID, page.ID); err != nil {
		log.Printf("record page view %s: %v", page.ID, err)
	}
	breadcrumbs, err := h.Store.ListVisibleAncestors(ctx, page.ID, req.user.ID, req.space.Role)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	children, err := h.Store.ListChildren(ctx, page.ID, req.user.ID, isAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	myDraft, err := h.Store.GetDraftRef(ctx, page.ID, req.user.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	starred, err := h.Store.IsPageStarred(ctx, req.user.ID, page.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	refs, err := h.Store.GetUserRefs(ctx, []uuid.UUID{page.CreatedBy, page.UpdatedBy})
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	labels, err := h.pageLabels(ctx, page.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	watching, err := h.pageWatching(ctx, page.ID, req.user.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load page")
		return
	}
	out := pageDetailOut{
		Page:        page,
		SpaceKey:    req.space.Key,
		Labels:      labels,
		Breadcrumbs: breadcrumbs,
		Children:    children,
		MyDraft:     myDraft,
		Editors:     pageOthers(h.Presence.Editors(page.ID), req.user.ID),
		Starred:     starred,
		Watching:    watching,
		MyRole:      req.space.Role,
	}
	if ref, ok := refs[page.CreatedBy]; ok {
		out.CreatedByUser = &ref
	}
	if ref, ok := refs[page.UpdatedBy]; ok {
		out.UpdatedByUser = &ref
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *PageHandler) Update(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	var body struct {
		Title *string `json:"title"`
	}
	if !pageDecode(w, r, &body, false) {
		return
	}
	if body.Title == nil {
		writeJSON(w, http.StatusOK, pageOut{Page: req.page, SpaceKey: req.space.Key})
		return
	}
	title := strings.TrimSpace(*body.Title)
	if title == "" || !pageValidTitle(title) {
		httpErr(w, http.StatusBadRequest, "invalid title")
		return
	}
	page, err := h.Store.UpdatePageTitle(r.Context(), req.page.ID, req.user.ID, title)
	if err != nil {
		pageStoreErr(w, r, err, "failed to update page")
		return
	}
	writeJSON(w, http.StatusOK, pageOut{Page: page, SpaceKey: req.space.Key})
}

func (h *PageHandler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	var body struct {
		Title       string          `json:"title"`
		ContentJSON json.RawMessage `json:"content_json"`
		ContentHTML string          `json:"content_html"`
		BaseVersion int             `json:"base_version"`
	}
	if !pageDecode(w, r, &body, false) {
		return
	}
	if body.BaseVersion < 0 || body.BaseVersion > math.MaxInt32 {
		httpErr(w, http.StatusBadRequest, "invalid base_version")
		return
	}
	title := strings.TrimSpace(body.Title)
	if !pageValidTitle(title) {
		httpErr(w, http.StatusBadRequest, "title too long")
		return
	}
	raw, valid := pageContentJSON(body.ContentJSON)
	if !valid {
		httpErr(w, http.StatusBadRequest, "content_json must be an object or null")
		return
	}
	draft, err := h.Store.UpsertDraft(r.Context(), store.DraftParams{
		PageID:      req.page.ID,
		UserID:      req.user.ID,
		Title:       title,
		ContentJSON: raw,
		ContentHTML: content.SanitizeHTML(body.ContentHTML),
		BaseVersion: body.BaseVersion,
	})
	if err != nil {
		pageStoreErr(w, r, err, "failed to save draft")
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *PageHandler) GetDraft(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	draft, err := h.Store.GetDraft(r.Context(), req.page.ID, req.user.ID)
	if err != nil {
		pageStoreErr(w, r, err, "failed to load draft")
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *PageHandler) DiscardDraft(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	if err := h.Store.DeleteDraft(r.Context(), req.page.ID, req.user.ID); err != nil {
		internalErr(w, r, err, "failed to discard draft")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PageHandler) Publish(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	var body struct {
		Title       string          `json:"title"`
		ContentJSON json.RawMessage `json:"content_json"`
		ContentHTML string          `json:"content_html"`
		BaseVersion int             `json:"base_version"`
		Message     *string         `json:"message"`
		Force       bool            `json:"force"`
	}
	if !pageDecode(w, r, &body, false) {
		return
	}
	if body.BaseVersion < 0 || body.BaseVersion > math.MaxInt32 {
		httpErr(w, http.StatusBadRequest, "invalid base_version")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || !pageValidTitle(title) {
		httpErr(w, http.StatusBadRequest, "title is required")
		return
	}
	raw, valid := pageContentJSON(body.ContentJSON)
	if !valid {
		httpErr(w, http.StatusBadRequest, "content_json must be an object or null")
		return
	}
	var message *string
	if body.Message != nil {
		m := strings.TrimSpace(*body.Message)
		if utf8.RuneCountInString(m) > pageMaxMessageRunes {
			httpErr(w, http.StatusBadRequest, "message too long")
			return
		}
		if m != "" {
			message = &m
		}
	}
	html := content.SanitizeHTML(body.ContentHTML)
	page, err := pagePublishNow(r.Context(), h.Deps, req.user, store.PublishParams{
		PageID:      req.page.ID,
		ActorID:     req.user.ID,
		Title:       title,
		ContentJSON: raw,
		ContentHTML: html,
		ContentText: pageContentText(raw, html),
		BaseVersion: body.BaseVersion,
		Force:       body.Force,
		Message:     message,
	})
	if err != nil {
		pageStoreErr(w, r, err, "failed to publish page")
		return
	}
	writeJSON(w, http.StatusOK, pageOut{Page: page, SpaceKey: req.space.Key})
}

func (h *PageHandler) Move(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	var body struct {
		ParentID *uuid.UUID `json:"parent_id"`
		AfterID  *uuid.UUID `json:"after_id"`
		Position *int       `json:"position"`
	}
	if !pageDecode(w, r, &body, false) {
		return
	}
	res, err := h.Store.MovePage(r.Context(), store.MoveParams{
		PageID:   req.page.ID,
		ParentID: body.ParentID,
		AfterID:  body.AfterID,
		Position: body.Position,
	})
	if err != nil {
		pageStoreErr(w, r, err, "failed to move page")
		return
	}
	h.Bus.Publish(domain.PageMoved{
		PageID:      res.Page.ID,
		SpaceID:     res.Page.SpaceID,
		ActorID:     req.user.ID,
		OldParentID: res.OldParentID,
		NewParentID: res.Page.ParentID,
	})
	writeJSON(w, http.StatusOK, pageOut{Page: res.Page, SpaceKey: req.space.Key})
}

func (h *PageHandler) Trash(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	if err := h.Store.TrashPage(r.Context(), req.page.ID, req.user.ID); err != nil {
		pageStoreErr(w, r, err, "failed to trash page")
		return
	}
	h.Bus.Publish(domain.PageTrashed{PageID: req.page.ID, SpaceID: req.page.SpaceID, ActorID: req.user.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (h *PageHandler) Restore(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	page, err := h.Store.RestorePage(r.Context(), req.page.ID)
	if err != nil {
		pageStoreErr(w, r, err, "failed to restore page")
		return
	}
	h.Bus.Publish(domain.PageRestored{PageID: page.ID, SpaceID: page.SpaceID, ActorID: req.user.ID})
	writeJSON(w, http.StatusOK, pageOut{Page: page, SpaceKey: req.space.Key})
}

func (h *PageHandler) Purge(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	var keys []string
	if h.Storage != nil && req.page.Status == domain.PageStatusTrashed {
		var err error
		keys, err = h.Store.ListAttachmentObjectKeysUnderPage(r.Context(), req.page.ID)
		if err != nil {
			internalErr(w, r, err, "failed to purge page")
			return
		}
	}
	if err := h.Store.PurgePage(r.Context(), req.page.ID); err != nil {
		pageStoreErr(w, r, err, "failed to purge page")
		return
	}
	attachmentPurgeObjects(r.Context(), h.Storage, keys)
	w.WriteHeader(http.StatusNoContent)
}

func (h *PageHandler) Copy(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	var body struct {
		Title    *string    `json:"title"`
		ParentID *uuid.UUID `json:"parent_id"`
	}
	if !pageDecode(w, r, &body, true) {
		return
	}
	title := "Copy of " + req.page.Title
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		title = strings.TrimSpace(*body.Title)
	}
	if !pageValidTitle(title) {
		httpErr(w, http.StatusBadRequest, "title too long")
		return
	}
	parent := req.page.ParentID
	if body.ParentID != nil {
		parent = body.ParentID
	}
	page, err := h.Store.CreatePage(r.Context(), store.NewPage{
		SpaceID:     req.page.SpaceID,
		ParentID:    parent,
		Title:       title,
		ContentJSON: req.page.ContentJSON,
		ContentHTML: req.page.ContentHTML,
		ContentText: req.page.ContentText,
		CreatedBy:   req.user.ID,
	})
	if err != nil {
		pageStoreErr(w, r, err, "failed to copy page")
		return
	}
	writeJSON(w, http.StatusCreated, pageOut{Page: page, SpaceKey: req.space.Key})
}

func (h *PageHandler) Children(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	children, err := h.Store.ListChildren(r.Context(), req.page.ID, req.user.ID, req.space.Role == domain.RoleAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to list children")
		return
	}
	writeJSON(w, http.StatusOK, children)
}

func (h *PageHandler) Star(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	if err := h.Store.StarPage(r.Context(), req.user.ID, req.page.ID); err != nil {
		internalErr(w, r, err, "failed to star page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PageHandler) Unstar(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	if err := h.Store.UnstarPage(r.Context(), req.user.ID, req.page.ID); err != nil {
		internalErr(w, r, err, "failed to unstar page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PageHandler) Editing(w http.ResponseWriter, r *http.Request) {
	req, ok := pageLoad(w, r, h.Store, true)
	if !ok {
		return
	}
	h.Presence.Touch(req.page.ID, req.user.Ref())
	writeJSON(w, http.StatusOK, map[string]any{
		"editors": pageOthers(h.Presence.Editors(req.page.ID), req.user.ID),
	})
}
