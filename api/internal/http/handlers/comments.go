package handlers

import (
	"encoding/json"
	"errors"
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
	commentMaxQuoteRunes = 2000
	commentMaxReplyDepth = 2
)

type CommentHandler struct {
	Deps
}

func (h *CommentHandler) Routes(r chi.Router) {
	byComment := func(perm domain.Perm) func(http.Handler) http.Handler {
		return mw.SpaceByResolver(h.Store, commentSpaceResolver(h.Store), perm)
	}
	r.With(mw.SpaceByPage(h.Store, domain.PermViewSpace)).Get("/pages/{pageID}/comments", h.List)
	r.With(mw.SpaceByPage(h.Store, domain.PermComment)).Post("/pages/{pageID}/comments", h.Create)
	r.With(byComment(domain.PermComment)).Patch("/comments/{commentID}", h.Update)
	r.With(byComment(domain.PermComment)).Delete("/comments/{commentID}", h.Delete)
	r.With(byComment(domain.PermComment)).Post("/comments/{commentID}/resolve", h.Resolve)
	r.With(byComment(domain.PermComment)).Post("/comments/{commentID}/reopen", h.Reopen)
}

func commentSpaceResolver(s *store.Store) func(r *http.Request) (uuid.UUID, error) {
	return func(r *http.Request) (uuid.UUID, error) {
		id, err := mw.URLUUID(r, "commentID")
		if err != nil {
			return uuid.Nil, err
		}
		return s.GetCommentSpaceID(r.Context(), id)
	}
}

type commentBodyInput struct {
	BodyJSON json.RawMessage `json:"body_json"`
	BodyHTML string          `json:"body_html"`
}

type commentCreateInput struct {
	commentBodyInput
	ParentID     *uuid.UUID `json:"parent_id"`
	InlineAnchor *struct {
		QuotedText string `json:"quoted_text"`
	} `json:"inline_anchor"`
}

func commentNormalizeBody(in commentBodyInput) (json.RawMessage, string, bool) {
	raw := json.RawMessage(strings.TrimSpace(string(in.BodyJSON)))
	if len(raw) == 0 || string(raw) == "null" {
		raw = nil
	} else if raw[0] != '{' {
		return nil, "", false
	}
	clean := content.SanitizeHTML(in.BodyHTML)
	hasText := strings.TrimSpace(content.ExtractTextFromHTML(clean)) != "" ||
		(raw != nil && strings.TrimSpace(content.ExtractText(raw)) != "")
	if !hasText {
		return nil, "", false
	}
	return raw, clean, true
}

func commentPageVisible(r *http.Request, info store.CommentPageInfo) bool {
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	if u == nil || sc == nil {
		return false
	}
	switch info.Status {
	case "draft":
		return info.CreatedBy == u.ID || sc.Role == domain.RoleAdmin
	case "trashed":
		return sc.Role.AtLeast(domain.RoleEditor)
	}
	return true
}

func commentRejectTrashed(w http.ResponseWriter, info store.CommentPageInfo) bool {
	if info.Status != "trashed" {
		return false
	}
	httpErrCode(w, http.StatusConflict, "page_trashed", "page is trashed")
	return true
}

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	pageID, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	info, err := h.Store.GetCommentPageInfo(r.Context(), pageID)
	if err != nil {
		commentStoreErr(w, r, err, "failed to load page")
		return
	}
	if !commentPageVisible(r, info) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	comments, err := h.Store.ListPageComments(r.Context(), pageID)
	if err != nil {
		internalErr(w, r, err, "failed to list comments")
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	pageID, err := urlUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	info, err := h.Store.GetCommentPageInfo(r.Context(), pageID)
	if err != nil {
		commentStoreErr(w, r, err, "failed to load page")
		return
	}
	if !commentPageVisible(r, info) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	if commentRejectTrashed(w, info) {
		return
	}
	var in commentCreateInput
	if err := readJSON(r, &in); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	bodyJSON, bodyHTML, ok := commentNormalizeBody(in.commentBodyInput)
	if !ok {
		httpErr(w, http.StatusBadRequest, "comment body is empty")
		return
	}
	var anchor json.RawMessage
	if in.InlineAnchor != nil {
		if in.ParentID != nil {
			httpErr(w, http.StatusBadRequest, "replies cannot have an inline anchor")
			return
		}
		quoted := strings.TrimSpace(in.InlineAnchor.QuotedText)
		if quoted == "" || utf8.RuneCountInString(quoted) > commentMaxQuoteRunes {
			httpErr(w, http.StatusBadRequest, "invalid inline anchor")
			return
		}
		anchor, _ = json.Marshal(map[string]string{"quoted_text": quoted})
	}
	if in.ParentID != nil {
		parent, err := h.Store.GetComment(r.Context(), *in.ParentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpErr(w, http.StatusBadRequest, "invalid parent comment")
				return
			}
			internalErr(w, r, err, "failed to load parent comment")
			return
		}
		if parent.PageID != pageID || parent.DeletedAt != nil {
			httpErr(w, http.StatusBadRequest, "invalid parent comment")
			return
		}
		depth, err := h.Store.GetCommentDepth(r.Context(), parent.ID)
		if err != nil {
			internalErr(w, r, err, "failed to load parent comment")
			return
		}
		if depth+1 > commentMaxReplyDepth {
			httpErr(w, http.StatusBadRequest, "reply nesting too deep")
			return
		}
	}
	created, err := h.Store.CreateComment(r.Context(), domain.Comment{
		ID:           uuid.Must(uuid.NewV7()),
		PageID:       pageID,
		ParentID:     in.ParentID,
		AuthorID:     u.ID,
		BodyJSON:     bodyJSON,
		BodyHTML:     bodyHTML,
		InlineAnchor: anchor,
	})
	if err != nil {
		internalErr(w, r, err, "failed to create comment")
		return
	}
	view, err := h.Store.GetCommentView(r.Context(), created.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load comment")
		return
	}
	h.Bus.Publish(domain.CommentCreated{
		CommentID:        created.ID,
		PageID:           pageID,
		SpaceID:          sc.ID,
		ActorID:          u.ID,
		ParentID:         in.ParentID,
		Inline:           anchor != nil,
		MentionedUserIDs: content.MentionedUserIDs(bodyJSON),
	})
	writeJSON(w, http.StatusCreated, view)
}

func (h *CommentHandler) loadVisible(w http.ResponseWriter, r *http.Request) (domain.Comment, bool) {
	id, err := urlUUID(r, "commentID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return domain.Comment{}, false
	}
	c, info, err := h.Store.GetCommentWithPage(r.Context(), id)
	if err != nil {
		commentStoreErr(w, r, err, "failed to load comment")
		return domain.Comment{}, false
	}
	if !commentPageVisible(r, info) {
		httpErr(w, http.StatusNotFound, "not found")
		return domain.Comment{}, false
	}
	if commentRejectTrashed(w, info) {
		return domain.Comment{}, false
	}
	return c, true
}

func (h *CommentHandler) respondView(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	view, err := h.Store.GetCommentView(r.Context(), id)
	if err != nil {
		commentStoreErr(w, r, err, "failed to load comment")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadVisible(w, r)
	if !ok {
		return
	}
	if c.DeletedAt != nil {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	if c.AuthorID != mw.UserFrom(r.Context()).ID {
		httpErr(w, http.StatusForbidden, "only the author can edit a comment")
		return
	}
	var in commentBodyInput
	if err := readJSON(r, &in); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	bodyJSON, bodyHTML, valid := commentNormalizeBody(in)
	if !valid {
		httpErr(w, http.StatusBadRequest, "comment body is empty")
		return
	}
	if err := h.Store.UpdateCommentBody(r.Context(), c.ID, bodyJSON, bodyHTML); err != nil {
		commentStoreErr(w, r, err, "failed to update comment")
		return
	}
	h.respondView(w, r, c.ID)
}

func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadVisible(w, r)
	if !ok {
		return
	}
	isAdmin := mw.SpaceFrom(r.Context()).Role == domain.RoleAdmin
	if c.AuthorID != mw.UserFrom(r.Context()).ID && !isAdmin {
		httpErr(w, http.StatusForbidden, "only the author or a space admin can delete a comment")
		return
	}
	if err := h.Store.SoftDeleteComment(r.Context(), c.ID); err != nil {
		internalErr(w, r, err, "failed to delete comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CommentHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	c, ok := h.commentThreadRoot(w, r)
	if !ok {
		return
	}
	u := mw.UserFrom(r.Context())
	changed, err := h.Store.ResolveComment(r.Context(), c.ID, u.ID)
	if err != nil {
		internalErr(w, r, err, "failed to resolve comment")
		return
	}
	if changed {
		h.Bus.Publish(domain.CommentResolved{
			CommentID: c.ID,
			PageID:    c.PageID,
			SpaceID:   mw.SpaceFrom(r.Context()).ID,
			ActorID:   u.ID,
		})
	}
	h.respondView(w, r, c.ID)
}

func (h *CommentHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	c, ok := h.commentThreadRoot(w, r)
	if !ok {
		return
	}
	if _, err := h.Store.ReopenComment(r.Context(), c.ID); err != nil {
		internalErr(w, r, err, "failed to reopen comment")
		return
	}
	h.respondView(w, r, c.ID)
}

func (h *CommentHandler) commentThreadRoot(w http.ResponseWriter, r *http.Request) (domain.Comment, bool) {
	c, ok := h.loadVisible(w, r)
	if !ok {
		return c, false
	}
	if c.DeletedAt != nil {
		httpErr(w, http.StatusNotFound, "not found")
		return c, false
	}
	if c.ParentID != nil {
		httpErr(w, http.StatusBadRequest, "only top-level comments can be resolved")
		return c, false
	}
	return c, true
}

func commentStoreErr(w http.ResponseWriter, r *http.Request, err error, msg string) {
	if errors.Is(err, store.ErrNotFound) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	internalErr(w, r, err, msg)
}
