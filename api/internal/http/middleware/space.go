package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

var ErrInvalidID = errors.New("invalid id")

type SpaceCtx struct {
	ID         uuid.UUID
	Key        string
	Role       domain.Role
	Visibility domain.Visibility
	Archived   bool
	Deleted    bool
}

func (s *SpaceCtx) ReadOnly() bool { return s.Archived || s.Deleted }

func SpaceFrom(ctx context.Context) *SpaceCtx {
	s, _ := ctx.Value(ctxSpace).(*SpaceCtx)
	return s
}

func URLUUID(r *http.Request, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s", ErrInvalidID, param)
	}
	return id, nil
}

type spaceLookup func(r *http.Request, userID uuid.UUID) (store.SpaceAccess, error)

func SpaceByKey(s *store.Store, perm domain.Perm) func(http.Handler) http.Handler {
	return spaceChain(s, perm, func(r *http.Request, userID uuid.UUID) (store.SpaceAccess, error) {
		key := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "key")))
		if key == "" {
			return store.SpaceAccess{}, ErrInvalidID
		}
		return s.GetSpaceAccessByKey(r.Context(), key, userID)
	})
}

func SpaceByPage(s *store.Store, perm domain.Perm) func(http.Handler) http.Handler {
	return spaceChain(s, perm, func(r *http.Request, userID uuid.UUID) (store.SpaceAccess, error) {
		pageID, err := URLUUID(r, "pageID")
		if err != nil {
			return store.SpaceAccess{}, err
		}
		return s.GetSpaceAccessByPage(r.Context(), pageID, userID)
	})
}

func SpaceByResolver(s *store.Store, resolve func(*http.Request) (uuid.UUID, error), perm domain.Perm) func(http.Handler) http.Handler {
	return spaceChain(s, perm, func(r *http.Request, userID uuid.UUID) (store.SpaceAccess, error) {
		spaceID, err := resolve(r)
		if err != nil {
			return store.SpaceAccess{}, err
		}
		return s.GetSpaceAccessByID(r.Context(), spaceID, userID)
	})
}

func spaceChain(s *store.Store, perm domain.Perm, lookup spaceLookup) func(http.Handler) http.Handler {
	auth := RequireAuth(s)
	resolve := resolveSpace(lookup)
	gate := RequireSpacePerm(perm)
	return func(next http.Handler) http.Handler {
		return auth(resolve(gate(next)))
	}
}

func resolveSpace(lookup spaceLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				WriteError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			access, err := lookup(r, u.ID)
			if err != nil {
				switch {
				case errors.Is(err, ErrInvalidID):
					WriteError(w, http.StatusBadRequest, "invalid id")
				case errors.Is(err, store.ErrNotFound):
					WriteError(w, http.StatusNotFound, "not found")
				default:
					WriteError(w, http.StatusInternalServerError, "internal error")
				}
				return
			}
			if access.DeletedAt != nil && !u.IsSystemAdmin {
				WriteError(w, http.StatusNotFound, "not found")
				return
			}
			role, ok := effectiveRole(u, access)
			if !ok {
				WriteError(w, http.StatusForbidden, "forbidden")
				return
			}
			sc := &SpaceCtx{
				ID:         access.ID,
				Key:        access.Key,
				Role:       role,
				Visibility: access.Visibility,
				Archived:   access.ArchivedAt != nil,
				Deleted:    access.DeletedAt != nil,
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSpace, sc)))
		})
	}
}

func effectiveRole(u *domain.User, a store.SpaceAccess) (domain.Role, bool) {
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

func RequireSpacePerm(perm domain.Perm) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc := SpaceFrom(r.Context())
			if sc == nil {
				WriteError(w, http.StatusForbidden, "forbidden")
				return
			}
			if !domain.RoleAllows(sc.Role, perm) {
				WriteError(w, http.StatusForbidden, "forbidden")
				return
			}
			if sc.ReadOnly() && sc.Role != domain.RoleAdmin && perm != domain.PermViewSpace {
				WriteError(w, http.StatusForbidden, "space is read-only")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
