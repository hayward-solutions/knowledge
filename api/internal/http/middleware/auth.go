package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const SessionCookie = "sid"

var CookieSecure = false

func NewSessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSessionToken
	ctxSpace
)

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(ctxUser).(*domain.User)
	return u
}

func SessionTokenFrom(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(ctxSessionToken).(string)
	return t, ok && t != ""
}

func authenticate(s *store.Store, r *http.Request) (*domain.User, string, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return nil, "", store.ErrNotFound
	}
	sess, err := s.GetValidSession(r.Context(), c.Value)
	if err != nil {
		return nil, "", err
	}
	u, err := s.GetUser(r.Context(), sess.UserID)
	if err != nil {
		return nil, "", err
	}
	if u.DisabledAt != nil {
		return nil, "", store.ErrNotFound
	}
	return &u, c.Value, nil
}

func withUser(r *http.Request, u *domain.User, token string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxUser, u)
	ctx = context.WithValue(ctx, ctxSessionToken, token)
	return r.WithContext(ctx)
}

func OptionalAuth(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, token, err := authenticate(s, r)
			if err == nil {
				r = withUser(r, u, token)
			} else if !errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAuth(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, token, err := authenticate(s, r)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					WriteError(w, http.StatusUnauthorized, "unauthenticated")
				} else {
					WriteError(w, http.StatusInternalServerError, "internal error")
				}
				return
			}
			next.ServeHTTP(w, withUser(r, u, token))
		})
	}
}

func RequireSystemAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil {
			WriteError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if !u.IsSystemAdmin {
			WriteError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
