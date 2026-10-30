package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	spaceMaxNameRunes        = 200
	spaceMaxDescriptionRunes = 2000
	spaceMaxIconRunes        = 64
)

var spaceKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

type SpaceHandler struct {
	Deps
}

func (h *SpaceHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/spaces", h.List)
	r.With(mw.RequireAuth(h.Store)).Post("/spaces", h.Create)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}", h.Get)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageSpace)).Patch("/spaces/{key}", h.Update)
	r.With(mw.SpaceByKey(h.Store, domain.PermDeleteSpace)).Delete("/spaces/{key}", h.Delete)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageSpace)).Post("/spaces/{key}/archive", h.Archive)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageSpace)).Post("/spaces/{key}/unarchive", h.Unarchive)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/members", h.ListMembers)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageMembers)).Put("/spaces/{key}/members/{userID}", h.PutMember)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageMembers)).Delete("/spaces/{key}/members/{userID}", h.RemoveMember)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/tree", h.Tree)
	r.With(mw.SpaceByKey(h.Store, domain.PermEditPages)).Get("/spaces/{key}/trash", h.Trash)
	r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Get("/admin/spaces", h.AdminList)
}

func (h *SpaceHandler) List(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	spaces, err := h.Store.ListSpacesAccessible(r.Context(), u.ID, u.IsSystemAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to list spaces")
		return
	}
	writeJSON(w, http.StatusOK, spaces)
}

func (h *SpaceHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	spaces, err := h.Store.ListAdminSpaces(r.Context())
	if err != nil {
		internalErr(w, r, err, "failed to list spaces")
		return
	}
	writeJSON(w, http.StatusOK, spaces)
}

func (h *SpaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	var body struct {
		Key         string  `json:"key"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Icon        *string `json:"icon"`
		Visibility  string  `json:"visibility"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	key := strings.ToUpper(strings.TrimSpace(body.Key))
	if !spaceKeyPattern.MatchString(key) {
		httpErr(w, http.StatusBadRequest, "key must be 2-10 characters, start with a letter, and contain only letters and digits")
		return
	}
	name, ok := spaceCleanName(body.Name)
	if !ok {
		httpErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	description := strings.TrimSpace(body.Description)
	if utf8.RuneCountInString(description) > spaceMaxDescriptionRunes {
		httpErr(w, http.StatusBadRequest, "description too long")
		return
	}
	icon, ok := spaceCleanIcon(body.Icon)
	if !ok {
		httpErr(w, http.StatusBadRequest, "invalid icon")
		return
	}
	visibility := domain.VisibilityInternal
	if body.Visibility != "" {
		visibility = domain.Visibility(body.Visibility)
		if !visibility.Valid() {
			httpErr(w, http.StatusBadRequest, "invalid visibility")
			return
		}
	}
	space, err := h.Store.CreateSpace(r.Context(), store.CreateSpaceParams{
		Key:         key,
		Name:        name,
		Description: description,
		Icon:        icon,
		Visibility:  visibility,
		CreatedBy:   u.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpErr(w, http.StatusConflict, "space key already in use")
			return
		}
		internalErr(w, r, err, "failed to create space")
		return
	}
	log.Printf("audit space.create actor=%s space=%s key=%s", u.ID, space.ID, space.Key)
	writeJSON(w, http.StatusCreated, space)
}

func (h *SpaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	h.respondSpace(w, r, http.StatusOK)
}

func (h *SpaceHandler) respondSpace(w http.ResponseWriter, r *http.Request, status int) {
	sc := mw.SpaceFrom(r.Context())
	space, err := h.Store.GetSpaceView(r.Context(), sc.ID)
	if err != nil {
		spaceStoreErr(w, r, err, "failed to load space")
		return
	}
	role := sc.Role
	space.Role = &role
	writeJSON(w, status, space)
}

func (h *SpaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil || raw == nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	var upd store.SpaceUpdate
	if v, ok := raw["name"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid name")
			return
		}
		name, ok := spaceCleanName(s)
		if !ok {
			httpErr(w, http.StatusBadRequest, "invalid name")
			return
		}
		upd.Name = &name
	}
	if v, ok := raw["description"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid description")
			return
		}
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) > spaceMaxDescriptionRunes {
			httpErr(w, http.StatusBadRequest, "description too long")
			return
		}
		upd.Description = &s
	}
	if v, ok := raw["icon"]; ok {
		var s *string
		if err := json.Unmarshal(v, &s); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid icon")
			return
		}
		icon, ok := spaceCleanIcon(s)
		if !ok {
			httpErr(w, http.StatusBadRequest, "invalid icon")
			return
		}
		upd.SetIcon = true
		upd.Icon = icon
	}
	if v, ok := raw["visibility"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid visibility")
			return
		}
		vis := domain.Visibility(s)
		if !vis.Valid() {
			httpErr(w, http.StatusBadRequest, "invalid visibility")
			return
		}
		upd.Visibility = &vis
	}
	if v, ok := raw["home_page_id"]; ok {
		var s *string
		if err := json.Unmarshal(v, &s); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid home page")
			return
		}
		upd.SetHomePage = true
		if s != nil {
			id, err := uuid.Parse(*s)
			if err != nil {
				httpErr(w, http.StatusBadRequest, "invalid home page")
				return
			}
			upd.HomePageID = &id
		}
	}
	if err := h.Store.UpdateSpace(r.Context(), sc.ID, upd); err != nil {
		if errors.Is(err, store.ErrInvalidHomePage) {
			httpErr(w, http.StatusBadRequest, err.Error())
			return
		}
		spaceStoreErr(w, r, err, "failed to update space")
		return
	}
	h.respondSpace(w, r, http.StatusOK)
}

func (h *SpaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	if err := h.Store.SoftDeleteSpace(r.Context(), sc.ID); err != nil {
		spaceStoreErr(w, r, err, "failed to delete space")
		return
	}
	if u := mw.UserFrom(r.Context()); u != nil {
		log.Printf("audit space.delete actor=%s space=%s key=%s", u.ID, sc.ID, sc.Key)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SpaceHandler) Archive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, true)
}

func (h *SpaceHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, false)
}

func (h *SpaceHandler) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	sc := mw.SpaceFrom(r.Context())
	if err := h.Store.SetSpaceArchived(r.Context(), sc.ID, archived); err != nil {
		spaceStoreErr(w, r, err, "failed to update space")
		return
	}
	h.respondSpace(w, r, http.StatusOK)
}

func (h *SpaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	members, err := h.Store.ListSpaceMembers(r.Context(), sc.ID)
	if err != nil {
		internalErr(w, r, err, "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *SpaceHandler) PutMember(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	actor := mw.UserFrom(r.Context())
	userID, err := urlUUID(r, "userID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Role domain.Role `json:"role"`
	}
	if err := readJSON(r, &body); err != nil || !body.Role.Valid() {
		httpErr(w, http.StatusBadRequest, "invalid role")
		return
	}
	target, err := h.Store.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "user not found")
			return
		}
		internalErr(w, r, err, "failed to load user")
		return
	}
	if target.DisabledAt != nil {
		httpErr(w, http.StatusConflict, "user is disabled")
		return
	}
	member, inserted, err := h.Store.UpsertSpaceMember(r.Context(), sc.ID, userID, body.Role)
	if err != nil {
		spaceMemberErr(w, r, err, "failed to save member")
		return
	}
	if inserted && h.Bus != nil {
		h.Bus.Publish(domain.SpaceMemberAdded{
			SpaceID: sc.ID,
			UserID:  userID,
			ActorID: actor.ID,
			Role:    body.Role,
		})
	}
	writeJSON(w, http.StatusOK, member)
}

func (h *SpaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	userID, err := urlUUID(r, "userID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Store.RemoveSpaceMember(r.Context(), sc.ID, userID); err != nil {
		spaceMemberErr(w, r, err, "failed to remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SpaceHandler) Tree(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	u := mw.UserFrom(r.Context())
	pages, err := h.Store.ListSpaceTree(r.Context(), sc.ID, u.ID, sc.Role == domain.RoleAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to load page tree")
		return
	}
	writeJSON(w, http.StatusOK, pages)
}

func (h *SpaceHandler) Trash(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	u := mw.UserFrom(r.Context())
	pages, err := h.Store.ListSpaceTrash(r.Context(), sc.ID, u.ID, sc.Role == domain.RoleAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to load trash")
		return
	}
	writeJSON(w, http.StatusOK, pages)
}

func spaceStoreErr(w http.ResponseWriter, r *http.Request, err error, msg string) {
	if errors.Is(err, store.ErrNotFound) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	internalErr(w, r, err, msg)
}

func spaceMemberErr(w http.ResponseWriter, r *http.Request, err error, msg string) {
	switch {
	case errors.Is(err, store.ErrLastAdmin):
		httpErrCode(w, http.StatusConflict, "last_admin", "a space must keep at least one admin member")
	case errors.Is(err, store.ErrNotFound):
		httpErr(w, http.StatusNotFound, "not found")
	default:
		internalErr(w, r, err, msg)
	}
}

func spaceCleanName(s string) (string, bool) {
	s = cleanName(s)
	if s == "" || utf8.RuneCountInString(s) > spaceMaxNameRunes {
		return "", false
	}
	return s, true
}

func spaceCleanIcon(s *string) (*string, bool) {
	if s == nil {
		return nil, true
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, true
	}
	if utf8.RuneCountInString(v) > spaceMaxIconRunes {
		return nil, false
	}
	return &v, true
}
