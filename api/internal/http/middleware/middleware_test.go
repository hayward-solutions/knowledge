package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type fixture struct {
	t       *testing.T
	s       *store.Store
	router  http.Handler
	tokens  map[string]string
	users   map[string]domain.User
	pages   map[string]uuid.UUID
	spaces  map[string]uuid.UUID
	pageIDs map[string]uuid.UUID
}

func echo(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	if u := mw.UserFrom(r.Context()); u != nil {
		out["user"] = u.Email
	}
	if sc := mw.SpaceFrom(r.Context()); sc != nil {
		out["key"] = sc.Key
		out["role"] = sc.Role
		out["archived"] = sc.Archived
		out["visibility"] = sc.Visibility
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func newFixture(t *testing.T) *fixture {
	s := testutil.NewDB(t)
	f := &fixture{t: t, s: s, tokens: map[string]string{}, users: map[string]domain.User{}, spaces: map[string]uuid.UUID{}, pages: map[string]uuid.UUID{}}

	add := func(name string, admin bool) domain.User {
		u := testutil.CreateUser(t, s, name+"@example.test", "", admin)
		f.users[name] = u
		f.tokens[name] = testutil.NewSession(t, s, u.ID)
		return u
	}
	sysadmin := add("sysadmin", true)
	add("nobody", false)
	add("viewer", false)
	add("editor", false)
	add("spaceadmin", false)

	for _, sp := range []struct {
		key  string
		opts testutil.SpaceOpts
	}{
		{"INT", testutil.SpaceOpts{Key: "INT", Visibility: domain.VisibilityInternal}},
		{"PRIV", testutil.SpaceOpts{Key: "PRIV", Visibility: domain.VisibilityPrivate}},
		{"ARCH", testutil.SpaceOpts{Key: "ARCH", Visibility: domain.VisibilityInternal, Archived: true}},
		{"PARCH", testutil.SpaceOpts{Key: "PARCH", Visibility: domain.VisibilityPrivate, Archived: true}},
		{"GONE", testutil.SpaceOpts{Key: "GONE", Visibility: domain.VisibilityInternal, Deleted: true}},
	} {
		sp.opts.CreatedBy = sysadmin.ID
		id := testutil.CreateSpace(t, s, sp.opts)
		f.spaces[sp.key] = id
		testutil.AddMember(t, s, id, f.users["viewer"].ID, domain.RoleViewer)
		testutil.AddMember(t, s, id, f.users["editor"].ID, domain.RoleEditor)
		testutil.AddMember(t, s, id, f.users["spaceadmin"].ID, domain.RoleAdmin)
		f.pages[sp.key] = testutil.CreatePage(t, s, id, sysadmin.ID, "Page "+sp.key)
	}

	r := chi.NewRouter()
	r.With(mw.SpaceByKey(s, domain.PermViewSpace)).Get("/spaces/{key}", echo)
	r.With(mw.SpaceByKey(s, domain.PermComment)).Post("/spaces/{key}/comments", echo)
	r.With(mw.SpaceByKey(s, domain.PermEditPages)).Post("/spaces/{key}/pages", echo)
	r.With(mw.SpaceByKey(s, domain.PermManageSpace)).Patch("/spaces/{key}", echo)
	r.With(mw.SpaceByPage(s, domain.PermViewSpace)).Get("/pages/{pageID}", echo)
	r.With(mw.SpaceByPage(s, domain.PermEditPages)).Patch("/pages/{pageID}", echo)
	r.With(mw.SpaceByResolver(s, func(r *http.Request) (uuid.UUID, error) {
		switch chi.URLParam(r, "commentID") {
		case "ok-priv":
			return f.spaces["PRIV"], nil
		case "bad":
			return uuid.Nil, mw.ErrInvalidID
		}
		return uuid.Nil, store.ErrNotFound
	}, domain.PermViewSpace)).Get("/comments/{commentID}", echo)
	r.With(mw.RequireAuth(s), mw.RequireSystemAdmin).Get("/admin/ping", echo)
	r.With(mw.OptionalAuth(s)).Get("/optional", echo)
	f.router = r
	return f
}

type result struct {
	code int
	body map[string]any
	raw  string
	ct   string
}

func (f *fixture) do(method, path, who string) result {
	req := httptest.NewRequest(method, path, nil)
	if who != "" {
		tok := f.tokens[who]
		if tok == "" {
			tok = who
		}
		req.AddCookie(&http.Cookie{Name: mw.SessionCookie, Value: tok})
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	res := result{code: rec.Code, raw: rec.Body.String(), ct: rec.Header().Get("Content-Type")}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func TestRequireAuth(t *testing.T) {
	f := newFixture(t)

	for name, who := range map[string]string{"no cookie": "", "garbage token": "garbage", "unknown token": strings.Repeat("a", 43)} {
		res := f.do("GET", "/spaces/INT", who)
		if res.code != 401 {
			t.Errorf("%s: code %d", name, res.code)
		}
		if res.body["error"] != "unauthenticated" || res.ct != "application/json" {
			t.Errorf("%s: body %q ct %q", name, res.raw, res.ct)
		}
	}

	if err := f.s.DeleteSession(t.Context(), f.tokens["viewer"]); err != nil {
		t.Fatal(err)
	}
	if res := f.do("GET", "/spaces/INT", "viewer"); res.code != 401 {
		t.Errorf("deleted session: code %d", res.code)
	}

	disabled := true
	if _, err := f.s.AdminUpdateUser(t.Context(), f.users["editor"].ID, store.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if res := f.do("GET", "/spaces/INT", "editor"); res.code != 401 {
		t.Errorf("disabled user: code %d", res.code)
	}
}

func TestSystemAdminAndOptionalAuth(t *testing.T) {
	f := newFixture(t)
	if res := f.do("GET", "/admin/ping", ""); res.code != 401 {
		t.Errorf("anon admin: %d", res.code)
	}
	if res := f.do("GET", "/admin/ping", "nobody"); res.code != 403 || res.body["error"] != "forbidden" {
		t.Errorf("non-admin: %d %s", res.code, res.raw)
	}
	if res := f.do("GET", "/admin/ping", "sysadmin"); res.code != 200 {
		t.Errorf("admin: %d", res.code)
	}
	if res := f.do("GET", "/optional", ""); res.code != 200 || res.body["user"] != nil {
		t.Errorf("optional anon: %d %s", res.code, res.raw)
	}
	if res := f.do("GET", "/optional", "viewer"); res.code != 200 || res.body["user"] != "viewer@example.test" {
		t.Errorf("optional authed: %d %s", res.code, res.raw)
	}
	if res := f.do("GET", "/optional", "bogus"); res.code != 200 || res.body["user"] != nil {
		t.Errorf("optional bogus: %d %s", res.code, res.raw)
	}
}

func TestSpaceRoleResolution(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name     string
		method   string
		path     string
		who      string
		wantCode int
		wantRole string
	}{
		{"internal: non-member views as viewer", "GET", "/spaces/INT", "nobody", 200, "viewer"},
		{"internal: non-member lowercase key", "GET", "/spaces/int", "nobody", 200, "viewer"},
		{"internal: non-member can comment", "POST", "/spaces/INT/comments", "nobody", 200, "viewer"},
		{"internal: non-member cannot edit", "POST", "/spaces/INT/pages", "nobody", 403, ""},
		{"internal: member viewer cannot edit", "POST", "/spaces/INT/pages", "viewer", 403, ""},
		{"internal: member editor edits", "POST", "/spaces/INT/pages", "editor", 200, "editor"},
		{"internal: editor cannot manage", "PATCH", "/spaces/INT", "editor", 403, ""},
		{"internal: space admin manages", "PATCH", "/spaces/INT", "spaceadmin", 200, "admin"},
		{"internal: system admin manages", "PATCH", "/spaces/INT", "sysadmin", 200, "admin"},

		{"private: non-member forbidden", "GET", "/spaces/PRIV", "nobody", 403, ""},
		{"private: member viewer views", "GET", "/spaces/PRIV", "viewer", 200, "viewer"},
		{"private: member viewer cannot edit", "POST", "/spaces/PRIV/pages", "viewer", 403, ""},
		{"private: member editor edits", "POST", "/spaces/PRIV/pages", "editor", 200, "editor"},
		{"private: system admin non-member is admin", "PATCH", "/spaces/PRIV", "sysadmin", 200, "admin"},
		{"private: space admin manages", "PATCH", "/spaces/PRIV", "spaceadmin", 200, "admin"},

		{"archived: non-member can view", "GET", "/spaces/ARCH", "nobody", 200, "viewer"},
		{"archived: viewer cannot comment", "POST", "/spaces/ARCH/comments", "viewer", 403, ""},
		{"archived: editor cannot edit", "POST", "/spaces/ARCH/pages", "editor", 403, ""},
		{"archived: editor can view", "GET", "/spaces/ARCH", "editor", 200, "editor"},
		{"archived: space admin can still write", "POST", "/spaces/ARCH/pages", "spaceadmin", 200, "admin"},
		{"archived: system admin can still write", "POST", "/spaces/ARCH/pages", "sysadmin", 200, "admin"},
		{"archived private: editor cannot edit", "POST", "/spaces/PARCH/pages", "editor", 403, ""},
		{"archived private: non-member forbidden", "GET", "/spaces/PARCH", "nobody", 403, ""},

		{"deleted: member not found", "GET", "/spaces/GONE", "editor", 404, ""},
		{"deleted: non-member not found", "GET", "/spaces/GONE", "nobody", 404, ""},
		{"deleted: system admin still resolves", "GET", "/spaces/GONE", "sysadmin", 200, "admin"},

		{"unknown space", "GET", "/spaces/NOPE", "sysadmin", 404, ""},
		{"anonymous", "GET", "/spaces/INT", "", 401, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := f.do(c.method, c.path, c.who)
			if res.code != c.wantCode {
				t.Fatalf("code %d (%s), want %d", res.code, res.raw, c.wantCode)
			}
			if res.ct != "application/json" {
				t.Errorf("content-type %q", res.ct)
			}
			if c.wantCode == 200 && res.body["role"] != c.wantRole {
				t.Errorf("role %v, want %s", res.body["role"], c.wantRole)
			}
			if c.wantCode != 200 && res.body["error"] == nil {
				t.Errorf("missing error envelope: %s", res.raw)
			}
		})
	}
}

func TestSpaceContextFields(t *testing.T) {
	f := newFixture(t)
	res := f.do("GET", "/spaces/ARCH", "editor")
	if res.body["key"] != "ARCH" || res.body["archived"] != true || res.body["visibility"] != "internal" {
		t.Fatalf("ctx = %s", res.raw)
	}
	res = f.do("GET", "/spaces/PRIV", "viewer")
	if res.body["archived"] != false || res.body["visibility"] != "private" {
		t.Fatalf("ctx = %s", res.raw)
	}
}

func TestSpaceByPage(t *testing.T) {
	f := newFixture(t)
	page := func(key string) string { return "/pages/" + f.pages[key].String() }

	cases := []struct {
		name     string
		method   string
		path     string
		who      string
		wantCode int
		wantKey  string
	}{
		{"internal page viewable by non-member", "GET", page("INT"), "nobody", 200, "INT"},
		{"private page forbidden to non-member", "GET", page("PRIV"), "nobody", 403, ""},
		{"private page viewable by member", "GET", page("PRIV"), "viewer", 200, "PRIV"},
		{"private page editable by editor", "PATCH", page("PRIV"), "editor", 200, "PRIV"},
		{"private page not editable by viewer", "PATCH", page("PRIV"), "viewer", 403, ""},
		{"archived page not editable by editor", "PATCH", page("ARCH"), "editor", 403, ""},
		{"archived page editable by space admin", "PATCH", page("ARCH"), "spaceadmin", 200, "ARCH"},
		{"deleted space page hidden", "GET", page("GONE"), "viewer", 404, ""},
		{"system admin sees private page", "GET", page("PRIV"), "sysadmin", 200, "PRIV"},
		{"unknown page", "GET", "/pages/" + uuid.NewString(), "sysadmin", 404, ""},
		{"malformed page id", "GET", "/pages/not-a-uuid", "sysadmin", 400, ""},
		{"anonymous", "GET", page("INT"), "", 401, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := f.do(c.method, c.path, c.who)
			if res.code != c.wantCode {
				t.Fatalf("code %d (%s), want %d", res.code, res.raw, c.wantCode)
			}
			if c.wantCode == 200 && res.body["key"] != c.wantKey {
				t.Errorf("key %v, want %s", res.body["key"], c.wantKey)
			}
		})
	}
}

func TestSpaceByResolver(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name     string
		path     string
		who      string
		wantCode int
	}{
		{"resolved private space, member", "/comments/ok-priv", "viewer", 200},
		{"resolved private space, non-member", "/comments/ok-priv", "nobody", 403},
		{"resolver not found", "/comments/missing", "sysadmin", 404},
		{"resolver invalid id", "/comments/bad", "sysadmin", 400},
		{"anonymous", "/comments/ok-priv", "", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if res := f.do("GET", c.path, c.who); res.code != c.wantCode {
				t.Fatalf("code %d (%s), want %d", res.code, res.raw, c.wantCode)
			}
		})
	}
}

func TestRequireSpacePermWithoutSpaceContext(t *testing.T) {
	h := mw.RequireSpacePerm(domain.PermViewSpace)(http.HandlerFunc(echo))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 403 {
		t.Fatalf("code %d", rec.Code)
	}
}
