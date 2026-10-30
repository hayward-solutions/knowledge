package handlers_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/storage"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type pagesObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (o *pagesObjectStore) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.objects[key] = data
	return nil
}

func (o *pagesObjectStore) Get(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.objects[key]
	if !ok {
		return nil, "", 0, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), "application/octet-stream", int64(len(data)), nil
}

func (o *pagesObjectStore) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.objects, key)
	return nil
}

func (o *pagesObjectStore) has(key string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.objects[key]
	return ok
}

type pagesEnv struct {
	*env
	bus     *events.Bus
	objects *pagesObjectStore
}

func newPagesEnv(t *testing.T) *pagesEnv {
	t.Helper()
	s := testutil.NewDB(t)
	bus := events.NewBus()
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	objects := &pagesObjectStore{objects: map[string][]byte{}}
	router := apphttp.NewRouter(handlers.Deps{
		Store:    s,
		Storage:  objects,
		Bus:      bus,
		Hub:      events.NewHub(),
		Presence: presence,
		Config:   &config.Config{AppOrigins: []string{"http://app.test"}},
		Version:  "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &pagesEnv{env: &env{t: t, store: s, server: srv}, bus: bus, objects: objects}
}

type pagesCollector struct {
	mu     sync.Mutex
	events []any
}

func (c *pagesCollector) add(e any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *pagesCollector) all() []any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]any(nil), c.events...)
}

type pagesWorld struct {
	*pagesEnv
	space   uuid.UUID
	home    uuid.UUID
	users   map[string]domain.User
	clients map[string]*client
	events  *pagesCollector
}

func pagesSetup(t *testing.T) *pagesWorld {
	t.Helper()
	e := newPagesEnv(t)
	w := &pagesWorld{
		pagesEnv: e,
		users:    map[string]domain.User{},
		clients:  map[string]*client{},
		events:   &pagesCollector{},
	}
	e.bus.Subscribe(w.events.add)
	for _, name := range []string{"sys", "spaceadmin", "ed1", "ed2", "viewer", "outsider"} {
		w.users[name] = testutil.CreateUser(t, e.store, name+"@example.test", "", name == "sys")
	}
	w.space = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "ENG", Visibility: domain.VisibilityPrivate, CreatedBy: w.users["sys"].ID})
	testutil.AddMember(t, e.store, w.space, w.users["spaceadmin"].ID, domain.RoleAdmin)
	testutil.AddMember(t, e.store, w.space, w.users["ed1"].ID, domain.RoleEditor)
	testutil.AddMember(t, e.store, w.space, w.users["ed2"].ID, domain.RoleEditor)
	testutil.AddMember(t, e.store, w.space, w.users["viewer"].ID, domain.RoleViewer)

	ctx := context.Background()
	home, err := e.store.CreatePage(ctx, store.NewPage{SpaceID: w.space, Title: "Home", CreatedBy: w.users["sys"].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.PublishPage(ctx, store.PublishParams{PageID: home.ID, ActorID: w.users["sys"].ID, Title: "Home"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool.Exec(ctx, `UPDATE spaces SET home_page_id = $2 WHERE id = $1`, w.space, home.ID); err != nil {
		t.Fatal(err)
	}
	w.home = home.ID
	return w
}

func (w *pagesWorld) as(name string) *client {
	w.t.Helper()
	if c, ok := w.clients[name]; ok {
		return c
	}
	c := w.e().client()
	c.mustLogin(name+"@example.test", testutil.DefaultPassword)
	w.clients[name] = c
	return c
}

func (w *pagesWorld) e() *env { return w.env }

func pagesID(m map[string]any) string {
	id, _ := m["id"].(string)
	return id
}

func pagesPath(id string, tail ...string) string {
	return "/api/v1/pages/" + id + strings.Join(tail, "")
}

func (w *pagesWorld) create(name, title string, parent any) map[string]any {
	w.t.Helper()
	body := map[string]any{"title": title}
	if parent != nil {
		body["parent_id"] = parent
	}
	r := w.as(name).do("POST", "/api/v1/spaces/ENG/pages", body)
	if r.status != 201 {
		w.t.Fatalf("create %q: %d %s", title, r.status, r.raw)
	}
	return r.body
}

func (w *pagesWorld) publish(name, id, title string, base int) map[string]any {
	w.t.Helper()
	r := w.as(name).do("POST", pagesPath(id, "/publish"), map[string]any{
		"title": title, "content_json": nil, "content_html": "<p>" + title + "</p>", "base_version": base,
	})
	if r.status != 200 {
		w.t.Fatalf("publish %q: %d %s", title, r.status, r.raw)
	}
	return r.body
}

func (w *pagesWorld) createPublished(name, title string, parent any) map[string]any {
	w.t.Helper()
	p := w.create(name, title, parent)
	return w.publish(name, pagesID(p), title, 0)
}

func (w *pagesWorld) childTitles(name, id string) []string {
	w.t.Helper()
	r := w.as(name).do("GET", pagesPath(id, "/children"), nil)
	if r.status != 200 {
		w.t.Fatalf("children: %d %s", r.status, r.raw)
	}
	out := []string{}
	for i, c := range r.list {
		if int(c["position"].(float64)) != i {
			w.t.Fatalf("positions must be contiguous: %s", r.raw)
		}
		out = append(out, c["title"].(string))
	}
	return out
}

func pagesTitlesEqual(a, b []string) bool {
	return strings.Join(a, "|") == strings.Join(b, "|")
}

func (w *pagesWorld) dbInt(query string, args ...any) int {
	w.t.Helper()
	var n int
	if err := w.store.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

func TestPagesCreate(t *testing.T) {
	w := pagesSetup(t)

	if r := w.e().client().do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x"}); r.status != 401 {
		t.Fatalf("anon: %d", r.status)
	}
	for name, want := range map[string]int{"viewer": 403, "outsider": 404} {
		if r := w.as(name).do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x"}); r.status != want {
			t.Fatalf("%s must not create: %d", name, r.status)
		}
	}

	first := w.create("ed1", "Hello World", nil)
	if first["status"] != "draft" || first["version"].(float64) != 0 || first["slug"] != "hello-world" ||
		first["parent_id"] != w.home.String() || first["position"].(float64) != 0 || first["space_key"] != "ENG" ||
		first["published_at"] != nil || first["created_by"] != w.users["ed1"].ID.String() {
		t.Fatalf("first: %v", first)
	}
	if _, leaked := first["content_text"]; leaked {
		t.Fatalf("content_text must not be serialised: %v", first)
	}
	doc, _ := first["content_json"].(map[string]any)
	if doc["type"] != "doc" {
		t.Fatalf("empty doc: %v", first["content_json"])
	}

	second := w.create("ed2", "Second", nil)
	if second["position"].(float64) != 1 {
		t.Fatalf("second position: %v", second["position"])
	}
	child := w.create("ed1", "Child", pagesID(first))
	if child["parent_id"] != pagesID(first) || child["position"].(float64) != 0 {
		t.Fatalf("child: %v", child)
	}
	if blank := w.create("ed1", "   ", nil); blank["title"] != "Untitled" {
		t.Fatalf("blank title: %v", blank["title"])
	}

	other := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "OPS", CreatedBy: w.users["sys"].ID})
	foreign := testutil.CreatePage(t, w.store, other, w.users["sys"].ID, "Foreign")
	for name, parent := range map[string]string{"cross-space": foreign.String(), "missing": uuid.Must(uuid.NewV7()).String()} {
		if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "parent_id": parent}); r.status != 400 {
			t.Fatalf("%s parent: %d %s", name, r.status, r.raw)
		}
	}
	if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "parent_id": "nope"}); r.status != 400 {
		t.Fatalf("malformed parent: %d", r.status)
	}

	trashedParent := w.createPublished("ed1", "Doomed", nil)
	if r := w.as("ed1").do("POST", pagesPath(pagesID(trashedParent), "/trash"), nil); r.status != 204 {
		t.Fatalf("trash: %d", r.status)
	}
	if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "parent_id": pagesID(trashedParent)}); r.status != 400 {
		t.Fatalf("trashed parent: %d %s", r.status, r.raw)
	}

	if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "template_id": uuid.Must(uuid.NewV7()).String()}); r.status != 404 {
		t.Fatalf("unknown template: %d %s", r.status, r.raw)
	}

	long := strings.Repeat("a", 501)
	if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": long}); r.status != 400 {
		t.Fatalf("long title: %d", r.status)
	}
}

func TestPagesCreateWithoutHomeGoesToRoot(t *testing.T) {
	w := pagesSetup(t)
	bare := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "NOH", CreatedBy: w.users["sys"].ID})
	testutil.AddMember(t, w.store, bare, w.users["ed1"].ID, domain.RoleEditor)
	r := w.as("ed1").do("POST", "/api/v1/spaces/NOH/pages", map[string]any{"title": "Rooted"})
	if r.status != 201 || r.body["parent_id"] != nil || r.body["space_key"] != "NOH" {
		t.Fatalf("root fallback: %d %s", r.status, r.raw)
	}
}

func TestPagesDraftStatusVisibility(t *testing.T) {
	w := pagesSetup(t)
	draft := w.create("ed1", "Secret plans", nil)
	id := pagesID(draft)

	probes := []struct{ method, path string }{
		{"GET", pagesPath(id)},
		{"PATCH", pagesPath(id)},
		{"GET", pagesPath(id, "/draft")},
		{"PUT", pagesPath(id, "/draft")},
		{"POST", pagesPath(id, "/publish")},
		{"POST", pagesPath(id, "/move")},
		{"POST", pagesPath(id, "/trash")},
		{"POST", pagesPath(id, "/copy")},
		{"GET", pagesPath(id, "/children")},
		{"POST", pagesPath(id, "/star")},
		{"POST", pagesPath(id, "/editing")},
		{"GET", pagesPath(id, "/versions")},
	}
	body := map[string]any{"title": "x", "content_json": nil, "content_html": "", "base_version": 0}
	for _, name := range []string{"ed2", "viewer"} {
		for _, p := range probes {
			r := w.as(name).do(p.method, p.path, body)
			if r.status != 404 && r.status != 403 {
				t.Errorf("%s %s %s: want 404/403, got %d %s", name, p.method, p.path, r.status, r.raw)
			}
		}
		if r := w.as(name).do("GET", pagesPath(id), nil); r.status != 404 {
			t.Errorf("%s GET draft page: %d", name, r.status)
		}
	}
	if r := w.as("ed2").do("PUT", pagesPath(id, "/draft"), body); r.status != 404 {
		t.Errorf("ed2 PUT draft: %d", r.status)
	}
	if r := w.as("ed2").do("POST", pagesPath(id, "/publish"), body); r.status != 404 {
		t.Errorf("ed2 publish: %d", r.status)
	}

	for _, name := range []string{"ed1", "spaceadmin", "sys"} {
		r := w.as(name).do("GET", pagesPath(id), nil)
		if r.status != 200 || r.body["title"] != "Secret plans" {
			t.Errorf("%s must see the draft: %d %s", name, r.status, r.raw)
		}
	}

	titles := func(name string) []string {
		r := w.as(name).do("GET", pagesPath(w.home.String()), nil)
		if r.status != 200 {
			t.Fatalf("home as %s: %d", name, r.status)
		}
		out := []string{}
		for _, c := range r.body["children"].([]any) {
			out = append(out, c.(map[string]any)["title"].(string))
		}
		return out
	}
	if got := titles("ed2"); len(got) != 0 {
		t.Fatalf("ed2 children: %v", got)
	}
	if got := titles("ed1"); len(got) != 1 || got[0] != "Secret plans" {
		t.Fatalf("ed1 children: %v", got)
	}
	if got := titles("spaceadmin"); len(got) != 1 {
		t.Fatalf("admin children: %v", got)
	}

	w.publish("ed1", id, "Secret plans", 0)
	if r := w.as("ed2").do("GET", pagesPath(id), nil); r.status != 200 {
		t.Fatalf("published page must be visible: %d", r.status)
	}
	if got := titles("ed2"); len(got) != 1 {
		t.Fatalf("ed2 children after publish: %v", got)
	}
}

func TestPagesGetDetail(t *testing.T) {
	w := pagesSetup(t)
	a := w.createPublished("ed1", "Section", nil)
	b := w.createPublished("ed1", "Topic", pagesID(a))
	c := w.createPublished("ed1", "Leaf", pagesID(b))
	id := pagesID(b)

	r := w.as("ed2").do("GET", pagesPath(id), nil)
	if r.status != 200 {
		t.Fatalf("get: %d %s", r.status, r.raw)
	}
	d := r.body
	for _, key := range []string{"id", "space_id", "space_key", "parent_id", "title", "slug", "position", "status", "version",
		"content_json", "content_html", "created_by", "updated_by", "created_by_user", "updated_by_user", "created_at",
		"updated_at", "published_at", "trashed_at", "trashed_by", "labels", "breadcrumbs", "children", "my_draft",
		"editors", "starred", "watching", "my_role"} {
		if _, ok := d[key]; !ok {
			t.Errorf("missing key %q in %s", key, r.raw)
		}
	}
	if _, leaked := d["content_text"]; leaked {
		t.Error("content_text leaked")
	}
	crumbs := d["breadcrumbs"].([]any)
	if len(crumbs) != 2 || crumbs[0].(map[string]any)["id"] != w.home.String() || crumbs[1].(map[string]any)["id"] != pagesID(a) {
		t.Fatalf("breadcrumbs must be root-first excluding self: %v", crumbs)
	}
	if crumbs[1].(map[string]any)["has_children"] != true {
		t.Fatalf("crumb summary: %v", crumbs[1])
	}
	kids := d["children"].([]any)
	if len(kids) != 1 || kids[0].(map[string]any)["id"] != pagesID(c) || kids[0].(map[string]any)["has_children"] != false {
		t.Fatalf("children: %v", kids)
	}
	if d["labels"] == nil || len(d["labels"].([]any)) != 0 || d["watching"] != false || d["starred"] != false || d["my_draft"] != nil {
		t.Fatalf("defaults: %s", r.raw)
	}
	if d["my_role"] != "editor" {
		t.Fatalf("my_role: %v", d["my_role"])
	}
	creator := d["created_by_user"].(map[string]any)
	if creator["id"] != w.users["ed1"].ID.String() || creator["display_name"] == nil {
		t.Fatalf("created_by_user: %v", creator)
	}
	if d["updated_by_user"].(map[string]any)["id"] != w.users["ed1"].ID.String() {
		t.Fatalf("updated_by_user: %v", d["updated_by_user"])
	}
	if got := w.as("viewer").do("GET", pagesPath(id), nil).body["my_role"]; got != "viewer" {
		t.Fatalf("viewer role: %v", got)
	}
	if got := w.as("sys").do("GET", pagesPath(id), nil).body["my_role"]; got != "admin" {
		t.Fatalf("sys role: %v", got)
	}

	if r := w.as("ed2").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "wip", "content_json": nil, "content_html": "", "base_version": 1}); r.status != 200 {
		t.Fatalf("draft: %d %s", r.status, r.raw)
	}
	if r := w.as("ed2").do("POST", pagesPath(id, "/star"), nil); r.status != 204 {
		t.Fatalf("star: %d", r.status)
	}
	d = w.as("ed2").do("GET", pagesPath(id), nil).body
	draft := d["my_draft"].(map[string]any)
	if draft["base_version"].(float64) != 1 || draft["updated_at"] == nil || d["starred"] != true {
		t.Fatalf("draft/star: %v", d)
	}
	if other := w.as("ed1").do("GET", pagesPath(id), nil).body; other["my_draft"] != nil || other["starred"] != false {
		t.Fatalf("draft and star are per user: %v", other)
	}

	before := w.dbInt(`SELECT count(*) FROM page_views WHERE page_id = $1`, id)
	if before != 4 {
		t.Fatalf("expected 4 distinct viewers (ed2, viewer, sys, ed1): %d", before)
	}
	w.as("ed2").do("GET", pagesPath(id), nil)
	if after := w.dbInt(`SELECT count(*) FROM page_views WHERE page_id = $1`, id); after != before {
		t.Fatalf("view upsert must not add rows: %d", after)
	}

	if r := w.as("outsider").do("GET", pagesPath(id), nil); r.status != 404 {
		t.Fatalf("outsider: %d", r.status)
	}
	if r := w.e().client().do("GET", pagesPath(id), nil); r.status != 401 {
		t.Fatalf("anon: %d", r.status)
	}
	if r := w.as("ed1").do("GET", pagesPath(uuid.Must(uuid.NewV7()).String()), nil); r.status != 404 {
		t.Fatalf("unknown: %d", r.status)
	}
	if r := w.as("ed1").do("GET", "/api/v1/pages/nope", nil); r.status != 400 {
		t.Fatalf("bad id: %d", r.status)
	}
}

func TestPagesPatchTitle(t *testing.T) {
	w := pagesSetup(t)
	p := w.createPublished("ed1", "Old name", nil)
	id := pagesID(p)

	r := w.as("ed2").do("PATCH", pagesPath(id), map[string]any{"title": "  New Name  "})
	if r.status != 200 || r.body["title"] != "New Name" || r.body["slug"] != "new-name" || r.body["version"].(float64) != 1 {
		t.Fatalf("patch: %d %s", r.status, r.raw)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_versions WHERE page_id = $1`, id); n != 1 {
		t.Fatalf("no version for title change: %d", n)
	}
	for _, title := range []string{"", "   "} {
		if r := w.as("ed1").do("PATCH", pagesPath(id), map[string]any{"title": title}); r.status != 400 {
			t.Fatalf("empty title %q: %d", title, r.status)
		}
	}
	if r := w.as("ed1").do("PATCH", pagesPath(id), map[string]any{}); r.status != 200 || r.body["title"] != "New Name" {
		t.Fatalf("no-op patch: %d %s", r.status, r.raw)
	}
	if r := w.as("viewer").do("PATCH", pagesPath(id), map[string]any{"title": "x"}); r.status != 403 {
		t.Fatalf("viewer: %d", r.status)
	}
}

func TestPagesDraftLifecycle(t *testing.T) {
	w := pagesSetup(t)
	p := w.create("ed1", "Doc", nil)
	id := pagesID(p)

	if r := w.as("ed1").do("GET", pagesPath(id, "/draft"), nil); r.status != 404 {
		t.Fatalf("no draft yet: %d", r.status)
	}
	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}}
	r := w.as("ed1").do("PUT", pagesPath(id, "/draft"), map[string]any{
		"title": "Doc v2", "content_json": doc, "content_html": `<p onclick="x()">hi</p><script>alert(1)</script>`, "base_version": 0,
	})
	if r.status != 200 {
		t.Fatalf("put: %d %s", r.status, r.raw)
	}
	if strings.Contains(r.body["content_html"].(string), "script") || strings.Contains(r.body["content_html"].(string), "onclick") {
		t.Fatalf("html must be sanitised: %v", r.body["content_html"])
	}
	for _, key := range []string{"page_id", "user_id", "title", "content_json", "content_html", "base_version", "updated_at"} {
		if _, ok := r.body[key]; !ok {
			t.Errorf("draft missing %q: %s", key, r.raw)
		}
	}
	if r.body["user_id"] != w.users["ed1"].ID.String() || r.body["title"] != "Doc v2" {
		t.Fatalf("draft body: %s", r.raw)
	}

	w.as("ed2")
	if r := w.as("ed1").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "again", "content_json": nil, "content_html": "", "base_version": 0}); r.status != 200 {
		t.Fatalf("second put: %d", r.status)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_drafts WHERE page_id = $1`, id); n != 1 {
		t.Fatalf("one draft row: %d", n)
	}
	got := w.as("ed1").do("GET", pagesPath(id, "/draft"), nil)
	if got.status != 200 || got.body["title"] != "again" || got.body["content_json"] != nil {
		t.Fatalf("get: %d %s", got.status, got.raw)
	}
	if r := w.as("ed1").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "x", "content_json": []any{1}, "content_html": "", "base_version": 0}); r.status != 400 {
		t.Fatalf("array content_json: %d", r.status)
	}

	w.publish("ed1", id, "Doc", 0)
	if n := w.dbInt(`SELECT count(*) FROM page_drafts WHERE page_id = $1`, id); n != 0 {
		t.Fatalf("publish removes the draft: %d", n)
	}
	w.as("ed2").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "mine", "content_json": nil, "content_html": "", "base_version": 1})
	if r := w.as("ed1").do("GET", pagesPath(id, "/draft"), nil); r.status != 404 {
		t.Fatalf("drafts are per user: %d", r.status)
	}
	if r := w.as("ed2").do("DELETE", pagesPath(id, "/draft"), nil); r.status != 204 {
		t.Fatalf("delete: %d", r.status)
	}
	if r := w.as("ed2").do("GET", pagesPath(id, "/draft"), nil); r.status != 404 {
		t.Fatalf("deleted: %d", r.status)
	}
	if r := w.as("ed2").do("DELETE", pagesPath(id, "/draft"), nil); r.status != 204 {
		t.Fatalf("delete again: %d", r.status)
	}
	if r := w.as("viewer").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "x"}); r.status != 403 {
		t.Fatalf("viewer put: %d", r.status)
	}
}

func TestPagesPublishVersionsAndConflicts(t *testing.T) {
	w := pagesSetup(t)
	p := w.create("ed1", "Roadmap", nil)
	id := pagesID(p)
	w.as("ed1").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "wip", "content_json": nil, "content_html": "", "base_version": 0})

	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Quarterly goals"}}}}}
	r := w.as("ed1").do("POST", pagesPath(id, "/publish"), map[string]any{
		"title": "Roadmap 2026", "content_json": doc, "content_html": "<p>Quarterly goals</p><script>x</script>",
		"base_version": 0, "message": "  first cut  ",
	})
	if r.status != 200 {
		t.Fatalf("publish: %d %s", r.status, r.raw)
	}
	pg := r.body
	if pg["version"].(float64) != 1 || pg["status"] != "published" || pg["published_at"] == nil || pg["slug"] != "roadmap-2026" ||
		pg["title"] != "Roadmap 2026" || pg["updated_by"] != w.users["ed1"].ID.String() || strings.Contains(pg["content_html"].(string), "script") {
		t.Fatalf("published page: %s", r.raw)
	}
	var text string
	if err := w.store.Pool.QueryRow(context.Background(), `SELECT content_text FROM pages WHERE id = $1`, id).Scan(&text); err != nil || !strings.Contains(text, "Quarterly goals") {
		t.Fatalf("content_text: %q %v", text, err)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_drafts WHERE page_id = $1`, id); n != 0 {
		t.Fatalf("draft must be deleted: %d", n)
	}
	var msg string
	if err := w.store.Pool.QueryRow(context.Background(), `SELECT message FROM page_versions WHERE page_id = $1 AND version = 1`, id).Scan(&msg); err != nil || msg != "first cut" {
		t.Fatalf("version message: %q %v", msg, err)
	}
	firstPublished := pg["published_at"]

	r = w.as("ed2").do("POST", pagesPath(id, "/publish"), map[string]any{
		"title": "Stale edit", "content_json": nil, "content_html": "<p>x</p>", "base_version": 0,
	})
	if r.status != 409 || r.body["code"] != "version_conflict" || r.body["current_version"].(float64) != 1 ||
		r.body["error"] != "page was updated by someone else" {
		t.Fatalf("conflict: %d %s", r.status, r.raw)
	}
	if n := w.dbInt(`SELECT version FROM pages WHERE id = $1`, id); n != 1 {
		t.Fatalf("conflict must not write: %d", n)
	}

	r = w.as("ed2").do("POST", pagesPath(id, "/publish"), map[string]any{
		"title": "Forced edit", "content_json": nil, "content_html": "<p>x</p>", "base_version": 0, "force": true,
	})
	if r.status != 200 || r.body["version"].(float64) != 2 || r.body["published_at"] != firstPublished {
		t.Fatalf("force: %d %s", r.status, r.raw)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_versions WHERE page_id = $1`, id); n != 2 {
		t.Fatalf("version rows: %d", n)
	}

	for name, body := range map[string]map[string]any{
		"empty title": {"title": "", "content_html": "", "base_version": 2},
		"blank title": {"title": "  ", "content_html": "", "base_version": 2},
		"array json":  {"title": "x", "content_json": []any{}, "content_html": "", "base_version": 2},
	} {
		if r := w.as("ed1").do("POST", pagesPath(id, "/publish"), body); r.status != 400 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}

	empty := w.create("ed1", "Empty page", nil)
	r = w.as("ed1").do("POST", pagesPath(pagesID(empty), "/publish"), map[string]any{
		"title": "Empty page", "content_json": nil, "content_html": "", "base_version": 0,
	})
	if r.status != 200 || r.body["version"].(float64) != 1 || r.body["content_json"] != nil {
		t.Fatalf("null content: %d %s", r.status, r.raw)
	}

	if r := w.as("viewer").do("POST", pagesPath(id, "/publish"), map[string]any{"title": "x", "base_version": 2}); r.status != 403 {
		t.Fatalf("viewer: %d", r.status)
	}
}

func TestPagesPublishEventCarriesMentions(t *testing.T) {
	w := pagesSetup(t)
	p := w.create("ed1", "Mentions", nil)
	id := pagesID(p)
	ed2, viewer := w.users["ed2"].ID.String(), w.users["viewer"].ID.String()
	mention := func(id, label string) map[string]any {
		return map[string]any{"type": "mention", "attrs": map[string]any{"id": id, "label": label}}
	}
	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		mention(ed2, "Ed"), mention(viewer, "Vi"), mention(ed2, "Ed"),
	}}}}
	r := w.as("ed1").do("POST", pagesPath(id, "/publish"), map[string]any{
		"title": "Mentions", "content_json": doc, "content_html": "", "base_version": 0,
	})
	if r.status != 200 {
		t.Fatalf("publish: %d %s", r.status, r.raw)
	}
	w.bus.Wait()

	var published []domain.PagePublished
	for _, e := range w.events.all() {
		if ev, ok := e.(domain.PagePublished); ok && ev.PageID.String() == id {
			published = append(published, ev)
		}
	}
	if len(published) != 1 {
		t.Fatalf("expected exactly one PagePublished: %+v", w.events.all())
	}
	ev := published[0]
	if ev.SpaceID != w.space || ev.ActorID != w.users["ed1"].ID || ev.Version != 1 || len(ev.MentionedUserIDs) != 2 {
		t.Fatalf("event: %+v", ev)
	}
	seen := map[string]bool{}
	for _, m := range ev.MentionedUserIDs {
		seen[m.String()] = true
	}
	if !seen[ed2] || !seen[viewer] {
		t.Fatalf("mentions: %+v", ev.MentionedUserIDs)
	}

	before := len(w.events.all())
	w.as("ed2").do("POST", pagesPath(id, "/publish"), map[string]any{"title": "x", "content_html": "", "base_version": 0})
	w.bus.Wait()
	if len(w.events.all()) != before {
		t.Fatal("a rejected publish must not emit an event")
	}
}

func TestPagesMove(t *testing.T) {
	w := pagesSetup(t)
	a := w.createPublished("ed1", "A", nil)
	b := w.createPublished("ed1", "B", nil)
	c := w.createPublished("ed1", "C", nil)
	home := w.home.String()

	if got := w.childTitles("ed1", home); !pagesTitlesEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("initial: %v", got)
	}
	r := w.as("ed1").do("POST", pagesPath(pagesID(c), "/move"), map[string]any{"parent_id": home, "position": 0})
	if r.status != 200 || r.body["position"].(float64) != 0 || r.body["parent_id"] != home {
		t.Fatalf("move to front: %d %s", r.status, r.raw)
	}
	if got := w.childTitles("ed1", home); !pagesTitlesEqual(got, []string{"C", "A", "B"}) {
		t.Fatalf("after position: %v", got)
	}
	if r := w.as("ed1").do("POST", pagesPath(pagesID(c), "/move"), map[string]any{"parent_id": home, "after_id": pagesID(b)}); r.status != 200 {
		t.Fatalf("after_id: %d %s", r.status, r.raw)
	}
	if got := w.childTitles("ed1", home); !pagesTitlesEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("after after_id: %v", got)
	}

	if r := w.as("ed1").do("POST", pagesPath(pagesID(b), "/move"), map[string]any{"parent_id": pagesID(a)}); r.status != 200 {
		t.Fatalf("reparent: %d %s", r.status, r.raw)
	}
	if got := w.childTitles("ed1", home); !pagesTitlesEqual(got, []string{"A", "C"}) {
		t.Fatalf("old siblings renumbered: %v", got)
	}
	if got := w.childTitles("ed1", pagesID(a)); !pagesTitlesEqual(got, []string{"B"}) {
		t.Fatalf("new children: %v", got)
	}

	grand := w.createPublished("ed1", "Grand", pagesID(b))
	for name, parent := range map[string]string{"descendant": pagesID(grand), "child": pagesID(b), "self": pagesID(a)} {
		if r := w.as("ed1").do("POST", pagesPath(pagesID(a), "/move"), map[string]any{"parent_id": parent}); r.status != 400 {
			t.Errorf("cycle via %s: %d %s", name, r.status, r.raw)
		}
	}

	other := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "OPS", CreatedBy: w.users["sys"].ID})
	foreign := testutil.CreatePage(t, w.store, other, w.users["sys"].ID, "Foreign")
	if r := w.as("ed1").do("POST", pagesPath(pagesID(a), "/move"), map[string]any{"parent_id": foreign.String()}); r.status != 400 {
		t.Fatalf("cross-space parent: %d %s", r.status, r.raw)
	}
	if r := w.as("ed1").do("POST", pagesPath(pagesID(a), "/move"), map[string]any{"parent_id": home, "after_id": foreign.String()}); r.status != 400 {
		t.Fatalf("bad anchor: %d %s", r.status, r.raw)
	}

	if r := w.as("ed1").do("POST", pagesPath(pagesID(grand), "/move"), map[string]any{"parent_id": nil}); r.status != 200 || r.body["parent_id"] != nil {
		t.Fatalf("to root: %d %s", r.status, r.raw)
	}
	if r := w.as("viewer").do("POST", pagesPath(pagesID(a), "/move"), map[string]any{"parent_id": nil}); r.status != 403 {
		t.Fatalf("viewer: %d", r.status)
	}

	w.bus.Wait()
	var moved []domain.PageMoved
	for _, e := range w.events.all() {
		if ev, ok := e.(domain.PageMoved); ok {
			moved = append(moved, ev)
		}
	}
	if len(moved) != 4 {
		t.Fatalf("PageMoved events: %d", len(moved))
	}
	var reparent *domain.PageMoved
	for i := range moved {
		if moved[i].PageID.String() == pagesID(b) && moved[i].NewParentID != nil && moved[i].NewParentID.String() == pagesID(a) {
			reparent = &moved[i]
		}
	}
	if reparent == nil || reparent.OldParentID == nil || reparent.OldParentID.String() != home || reparent.SpaceID != w.space || reparent.ActorID != w.users["ed1"].ID {
		t.Fatalf("reparent event: %+v", moved)
	}
}

func TestPagesTrashRestorePurge(t *testing.T) {
	w := pagesSetup(t)
	parent := w.createPublished("ed1", "Parent", nil)
	child := w.createPublished("ed1", "Child", pagesID(parent))
	grand := w.create("ed1", "Grand", pagesID(child))
	pid, cid, gid := pagesID(parent), pagesID(child), pagesID(grand)

	if r := w.as("ed1").do("POST", pagesPath(w.home.String(), "/trash"), nil); r.status != 400 {
		t.Fatalf("home page: %d %s", r.status, r.raw)
	}
	if r := w.as("viewer").do("POST", pagesPath(pid, "/trash"), nil); r.status != 403 {
		t.Fatalf("viewer: %d", r.status)
	}
	if r := w.as("ed1").do("POST", pagesPath(pid, "/trash"), nil); r.status != 204 {
		t.Fatalf("trash: %d %s", r.status, r.raw)
	}
	if r := w.as("ed1").do("POST", pagesPath(pid, "/trash"), nil); r.status != 409 {
		t.Fatalf("trash twice: %d", r.status)
	}

	var stamps = map[string]string{}
	for _, id := range []string{pid, cid, gid} {
		r := w.as("sys").do("GET", pagesPath(id), nil)
		if r.status != 200 || r.body["status"] != "trashed" || r.body["trashed_at"] == nil || r.body["trashed_by"] != w.users["ed1"].ID.String() {
			t.Fatalf("%s must be trashed: %d %s", id, r.status, r.raw)
		}
		stamps[id] = r.body["trashed_at"].(string)
	}
	if stamps[pid] != stamps[cid] || stamps[cid] != stamps[gid] {
		t.Fatalf("shared trashed_at: %v", stamps)
	}
	for _, p := range []string{pagesPath(pid), pagesPath(pid, "/children"), pagesPath(pid, "/versions"), pagesPath(pid, "/versions/1")} {
		if r := w.as("viewer").do("GET", p, nil); r.status != 404 {
			t.Fatalf("viewer must not read trashed page via %s: %d %s", p, r.status, r.raw)
		}
		if r := w.as("ed2").do("GET", p, nil); r.status != 200 {
			t.Fatalf("editor reads trashed page via %s: %d %s", p, r.status, r.raw)
		}
	}
	if r := w.as("viewer").do("POST", pagesPath(pid, "/star"), nil); r.status != 404 {
		t.Fatalf("viewer star trashed: %d", r.status)
	}
	if got := w.childTitles("ed1", w.home.String()); len(got) != 0 {
		t.Fatalf("trashed pages leave the tree: %v", got)
	}

	body := map[string]any{"title": "x", "content_json": nil, "content_html": "", "base_version": 1}
	for _, p := range []struct{ method, path string }{
		{"PATCH", pagesPath(pid)},
		{"PUT", pagesPath(pid, "/draft")},
		{"POST", pagesPath(pid, "/publish")},
		{"POST", pagesPath(pid, "/move")},
		{"POST", pagesPath(pid, "/copy")},
		{"POST", pagesPath(pid, "/editing")},
		{"POST", pagesPath(pid, "/versions/1/restore")},
	} {
		r := w.as("ed1").do(p.method, p.path, body)
		if r.status != 409 || r.body["code"] != "page_trashed" {
			t.Errorf("%s %s on trashed page: %d %s", p.method, p.path, r.status, r.raw)
		}
	}

	r := w.as("ed2").do("POST", pagesPath(cid, "/restore"), nil)
	if r.status != 200 || r.body["status"] != "published" || r.body["parent_id"] != w.home.String() || r.body["trashed_at"] != nil || r.body["space_key"] != "ENG" {
		t.Fatalf("restore child while parent trashed: %d %s", r.status, r.raw)
	}
	if g := w.as("sys").do("GET", pagesPath(gid), nil).body; g["status"] != "draft" || g["trashed_at"] != nil {
		t.Fatalf("grandchild never published restores as draft: %v", g)
	}
	if p := w.as("sys").do("GET", pagesPath(pid), nil).body; p["status"] != "trashed" {
		t.Fatalf("parent stays trashed: %v", p)
	}
	if r := w.as("ed2").do("POST", pagesPath(cid, "/restore"), nil); r.status != 400 {
		t.Fatalf("restore live page: %d", r.status)
	}

	r = w.as("ed1").do("POST", pagesPath(pid, "/restore"), nil)
	if r.status != 200 || r.body["status"] != "published" || r.body["parent_id"] != w.home.String() {
		t.Fatalf("restore parent: %d %s", r.status, r.raw)
	}

	w.as("ed1").do("POST", pagesPath(pid, "/trash"), nil)
	if r := w.as("ed1").do("DELETE", pagesPath(pid), nil); r.status != 403 {
		t.Fatalf("editor purge: %d", r.status)
	}
	if r := w.as("spaceadmin").do("DELETE", pagesPath(cid), nil); r.status != 400 {
		t.Fatalf("purge of live page: %d %s", r.status, r.raw)
	}
	if r := w.as("spaceadmin").do("DELETE", pagesPath(pid), nil); r.status != 204 {
		t.Fatalf("admin purge: %d %s", r.status, r.raw)
	}
	if r := w.as("spaceadmin").do("GET", pagesPath(pid), nil); r.status != 404 {
		t.Fatalf("purged: %d", r.status)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_versions WHERE page_id = $1`, pid); n != 0 {
		t.Fatalf("versions cascade: %d", n)
	}

	w.as("ed1").do("POST", pagesPath(cid, "/trash"), nil)
	if r := w.as("sys").do("DELETE", pagesPath(cid), nil); r.status != 204 {
		t.Fatalf("system admin purge: %d", r.status)
	}

	w.bus.Wait()
	counts := map[string]int{}
	for _, e := range w.events.all() {
		switch ev := e.(type) {
		case domain.PageTrashed:
			counts["trashed"]++
			if ev.SpaceID != w.space || ev.ActorID != w.users["ed1"].ID {
				t.Errorf("trashed event: %+v", ev)
			}
		case domain.PageRestored:
			counts["restored"]++
		}
	}
	if counts["trashed"] != 3 || counts["restored"] != 2 {
		t.Fatalf("event counts: %v", counts)
	}
}

func TestPagesRestoreWithoutHomeGoesToRoot(t *testing.T) {
	w := pagesSetup(t)
	bare := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "NOH", CreatedBy: w.users["sys"].ID})
	testutil.AddMember(t, w.store, bare, w.users["ed1"].ID, domain.RoleEditor)
	c := w.as("ed1")
	parent := c.do("POST", "/api/v1/spaces/NOH/pages", map[string]any{"title": "P"}).body
	child := c.do("POST", "/api/v1/spaces/NOH/pages", map[string]any{"title": "C", "parent_id": pagesID(parent)}).body
	if r := c.do("POST", pagesPath(pagesID(parent), "/trash"), nil); r.status != 204 {
		t.Fatalf("trash: %d", r.status)
	}
	r := c.do("POST", pagesPath(pagesID(child), "/restore"), nil)
	if r.status != 200 || r.body["parent_id"] != nil {
		t.Fatalf("root fallback: %d %s", r.status, r.raw)
	}
}

func TestPagesCopy(t *testing.T) {
	w := pagesSetup(t)
	section := w.createPublished("ed1", "Section", nil)
	src := w.create("ed1", "Guide", pagesID(section))
	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "body"}}}}}
	r := w.as("ed1").do("POST", pagesPath(pagesID(src), "/publish"), map[string]any{
		"title": "Guide", "content_json": doc, "content_html": "<p>body</p>", "base_version": 0,
	})
	if r.status != 200 {
		t.Fatalf("publish: %d", r.status)
	}

	r = w.as("ed2").do("POST", pagesPath(pagesID(src), "/copy"), map[string]any{})
	if r.status != 201 {
		t.Fatalf("copy: %d %s", r.status, r.raw)
	}
	cp := r.body
	if cp["id"] == pagesID(src) || cp["title"] != "Copy of Guide" || cp["status"] != "draft" || cp["version"].(float64) != 0 ||
		cp["parent_id"] != pagesID(section) || cp["content_html"] != "<p>body</p>" || cp["created_by"] != w.users["ed2"].ID.String() ||
		cp["slug"] != "copy-of-guide" || cp["position"].(float64) != 1 {
		t.Fatalf("copy: %s", r.raw)
	}
	if cp["content_json"].(map[string]any)["type"] != "doc" {
		t.Fatalf("content_json copied: %v", cp["content_json"])
	}
	if r := w.as("ed1").do("GET", pagesPath(pagesID(cp)), nil); r.status != 404 {
		t.Fatalf("copy is a draft only its creator sees: %d", r.status)
	}

	r = w.as("ed1").do("POST", pagesPath(pagesID(src), "/copy"), map[string]any{"title": "Fresh name", "parent_id": w.home.String()})
	if r.status != 201 || r.body["title"] != "Fresh name" || r.body["parent_id"] != w.home.String() {
		t.Fatalf("custom copy: %d %s", r.status, r.raw)
	}
	if r := w.as("ed1").do("POST", pagesPath(pagesID(src), "/copy"), nil); r.status != 201 {
		t.Fatalf("copy without body: %d %s", r.status, r.raw)
	}
	if r := w.as("ed1").do("POST", pagesPath(pagesID(src), "/copy"), map[string]any{"parent_id": uuid.Must(uuid.NewV7()).String()}); r.status != 400 {
		t.Fatalf("bad parent: %d", r.status)
	}
	if r := w.as("viewer").do("POST", pagesPath(pagesID(src), "/copy"), map[string]any{}); r.status != 403 {
		t.Fatalf("viewer: %d", r.status)
	}
	if n := w.dbInt(`SELECT version FROM pages WHERE id = $1`, pagesID(src)); n != 1 {
		t.Fatalf("source untouched: %d", n)
	}
}

func TestPagesStarAndChildren(t *testing.T) {
	w := pagesSetup(t)
	parent := w.createPublished("ed1", "Parent", nil)
	w.createPublished("ed1", "One", pagesID(parent))
	w.createPublished("ed1", "Two", pagesID(parent))
	id := pagesID(parent)

	r := w.as("viewer").do("GET", pagesPath(id, "/children"), nil)
	if r.status != 200 || len(r.list) != 2 || r.list[0]["title"] != "One" {
		t.Fatalf("children: %d %s", r.status, r.raw)
	}
	for _, key := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children"} {
		if _, ok := r.list[0][key]; !ok {
			t.Errorf("summary missing %q", key)
		}
	}
	if r := w.as("viewer").do("GET", pagesPath(r.list[0]["id"].(string), "/children"), nil); r.status != 200 || string(r.raw) != "[]\n" {
		t.Fatalf("leaf children: %d %q", r.status, r.raw)
	}

	if r := w.as("viewer").do("POST", pagesPath(id, "/star"), nil); r.status != 204 {
		t.Fatalf("viewer star: %d", r.status)
	}
	if r := w.as("viewer").do("POST", pagesPath(id, "/star"), nil); r.status != 204 {
		t.Fatalf("star twice: %d", r.status)
	}
	if got := w.as("viewer").do("GET", pagesPath(id), nil).body["starred"]; got != true {
		t.Fatalf("starred: %v", got)
	}
	if got := w.as("ed1").do("GET", pagesPath(id), nil).body["starred"]; got != false {
		t.Fatalf("other user: %v", got)
	}
	if r := w.as("viewer").do("DELETE", pagesPath(id, "/star"), nil); r.status != 204 {
		t.Fatalf("unstar: %d", r.status)
	}
	if got := w.as("viewer").do("GET", pagesPath(id), nil).body["starred"]; got != false {
		t.Fatalf("unstarred: %v", got)
	}
	if r := w.as("outsider").do("POST", pagesPath(id, "/star"), nil); r.status != 404 {
		t.Fatalf("outsider star: %d", r.status)
	}
}

func TestPagesPresenceExcludesCaller(t *testing.T) {
	w := pagesSetup(t)
	id := pagesID(w.createPublished("ed1", "Shared", nil))

	r := w.as("ed1").do("POST", pagesPath(id, "/editing"), nil)
	if r.status != 200 || string(r.raw) != "{\"editors\":[]}\n" {
		t.Fatalf("first heartbeat: %d %q", r.status, r.raw)
	}
	r = w.as("ed2").do("POST", pagesPath(id, "/editing"), nil)
	editors, _ := r.body["editors"].([]any)
	if r.status != 200 || len(editors) != 1 || editors[0].(map[string]any)["id"] != w.users["ed1"].ID.String() {
		t.Fatalf("ed2 sees ed1 only: %d %s", r.status, r.raw)
	}
	for _, key := range []string{"id", "display_name", "email"} {
		if _, ok := editors[0].(map[string]any)[key]; !ok {
			t.Errorf("editor ref missing %q", key)
		}
	}
	detail := w.as("ed1").do("GET", pagesPath(id), nil).body["editors"].([]any)
	if len(detail) != 1 || detail[0].(map[string]any)["id"] != w.users["ed2"].ID.String() {
		t.Fatalf("detail editors exclude the caller: %v", detail)
	}
	if got := w.as("viewer").do("GET", pagesPath(id), nil).body["editors"].([]any); len(got) != 2 {
		t.Fatalf("viewer sees both editors: %v", got)
	}
	if r := w.as("viewer").do("POST", pagesPath(id, "/editing"), nil); r.status != 403 {
		t.Fatalf("viewer heartbeat: %d", r.status)
	}
}

func TestPagesArchivedSpaceIsReadOnly(t *testing.T) {
	w := pagesSetup(t)
	id := pagesID(w.createPublished("ed1", "Page", nil))
	if _, err := w.store.Pool.Exec(context.Background(), `UPDATE spaces SET archived_at = now() WHERE id = $1`, w.space); err != nil {
		t.Fatal(err)
	}
	if r := w.as("ed1").do("GET", pagesPath(id), nil); r.status != 200 {
		t.Fatalf("view: %d", r.status)
	}
	if r := w.as("ed1").do("PATCH", pagesPath(id), map[string]any{"title": "x"}); r.status != 403 {
		t.Fatalf("edit: %d", r.status)
	}
	if r := w.as("ed1").do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x"}); r.status != 403 {
		t.Fatalf("create: %d", r.status)
	}
}

func TestPagesBreadcrumbsSkipHiddenDrafts(t *testing.T) {
	w := pagesSetup(t)
	hidden := w.create("ed1", "Hidden parent", nil)
	child := w.createPublished("ed1", "Visible child", pagesID(hidden))

	crumbIDs := func(name string) []string {
		r := w.as(name).do("GET", pagesPath(pagesID(child)), nil)
		if r.status != 200 {
			t.Fatalf("%s get: %d %s", name, r.status, r.raw)
		}
		out := []string{}
		for _, c := range r.body["breadcrumbs"].([]any) {
			out = append(out, c.(map[string]any)["id"].(string))
		}
		return out
	}
	home := w.home.String()
	if got := crumbIDs("ed2"); len(got) != 1 || got[0] != home {
		t.Fatalf("ed2 must not see the hidden draft ancestor: %v", got)
	}
	for _, name := range []string{"ed1", "spaceadmin", "sys"} {
		if got := crumbIDs(name); len(got) != 2 || got[0] != home || got[1] != pagesID(hidden) {
			t.Fatalf("%s sees the full chain: %v", name, got)
		}
	}
}

func TestPagesDetailLabels(t *testing.T) {
	w := pagesSetup(t)
	id := pagesID(w.createPublished("ed1", "Labelled", nil))
	pid, _ := uuid.Parse(id)
	if _, err := w.store.SetPageLabels(context.Background(), pid, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	labels := w.as("viewer").do("GET", pagesPath(id), nil).body["labels"].([]any)
	names := []string{}
	for _, l := range labels {
		names = append(names, l.(map[string]any)["name"].(string))
	}
	if len(names) != 2 || !(names[0] == "alpha" && names[1] == "beta") {
		t.Fatalf("labels: %v", names)
	}
}

func TestPagesPurgeRemovesAttachmentObjects(t *testing.T) {
	w := pagesSetup(t)
	parent := w.createPublished("ed1", "Parent", nil)
	child := w.createPublished("ed1", "Child", pagesID(parent))
	other := w.createPublished("ed1", "Other", nil)
	ctx := context.Background()

	add := func(page map[string]any, key string) {
		pid, _ := uuid.Parse(pagesID(page))
		a := domain.Attachment{
			ID: uuid.Must(uuid.NewV7()), PageID: pid, SpaceID: w.space, UploaderID: w.users["ed1"].ID,
			Filename: "f.txt", ContentType: "text/plain", Size: 1, S3Key: key,
		}
		if err := w.store.CreateAttachment(ctx, &a); err != nil {
			t.Fatal(err)
		}
		if err := w.objects.Put(ctx, key, "text/plain", strings.NewReader("x"), 1); err != nil {
			t.Fatal(err)
		}
	}
	add(parent, "k/parent")
	add(child, "k/child")
	add(other, "k/other")

	if r := w.as("spaceadmin").do("DELETE", pagesPath(pagesID(parent)), nil); r.status != 400 {
		t.Fatalf("purge of live page: %d", r.status)
	}
	if !w.objects.has("k/parent") || !w.objects.has("k/child") {
		t.Fatal("a rejected purge must not delete objects")
	}
	w.as("ed1").do("POST", pagesPath(pagesID(parent), "/trash"), nil)
	if r := w.as("spaceadmin").do("DELETE", pagesPath(pagesID(parent)), nil); r.status != 204 {
		t.Fatalf("purge: %d %s", r.status, r.raw)
	}
	if w.objects.has("k/parent") || w.objects.has("k/child") {
		t.Fatal("purge must delete the objects of the page and its descendants")
	}
	if !w.objects.has("k/other") {
		t.Fatal("unrelated objects must survive")
	}
}
