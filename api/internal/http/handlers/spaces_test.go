package handlers_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func spUser(t *testing.T, e *env, email string, admin bool) (domain.User, *client) {
	t.Helper()
	u := testutil.CreateUser(t, e.store, email, "", admin)
	c := e.client()
	c.mustLogin(email, testutil.DefaultPassword)
	return u, c
}

func spPage(t *testing.T, e *env, spaceID, createdBy uuid.UUID, title, status string, parent *uuid.UUID, position int) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := e.store.Pool.Exec(context.Background(),
		`INSERT INTO pages (id, space_id, parent_id, title, slug, position, status, created_by, updated_by, trashed_at, trashed_by)
		 VALUES ($1, $2, $3, $4, $4, $5, $6::text, $7::uuid, $7::uuid, CASE WHEN $6::text = 'trashed' THEN now() END, CASE WHEN $6::text = 'trashed' THEN $7::uuid END)`,
		id, spaceID, parent, title, position, status, createdBy)
	if err != nil {
		t.Fatalf("insert page %s: %v", title, err)
	}
	return id
}

func spCreate(t *testing.T, c *client, key, name, visibility string) map[string]any {
	t.Helper()
	body := map[string]any{"key": key, "name": name}
	if visibility != "" {
		body["visibility"] = visibility
	}
	r := c.do("POST", "/api/v1/spaces", body)
	if r.status != 201 {
		t.Fatalf("create space %s: %d %s", key, r.status, r.raw)
	}
	return r.body
}

func spKeys(list []map[string]any) []string {
	out := []string{}
	for _, s := range list {
		out = append(out, s["key"].(string))
	}
	return out
}

func spEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpacesCreate(t *testing.T) {
	e := newEnv(t, nil)
	creator, c := spUser(t, e, "creator@example.test", false)

	if r := e.client().do("POST", "/api/v1/spaces", map[string]any{"key": "ENG", "name": "Eng"}); r.status != 401 {
		t.Fatalf("anon create: %d", r.status)
	}

	bad := []map[string]any{
		{"key": "", "name": "x"},
		{"key": "A", "name": "x"},
		{"key": "1AB", "name": "x"},
		{"key": "A-B", "name": "x"},
		{"key": "ABCDEFGHIJK", "name": "x"},
		{"key": "ÄBC", "name": "x"},
		{"key": "ENG", "name": "   "},
		{"key": "ENG"},
		{"key": "ENG", "name": "x", "visibility": "public"},
	}
	for _, b := range bad {
		if r := c.do("POST", "/api/v1/spaces", b); r.status != 400 {
			t.Errorf("body %v: %d %s", b, r.status, r.raw)
		}
	}

	sp := spCreate(t, c, "  eng ", "  Engineering  ", "")
	if sp["key"] != "ENG" || sp["name"] != "Engineering" || sp["visibility"] != "internal" || sp["role"] != "admin" {
		t.Fatalf("created: %v", sp)
	}
	if sp["member_count"] != float64(1) || sp["page_count"] != float64(1) || sp["archived_at"] != nil || sp["created_by"] != creator.ID.String() {
		t.Fatalf("created counts: %v", sp)
	}
	homeID, _ := sp["home_page_id"].(string)
	if homeID == "" {
		t.Fatalf("home page not linked: %v", sp)
	}

	var title, slug, status string
	var version int
	err := e.store.Pool.QueryRow(context.Background(),
		`SELECT title, slug, status, version FROM pages WHERE id = $1 AND space_id = $2`, homeID, sp["id"]).
		Scan(&title, &slug, &status, &version)
	if err != nil || title != "Engineering" || slug != "engineering" || status != "published" || version != 1 {
		t.Fatalf("home page row: %q %q %q %d %v", title, slug, status, version, err)
	}

	for _, key := range []string{"ENG", "eng", " Eng "} {
		if r := c.do("POST", "/api/v1/spaces", map[string]any{"key": key, "name": "Dup"}); r.status != 409 {
			t.Errorf("duplicate %q: %d %s", key, r.status, r.raw)
		}
	}

	members := c.do("GET", "/api/v1/spaces/ENG/members", nil)
	if members.status != 200 || len(members.list) != 1 || members.list[0]["role"] != "admin" || members.list[0]["user_id"] != creator.ID.String() {
		t.Fatalf("creator membership: %d %s", members.status, members.raw)
	}

	got := c.do("GET", "/api/v1/spaces/eng", nil)
	if got.status != 200 || got.body["key"] != "ENG" || got.body["role"] != "admin" || got.body["home_page_id"] != homeID {
		t.Fatalf("get: %d %s", got.status, got.raw)
	}

	icon := c.do("POST", "/api/v1/spaces", map[string]any{"key": "OPS", "name": "Ops", "description": "run", "icon": "wrench", "visibility": "private"})
	if icon.status != 201 || icon.body["icon"] != "wrench" || icon.body["description"] != "run" || icon.body["visibility"] != "private" {
		t.Fatalf("create with options: %d %s", icon.status, icon.raw)
	}
}

func TestSpacesListRoles(t *testing.T) {
	e := newEnv(t, nil)
	owner, ownerC := spUser(t, e, "owner@example.test", false)
	member, memberC := spUser(t, e, "member@example.test", false)
	_, outsiderC := spUser(t, e, "outsider@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	spCreate(t, ownerC, "PUB", "Bravo", "internal")
	spCreate(t, ownerC, "PRIV", "alpha", "private")
	spCreate(t, ownerC, "GONE", "Zulu", "internal")
	spCreate(t, ownerC, "ARCH", "Charlie", "internal")
	if r := ownerC.do("PUT", "/api/v1/spaces/PRIV/members/"+member.ID.String(), map[string]any{"role": "editor"}); r.status != 200 {
		t.Fatalf("add member: %d %s", r.status, r.raw)
	}
	if r := ownerC.do("POST", "/api/v1/spaces/ARCH/archive", nil); r.status != 200 {
		t.Fatalf("archive: %d %s", r.status, r.raw)
	}
	if r := ownerC.do("DELETE", "/api/v1/spaces/GONE", nil); r.status != 204 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}

	if anon := e.client().do("GET", "/api/v1/spaces", nil); anon.status != 401 {
		t.Fatalf("anon list: %d", anon.status)
	}

	byKey := func(r response) map[string]map[string]any {
		m := map[string]map[string]any{}
		for _, s := range r.list {
			m[s["key"].(string)] = s
		}
		return m
	}

	r := memberC.do("GET", "/api/v1/spaces", nil)
	if r.status != 200 || !spEqual(spKeys(r.list), []string{"PRIV", "PUB", "ARCH"}) {
		t.Fatalf("member list: %d %s", r.status, r.raw)
	}
	m := byKey(r)
	if m["PRIV"]["role"] != "editor" || m["PUB"]["role"] != "viewer" || m["ARCH"]["role"] != "viewer" {
		t.Fatalf("member roles: %s", r.raw)
	}
	if m["PRIV"]["member_count"] != float64(2) || m["PUB"]["member_count"] != float64(1) || m["PUB"]["page_count"] != float64(1) {
		t.Fatalf("counts: %s", r.raw)
	}
	if m["ARCH"]["archived_at"] == nil {
		t.Fatalf("archived spaces must be included with archived_at: %s", r.raw)
	}

	r = outsiderC.do("GET", "/api/v1/spaces", nil)
	if !spEqual(spKeys(r.list), []string{"PUB", "ARCH"}) {
		t.Fatalf("outsider list: %s", r.raw)
	}

	r = ownerC.do("GET", "/api/v1/spaces", nil)
	if !spEqual(spKeys(r.list), []string{"PRIV", "PUB", "ARCH"}) || byKey(r)["PRIV"]["role"] != "admin" {
		t.Fatalf("owner list: %s", r.raw)
	}

	r = rootC.do("GET", "/api/v1/spaces", nil)
	if !spEqual(spKeys(r.list), []string{"PRIV", "PUB", "ARCH"}) {
		t.Fatalf("system admin list: %s", r.raw)
	}
	for _, s := range r.list {
		if s["role"] != "admin" {
			t.Fatalf("system admin role: %v", s)
		}
	}
	_ = owner
}

func TestSpacesPrivateVisibility(t *testing.T) {
	e := newEnv(t, nil)
	_, ownerC := spUser(t, e, "owner@example.test", false)
	viewer, viewerC := spUser(t, e, "viewer@example.test", false)
	_, outsiderC := spUser(t, e, "outsider@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	spCreate(t, ownerC, "SEC", "Secret", "private")
	ownerC.do("PUT", "/api/v1/spaces/SEC/members/"+viewer.ID.String(), map[string]any{"role": "viewer"})

	for _, path := range []string{"", "/members", "/tree"} {
		if r := outsiderC.do("GET", "/api/v1/spaces/SEC"+path, nil); r.status != 404 {
			t.Errorf("outsider GET %q: %d %s", path, r.status, r.raw)
		}
		if r := viewerC.do("GET", "/api/v1/spaces/SEC"+path, nil); r.status != 200 {
			t.Errorf("viewer GET %q: %d %s", path, r.status, r.raw)
		}
		if r := rootC.do("GET", "/api/v1/spaces/SEC"+path, nil); r.status != 200 {
			t.Errorf("root GET %q: %d %s", path, r.status, r.raw)
		}
	}
	if r := viewerC.do("GET", "/api/v1/spaces/SEC", nil); r.body["role"] != "viewer" {
		t.Fatalf("viewer role: %s", r.raw)
	}
	if r := outsiderC.do("GET", "/api/v1/spaces/NOPE", nil); r.status != 404 {
		t.Fatalf("unknown key: %d", r.status)
	}

	if r := ownerC.do("PATCH", "/api/v1/spaces/SEC", map[string]any{"visibility": "internal"}); r.status != 200 || r.body["visibility"] != "internal" {
		t.Fatalf("make internal: %d %s", r.status, r.raw)
	}
	if r := outsiderC.do("GET", "/api/v1/spaces/SEC", nil); r.status != 200 || r.body["role"] != "viewer" {
		t.Fatalf("outsider after internal: %d %s", r.status, r.raw)
	}
}

func TestSpacesUpdateAndDelete(t *testing.T) {
	e := newEnv(t, nil)
	owner, ownerC := spUser(t, e, "owner@example.test", false)
	editor, editorC := spUser(t, e, "editor@example.test", false)
	_, viewerC := spUser(t, e, "viewer@example.test", false)

	sp := spCreate(t, ownerC, "DOC", "Docs", "internal")
	spaceID := uuid.MustParse(sp["id"].(string))
	ownerC.do("PUT", "/api/v1/spaces/DOC/members/"+editor.ID.String(), map[string]any{"role": "editor"})

	if r := editorC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"name": "Nope"}); r.status != 403 {
		t.Fatalf("editor patch: %d", r.status)
	}
	if r := viewerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"name": "Nope"}); r.status != 403 {
		t.Fatalf("viewer patch: %d", r.status)
	}
	if r := e.client().do("PATCH", "/api/v1/spaces/DOC", map[string]any{"name": "Nope"}); r.status != 401 {
		t.Fatalf("anon patch: %d", r.status)
	}

	r := ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"name": " Documentation ", "description": "all the docs", "icon": "book"})
	if r.status != 200 || r.body["name"] != "Documentation" || r.body["description"] != "all the docs" || r.body["icon"] != "book" || r.body["role"] != "admin" {
		t.Fatalf("patch: %d %s", r.status, r.raw)
	}
	if r.body["member_count"] != float64(2) || r.body["home_page_id"] != sp["home_page_id"] {
		t.Fatalf("patch kept fields: %s", r.raw)
	}
	r = ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"icon": nil})
	if r.status != 200 || r.body["icon"] != nil || r.body["name"] != "Documentation" {
		t.Fatalf("clear icon: %d %s", r.status, r.raw)
	}
	r = ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{})
	if r.status != 200 || r.body["name"] != "Documentation" {
		t.Fatalf("empty patch: %d %s", r.status, r.raw)
	}

	for _, body := range []map[string]any{
		{"name": ""},
		{"name": 5},
		{"visibility": "public"},
		{"home_page_id": "not-a-uuid"},
		{"icon": 5},
	} {
		if r := ownerC.do("PATCH", "/api/v1/spaces/DOC", body); r.status != 400 {
			t.Errorf("patch %v: %d %s", body, r.status, r.raw)
		}
	}

	child := spPage(t, e, spaceID, owner.ID, "Child", "published", nil, 1)
	r = ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"home_page_id": child.String()})
	if r.status != 200 || r.body["home_page_id"] != child.String() {
		t.Fatalf("set home page: %d %s", r.status, r.raw)
	}
	other := spCreate(t, ownerC, "OTH", "Other", "internal")
	if r := ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"home_page_id": other["home_page_id"]}); r.status != 400 {
		t.Fatalf("foreign home page: %d %s", r.status, r.raw)
	}
	trashed := spPage(t, e, spaceID, owner.ID, "Bin", "trashed", nil, 2)
	if r := ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"home_page_id": trashed.String()}); r.status != 400 {
		t.Fatalf("trashed home page: %d %s", r.status, r.raw)
	}
	if r := ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"home_page_id": uuid.NewString()}); r.status != 400 {
		t.Fatalf("missing home page: %d %s", r.status, r.raw)
	}
	r = ownerC.do("PATCH", "/api/v1/spaces/DOC", map[string]any{"home_page_id": nil})
	if r.status != 200 || r.body["home_page_id"] != nil {
		t.Fatalf("clear home page: %d %s", r.status, r.raw)
	}

	if r := editorC.do("DELETE", "/api/v1/spaces/DOC", nil); r.status != 403 {
		t.Fatalf("editor delete: %d", r.status)
	}
	if r := ownerC.do("DELETE", "/api/v1/spaces/DOC", nil); r.status != 204 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}
	if r := ownerC.do("GET", "/api/v1/spaces/DOC", nil); r.status != 404 {
		t.Fatalf("get deleted: %d", r.status)
	}
	if r := ownerC.do("GET", "/api/v1/spaces", nil); !spEqual(spKeys(r.list), []string{"OTH"}) {
		t.Fatalf("list after delete: %s", r.raw)
	}
	if r := ownerC.do("POST", "/api/v1/spaces", map[string]any{"key": "DOC", "name": "Again"}); r.status != 409 {
		t.Fatalf("deleted keys stay reserved: %d", r.status)
	}
}

func TestSpacesArchive(t *testing.T) {
	e := newEnv(t, nil)
	_, adminC := spUser(t, e, "admin@example.test", false)
	editor, editorC := spUser(t, e, "editor@example.test", false)
	viewer, viewerC := spUser(t, e, "viewer@example.test", false)

	spCreate(t, adminC, "ARC", "Archive me", "internal")
	adminC.do("PUT", "/api/v1/spaces/ARC/members/"+editor.ID.String(), map[string]any{"role": "editor"})
	adminC.do("PUT", "/api/v1/spaces/ARC/members/"+viewer.ID.String(), map[string]any{"role": "viewer"})

	if r := editorC.do("POST", "/api/v1/spaces/ARC/archive", nil); r.status != 403 {
		t.Fatalf("editor archive: %d", r.status)
	}
	r := adminC.do("POST", "/api/v1/spaces/ARC/archive", nil)
	if r.status != 200 || r.body["archived_at"] == nil || r.body["role"] != "admin" {
		t.Fatalf("archive: %d %s", r.status, r.raw)
	}
	if r := adminC.do("POST", "/api/v1/spaces/ARC/archive", nil); r.status != 200 {
		t.Fatalf("archive twice: %d", r.status)
	}

	if r := viewerC.do("GET", "/api/v1/spaces/ARC", nil); r.status != 200 || r.body["archived_at"] == nil {
		t.Fatalf("viewer can still read: %d %s", r.status, r.raw)
	}
	if r := viewerC.do("GET", "/api/v1/spaces/ARC/tree", nil); r.status != 200 {
		t.Fatalf("tree read on archived: %d", r.status)
	}
	for _, req := range [][3]string{
		{"PATCH", "/api/v1/spaces/ARC", "name"},
		{"PUT", "/api/v1/spaces/ARC/members/" + viewer.ID.String(), "role"},
		{"DELETE", "/api/v1/spaces/ARC/members/" + viewer.ID.String(), ""},
		{"POST", "/api/v1/spaces/ARC/unarchive", ""},
	} {
		var body any
		switch req[2] {
		case "name":
			body = map[string]any{"name": "x"}
		case "role":
			body = map[string]any{"role": "editor"}
		}
		if r := editorC.do(req[0], req[1], body); r.status != 403 {
			t.Errorf("editor %s %s on archived space: %d %s", req[0], req[1], r.status, r.raw)
		}
	}

	if r := adminC.do("PATCH", "/api/v1/spaces/ARC", map[string]any{"description": "still editable by admin"}); r.status != 200 {
		t.Fatalf("admin patch archived: %d %s", r.status, r.raw)
	}
	r = adminC.do("POST", "/api/v1/spaces/ARC/unarchive", nil)
	if r.status != 200 || r.body["archived_at"] != nil {
		t.Fatalf("unarchive: %d %s", r.status, r.raw)
	}
	if r := adminC.do("POST", "/api/v1/spaces/ARC/unarchive", nil); r.status != 200 {
		t.Fatalf("unarchive twice: %d", r.status)
	}
}

func TestSpacesMembers(t *testing.T) {
	e := newEnv(t, nil)
	creator, creatorC := spUser(t, e, "creator@example.test", false)
	amy, _ := spUser(t, e, "amy@example.test", false)
	bob, bobC := spUser(t, e, "bob@example.test", false)
	off := testutil.CreateUser(t, e.store, "off@example.test", "", false)
	disabled := true
	if _, err := e.store.AdminUpdateUser(context.Background(), off.ID, store.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	spCreate(t, creatorC, "MEM", "Members", "internal")
	base := "/api/v1/spaces/MEM/members/"

	r := creatorC.do("PUT", base+amy.ID.String(), map[string]any{"role": "editor"})
	if r.status != 200 || r.body["role"] != "editor" || r.body["user_id"] != amy.ID.String() || r.body["space_id"] == nil || r.body["created_at"] == nil {
		t.Fatalf("add amy: %d %s", r.status, r.raw)
	}
	user, _ := r.body["user"].(map[string]any)
	if user["id"] != amy.ID.String() || user["display_name"] != "amy@example.test" {
		t.Fatalf("member user ref: %s", r.raw)
	}
	if r := creatorC.do("PUT", base+amy.ID.String(), map[string]any{"role": "viewer"}); r.status != 200 || r.body["role"] != "viewer" {
		t.Fatalf("update amy: %d %s", r.status, r.raw)
	}
	creatorC.do("PUT", base+bob.ID.String(), map[string]any{"role": "viewer"})

	list := creatorC.do("GET", "/api/v1/spaces/MEM/members", nil)
	if list.status != 200 || len(list.list) != 3 {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
	if list.list[0]["user_id"] != amy.ID.String() || list.list[1]["user_id"] != bob.ID.String() || list.list[2]["user_id"] != creator.ID.String() {
		t.Fatalf("members not sorted by display name: %s", list.raw)
	}

	if r := bobC.do("PUT", base+bob.ID.String(), map[string]any{"role": "admin"}); r.status != 403 {
		t.Fatalf("viewer self-promote: %d", r.status)
	}
	if r := bobC.do("DELETE", base+amy.ID.String(), nil); r.status != 403 {
		t.Fatalf("viewer remove: %d", r.status)
	}
	if r := bobC.do("GET", "/api/v1/spaces/MEM/members", nil); r.status != 200 {
		t.Fatalf("viewer can list members: %d", r.status)
	}

	for name, req := range map[string]struct {
		path string
		body any
		want int
	}{
		"bad role":     {base + amy.ID.String(), map[string]any{"role": "owner"}, 400},
		"empty role":   {base + amy.ID.String(), map[string]any{}, 400},
		"bad id":       {base + "nope", map[string]any{"role": "viewer"}, 400},
		"unknown user": {base + uuid.NewString(), map[string]any{"role": "viewer"}, 404},
		"disabled":     {base + off.ID.String(), map[string]any{"role": "viewer"}, 409},
	} {
		if r := creatorC.do("PUT", req.path, req.body); r.status != req.want {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}

	if r := creatorC.do("PUT", base+creator.ID.String(), map[string]any{"role": "editor"}); r.status != 409 || r.body["code"] != "last_admin" {
		t.Fatalf("demote last admin: %d %s", r.status, r.raw)
	}
	if r := creatorC.do("DELETE", base+creator.ID.String(), nil); r.status != 409 {
		t.Fatalf("remove last admin: %d %s", r.status, r.raw)
	}

	if r := creatorC.do("PUT", base+bob.ID.String(), map[string]any{"role": "admin"}); r.status != 200 {
		t.Fatalf("promote bob: %d", r.status)
	}
	if r := creatorC.do("PUT", base+creator.ID.String(), map[string]any{"role": "editor"}); r.status != 200 {
		t.Fatalf("demote creator with bob as admin: %d %s", r.status, r.raw)
	}
	if r := creatorC.do("PUT", base+bob.ID.String(), map[string]any{"role": "editor"}); r.status != 403 {
		t.Fatalf("demoted creator can no longer manage: %d", r.status)
	}
	if r := bobC.do("DELETE", base+bob.ID.String(), nil); r.status != 409 {
		t.Fatalf("bob removing himself as last admin: %d %s", r.status, r.raw)
	}
	if r := bobC.do("DELETE", base+amy.ID.String(), nil); r.status != 204 {
		t.Fatalf("remove amy: %d", r.status)
	}
	if r := bobC.do("DELETE", base+amy.ID.String(), nil); r.status != 404 {
		t.Fatalf("remove amy twice: %d", r.status)
	}
	if r := bobC.do("DELETE", base+"nope", nil); r.status != 400 {
		t.Fatalf("remove bad id: %d", r.status)
	}
}

func TestSpacesLastAdminIgnoresSystemAdmins(t *testing.T) {
	e := newEnv(t, nil)
	creator, creatorC := spUser(t, e, "creator@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	spCreate(t, creatorC, "ROOT", "Root", "internal")
	path := "/api/v1/spaces/ROOT/members/" + creator.ID.String()
	if r := rootC.do("PUT", path, map[string]any{"role": "viewer"}); r.status != 409 {
		t.Fatalf("system admin demoting the only member admin: %d %s", r.status, r.raw)
	}
	if r := rootC.do("DELETE", path, nil); r.status != 409 {
		t.Fatalf("system admin removing the only member admin: %d %s", r.status, r.raw)
	}
}

func TestSpacesMemberAddedEvent(t *testing.T) {
	s := testutil.NewDB(t)
	bus := events.NewBus()
	var mu sync.Mutex
	var got []domain.SpaceMemberAdded
	bus.Subscribe(func(ev any) {
		if m, ok := ev.(domain.SpaceMemberAdded); ok {
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
		}
	})
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store: s, Bus: bus, Hub: events.NewHub(), Presence: presence,
		Config: &config.Config{AppOrigins: []string{"http://app.test"}}, Version: "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	e := &env{t: t, store: s, server: srv}

	actor, c := spUser(t, e, "actor@example.test", false)
	target, _ := spUser(t, e, "target@example.test", false)
	spCreate(t, c, "EVT", "Events", "internal")
	path := "/api/v1/spaces/EVT/members/" + target.ID.String()

	c.do("PUT", path, map[string]any{"role": "editor"})
	c.do("PUT", path, map[string]any{"role": "viewer"})
	bus.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected exactly one SpaceMemberAdded on insert, got %d: %+v", len(got), got)
	}
	if got[0].UserID != target.ID || got[0].ActorID != actor.ID || got[0].Role != domain.RoleEditor || got[0].SpaceID == uuid.Nil {
		t.Fatalf("event: %+v", got[0])
	}
}

func TestSpacesTreeVisibility(t *testing.T) {
	e := newEnv(t, nil)
	admin, adminC := spUser(t, e, "admin@example.test", false)
	editor, editorC := spUser(t, e, "editor@example.test", false)
	other, otherC := spUser(t, e, "other@example.test", false)
	_, viewerC := spUser(t, e, "viewer@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	sp := spCreate(t, adminC, "TRE", "Tree", "internal")
	spaceID := uuid.MustParse(sp["id"].(string))
	home := uuid.MustParse(sp["home_page_id"].(string))
	adminC.do("PUT", "/api/v1/spaces/TRE/members/"+editor.ID.String(), map[string]any{"role": "editor"})
	adminC.do("PUT", "/api/v1/spaces/TRE/members/"+other.ID.String(), map[string]any{"role": "editor"})

	a := spPage(t, e, spaceID, editor.ID, "A", "published", &home, 1)
	b := spPage(t, e, spaceID, editor.ID, "B", "published", &home, 0)
	spPage(t, e, spaceID, editor.ID, "A child", "published", &a, 0)
	spPage(t, e, spaceID, editor.ID, "Editor draft", "draft", &b, 0)
	spPage(t, e, spaceID, other.ID, "Other draft", "draft", &b, 1)
	spPage(t, e, spaceID, admin.ID, "Admin draft", "draft", nil, 9)
	spPage(t, e, spaceID, editor.ID, "Binned", "trashed", &b, 2)

	r := editorC.do("GET", "/api/v1/spaces/TRE/tree", nil)
	if r.status != 200 {
		t.Fatalf("editor tree: %d %s", r.status, r.raw)
	}
	have := map[string]map[string]any{}
	for _, p := range r.list {
		have[p["title"].(string)] = p
		for _, f := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children"} {
			if _, ok := p[f]; !ok {
				t.Fatalf("tree item missing %s: %v", f, p)
			}
		}
	}
	if len(r.list) != 5 || have["Editor draft"] == nil || have["Other draft"] != nil || have["Admin draft"] != nil || have["Binned"] != nil {
		t.Fatalf("editor visibility: %s", r.raw)
	}
	if have["Tree"]["parent_id"] != nil || have["B"]["parent_id"] != home.String() || have["Editor draft"]["status"] != "draft" {
		t.Fatalf("tree fields: %s", r.raw)
	}
	if have["B"]["has_children"] != true || have["A"]["has_children"] != true || have["A child"]["has_children"] != false || have["Tree"]["has_children"] != true {
		t.Fatalf("has_children: %s", r.raw)
	}
	if r.list[0]["id"] != home.String() {
		t.Fatalf("root page first: %s", r.raw)
	}

	r = viewerC.do("GET", "/api/v1/spaces/TRE/tree", nil)
	if len(r.list) != 4 {
		t.Fatalf("viewer sees only published: %s", r.raw)
	}
	for _, p := range r.list {
		if p["status"] != "published" {
			t.Fatalf("viewer saw non-published: %v", p)
		}
		if p["title"] == "B" && p["has_children"] != false {
			t.Fatalf("B has only hidden children for a viewer: %v", p)
		}
	}

	r = otherC.do("GET", "/api/v1/spaces/TRE/tree", nil)
	have = map[string]map[string]any{}
	for _, p := range r.list {
		have[p["title"].(string)] = p
	}
	if len(r.list) != 5 || have["Other draft"] == nil || have["Editor draft"] != nil {
		t.Fatalf("other editor tree: %s", r.raw)
	}

	for name, c := range map[string]*client{"space admin": adminC, "system admin": rootC} {
		r = c.do("GET", "/api/v1/spaces/TRE/tree", nil)
		if r.status != 200 {
			t.Fatalf("%s tree: %d %s", name, r.status, r.raw)
		}
		titles := map[string]bool{}
		for _, p := range r.list {
			titles[p["title"].(string)] = true
		}
		if !titles["Editor draft"] || !titles["Other draft"] || !titles["Admin draft"] || titles["Binned"] || len(r.list) != 7 {
			t.Fatalf("%s must see every draft but no trash: %s", name, r.raw)
		}
	}
}

func TestSpacesTrash(t *testing.T) {
	e := newEnv(t, nil)
	admin, adminC := spUser(t, e, "admin@example.test", false)
	editor, editorC := spUser(t, e, "editor@example.test", false)
	other, otherC := spUser(t, e, "other@example.test", false)
	_, viewerC := spUser(t, e, "viewer@example.test", false)
	_, outsiderC := spUser(t, e, "outsider@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	sp := spCreate(t, adminC, "TRS", "Trash", "internal")
	spaceID := uuid.MustParse(sp["id"].(string))
	adminC.do("PUT", "/api/v1/spaces/TRS/members/"+editor.ID.String(), map[string]any{"role": "editor"})
	adminC.do("PUT", "/api/v1/spaces/TRS/members/"+other.ID.String(), map[string]any{"role": "editor"})
	adminC.do("PUT", "/api/v1/spaces/TRS/members/"+admin.ID.String(), map[string]any{"role": "admin"})

	if r := editorC.do("GET", "/api/v1/spaces/TRS/trash", nil); r.status != 200 || r.list == nil || len(r.list) != 0 {
		t.Fatalf("empty trash must be []: %d %s", r.status, r.raw)
	}
	if r := viewerC.do("GET", "/api/v1/spaces/TRS/trash", nil); r.status != 403 {
		t.Fatalf("viewer trash: %d %s", r.status, r.raw)
	}
	if r := outsiderC.do("GET", "/api/v1/spaces/TRS/trash", nil); r.status != 403 {
		t.Fatalf("internal-space implicit viewer trash: %d %s", r.status, r.raw)
	}
	if r := e.client().do("GET", "/api/v1/spaces/TRS/trash", nil); r.status != 401 {
		t.Fatalf("anon trash: %d", r.status)
	}

	spPage(t, e, spaceID, admin.ID, "live", "published", nil, 0)
	older := spPage(t, e, spaceID, admin.ID, "older", "trashed", nil, 1)
	newer := spPage(t, e, spaceID, admin.ID, "newer", "trashed", nil, 2)
	editorDraft := spPage(t, e, spaceID, editor.ID, "editor never published", "trashed", nil, 3)
	if _, err := e.store.Pool.Exec(context.Background(),
		`UPDATE pages SET trashed_at = now() - interval '1 day' WHERE id = $1`, older); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool.Exec(context.Background(),
		`UPDATE pages SET version = 2, trashed_at = now() - interval '2 days' WHERE id IN ($1, $2)`, older, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool.Exec(context.Background(),
		`UPDATE pages SET trashed_at = now() - interval '3 days' WHERE id = $1`, editorDraft); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool.Exec(context.Background(),
		`UPDATE pages SET trashed_at = now() - interval '1 hour' WHERE id = $1`, newer); err != nil {
		t.Fatal(err)
	}

	r := otherC.do("GET", "/api/v1/spaces/TRS/trash", nil)
	if r.status != 200 || len(r.list) != 2 {
		t.Fatalf("other editor trash (published-then-trashed only): %d %s", r.status, r.raw)
	}
	if r.list[0]["id"] != newer.String() || r.list[1]["id"] != older.String() {
		t.Fatalf("trash order newest first: %s", r.raw)
	}
	first := r.list[0]
	by, _ := first["trashed_by_user"].(map[string]any)
	if first["status"] != "trashed" || first["trashed_at"] == nil || first["trashed_by"] != admin.ID.String() ||
		by["display_name"] != "admin@example.test" || first["slug"] == nil || first["has_children"] != false {
		t.Fatalf("trash item: %v", first)
	}
	if _, err := time.Parse(time.RFC3339Nano, first["trashed_at"].(string)); err != nil {
		t.Fatalf("trashed_at format: %v", err)
	}

	r = editorC.do("GET", "/api/v1/spaces/TRS/trash", nil)
	if r.status != 200 || len(r.list) != 3 || r.list[2]["id"] != editorDraft.String() {
		t.Fatalf("creator sees own never-published trashed page: %d %s", r.status, r.raw)
	}
	for name, c := range map[string]*client{"space admin": adminC, "system admin": rootC} {
		r = c.do("GET", "/api/v1/spaces/TRS/trash", nil)
		if r.status != 200 || len(r.list) != 3 {
			t.Fatalf("%s sees every trashed page: %d %s", name, r.status, r.raw)
		}
	}
}

func TestSpacesAdminList(t *testing.T) {
	e := newEnv(t, nil)
	_, userC := spUser(t, e, "user@example.test", false)
	_, rootC := spUser(t, e, "root@example.test", true)

	if r := e.client().do("GET", "/api/v1/admin/spaces", nil); r.status != 401 {
		t.Fatalf("anon: %d", r.status)
	}
	if r := userC.do("GET", "/api/v1/admin/spaces", nil); r.status != 403 {
		t.Fatalf("non-admin: %d %s", r.status, r.raw)
	}

	spCreate(t, userC, "ZED", "Zed", "private")
	spCreate(t, userC, "ARC", "Arc", "internal")
	spCreate(t, userC, "DEL", "Del", "internal")
	userC.do("POST", "/api/v1/spaces/ARC/archive", nil)
	userC.do("DELETE", "/api/v1/spaces/DEL", nil)

	r := rootC.do("GET", "/api/v1/admin/spaces", nil)
	if r.status != 200 || !spEqual(spKeys(r.list), []string{"ARC", "DEL", "ZED"}) {
		t.Fatalf("admin list: %d %s", r.status, r.raw)
	}
	for _, s := range r.list {
		if s["member_count"] != float64(1) || s["page_count"] != float64(1) {
			t.Fatalf("admin counts: %v", s)
		}
	}
	if r.list[0]["archived_at"] == nil || r.list[1]["deleted_at"] == nil || r.list[2]["deleted_at"] != nil {
		t.Fatalf("admin flags: %s", r.raw)
	}
}
