package handlers_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type templateWorld struct {
	e        *env
	admin    domain.User
	editor   domain.User
	viewer   domain.User
	outsider domain.User
	eng      uuid.UUID
	ops      uuid.UUID
	priv     uuid.UUID
}

func newTemplateWorld(t *testing.T) *templateWorld {
	t.Helper()
	e := newEnv(t, nil)
	w := &templateWorld{e: e}
	w.admin = testutil.CreateUser(t, e.store, "admin@example.test", "", true)
	w.editor = testutil.CreateUser(t, e.store, "editor@example.test", "", false)
	w.viewer = testutil.CreateUser(t, e.store, "viewer@example.test", "", false)
	w.outsider = testutil.CreateUser(t, e.store, "outsider@example.test", "", false)
	w.eng = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "ENG", CreatedBy: w.admin.ID})
	w.ops = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "OPS", CreatedBy: w.admin.ID})
	w.priv = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "PRV", Visibility: domain.VisibilityPrivate, CreatedBy: w.admin.ID})
	testutil.AddMember(t, e.store, w.eng, w.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, e.store, w.eng, w.viewer.ID, domain.RoleViewer)
	testutil.AddMember(t, e.store, w.ops, w.editor.ID, domain.RoleViewer)
	testutil.AddMember(t, e.store, w.priv, w.editor.ID, domain.RoleAdmin)
	return w
}

func (w *templateWorld) as(t *testing.T, u domain.User) *client {
	t.Helper()
	c := w.e.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func templateTestInput(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": "desc",
		"content_json": map[string]any{
			"type":    "doc",
			"content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "hello body"}}}},
		},
		"content_html": "<p>hello body</p>",
	}
}

func templateTestNames(t *testing.T, r response) []string {
	t.Helper()
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, r.raw)
	}
	out := []string{}
	for _, item := range r.list {
		out = append(out, item["name"].(string))
	}
	return out
}

func templateTestJoin(names []string) string { return strings.Join(names, "|") }

func TestTemplatesStartersSeedAndList(t *testing.T) {
	w := newTemplateWorld(t)

	user := w.as(t, w.outsider)
	if got := templateTestNames(t, user.do("GET", "/api/v1/templates", nil)); len(got) != 0 {
		t.Fatalf("non-admin must not trigger seeding, got %v", got)
	}

	admin := w.as(t, w.admin)
	r := admin.do("GET", "/api/v1/templates", nil)
	got := templateTestNames(t, r)
	want := "Decision record|How-to guide|Meeting notes|Retrospective"
	if templateTestJoin(got) != want {
		t.Fatalf("starters %v", got)
	}
	for _, item := range r.list {
		if item["space_id"] != nil || item["space_key"] != nil {
			t.Fatalf("starter should be global: %v", item)
		}
		doc, ok := item["content_json"].(map[string]any)
		if !ok || doc["type"] != "doc" {
			t.Fatalf("starter content_json: %v", item["content_json"])
		}
		html := item["content_html"].(string)
		if html == "" || content.SanitizeHTML(html) != html {
			t.Fatalf("starter html not stable under sanitiser: %q", html)
		}
		if item["created_by"] != w.admin.ID.String() {
			t.Fatalf("created_by %v", item["created_by"])
		}
	}

	again := templateTestNames(t, admin.do("GET", "/api/v1/templates", nil))
	if templateTestJoin(again) != want {
		t.Fatalf("seeding must be idempotent, got %v", again)
	}
	if got := templateTestNames(t, user.do("GET", "/api/v1/templates", nil)); templateTestJoin(got) != want {
		t.Fatalf("non-admin sees seeded globals, got %v", got)
	}

	unauth := w.e.client().do("GET", "/api/v1/templates", nil)
	if unauth.status != 401 {
		t.Fatalf("unauthenticated list: %d", unauth.status)
	}
}

func TestTemplatesStarterContentFeatures(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	r := admin.do("GET", "/api/v1/templates", nil)
	var sb strings.Builder
	for _, item := range r.list {
		sb.WriteString(item["content_html"].(string))
	}
	all := sb.String()
	for _, frag := range []string{`data-type="panel"`, `data-type="taskList"`, `<table>`, `<h2>`, `data-type="toc"`} {
		if !strings.Contains(all, frag) {
			t.Errorf("starters missing %s", frag)
		}
	}
}

func TestTemplatesListScopedToEditableSpaces(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	for _, p := range []struct{ path, name string }{
		{"/api/v1/spaces/ENG/templates", "eng tpl"},
		{"/api/v1/spaces/OPS/templates", "ops tpl"},
		{"/api/v1/spaces/PRV/templates", "prv tpl"},
		{"/api/v1/admin/templates", "Zz global"},
	} {
		if r := admin.do("POST", p.path, templateTestInput(p.name)); r.status != 201 {
			t.Fatalf("create %s: %d %s", p.name, r.status, r.raw)
		}
	}
	filter := func(names []string) string {
		out := []string{}
		for _, n := range names {
			switch n {
			case "eng tpl", "ops tpl", "prv tpl", "Zz global":
				out = append(out, n)
			}
		}
		return templateTestJoin(out)
	}
	editorList := templateTestNames(t, w.as(t, w.editor).do("GET", "/api/v1/templates", nil))
	if got := filter(editorList); got != "Zz global|eng tpl|prv tpl" {
		t.Fatalf("editor list %v", editorList)
	}
	if editorList[len(editorList)-1] != "prv tpl" {
		t.Fatalf("space templates sort after globals: %v", editorList)
	}
	viewerList := templateTestNames(t, w.as(t, w.viewer).do("GET", "/api/v1/templates", nil))
	if got := filter(viewerList); got != "Zz global" {
		t.Fatalf("viewer list %v", viewerList)
	}
	adminList := templateTestNames(t, admin.do("GET", "/api/v1/templates", nil))
	if got := filter(adminList); got != "Zz global|eng tpl|ops tpl|prv tpl" {
		t.Fatalf("admin list %v", adminList)
	}
}

func TestTemplatesSpaceCRUDByEditor(t *testing.T) {
	w := newTemplateWorld(t)
	editor := w.as(t, w.editor)

	created := editor.do("POST", "/api/v1/spaces/eng/templates", templateTestInput("Runbook"))
	if created.status != 201 {
		t.Fatalf("create: %d %s", created.status, created.raw)
	}
	id := created.body["id"].(string)
	if created.body["space_key"] != "ENG" || created.body["space_id"] != w.eng.String() ||
		created.body["name"] != "Runbook" || created.body["description"] != "desc" ||
		created.body["content_html"] != "<p>hello body</p>" || created.body["created_by"] != w.editor.ID.String() {
		t.Fatalf("created %v", created.body)
	}
	if doc, ok := created.body["content_json"].(map[string]any); !ok || doc["type"] != "doc" {
		t.Fatalf("content_json %v", created.body["content_json"])
	}

	list := editor.do("GET", "/api/v1/spaces/ENG/templates", nil)
	if got := templateTestNames(t, list); templateTestJoin(got) != "Runbook" {
		t.Fatalf("space list %v", got)
	}
	viewerList := w.as(t, w.viewer).do("GET", "/api/v1/spaces/ENG/templates", nil)
	if got := templateTestNames(t, viewerList); templateTestJoin(got) != "Runbook" {
		t.Fatalf("viewer may list a space's templates, got %v", got)
	}

	got := editor.do("GET", "/api/v1/templates/"+id, nil)
	if got.status != 200 || got.body["name"] != "Runbook" || got.body["space_key"] != "ENG" {
		t.Fatalf("get: %d %s", got.status, got.raw)
	}

	patched := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "Runbook v2", "content_html": "<p>new</p><script>alert(1)</script>"})
	if patched.status != 200 {
		t.Fatalf("patch: %d %s", patched.status, patched.raw)
	}
	if patched.body["name"] != "Runbook v2" || patched.body["content_html"] != "<p>new</p>" || patched.body["description"] != "desc" {
		t.Fatalf("patched %v", patched.body)
	}
	if doc, ok := patched.body["content_json"].(map[string]any); !ok || doc["type"] != "doc" {
		t.Fatalf("content_json must be untouched: %v", patched.body["content_json"])
	}

	cleared := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"content_json": nil})
	if cleared.status != 200 || cleared.body["content_json"] != nil {
		t.Fatalf("clear content_json: %d %s", cleared.status, cleared.raw)
	}
	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"content_json": []any{1}}); r.status != 400 {
		t.Fatalf("array content_json: %d", r.status)
	}
	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "  "}); r.status != 400 {
		t.Fatalf("blank name: %d", r.status)
	}

	if r := editor.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 204 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}
	if r := editor.do("GET", "/api/v1/templates/"+id, nil); r.status != 404 {
		t.Fatalf("get after delete: %d", r.status)
	}
	if r := editor.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 404 {
		t.Fatalf("second delete: %d", r.status)
	}
	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "x"}); r.status != 404 {
		t.Fatalf("patch after delete: %d", r.status)
	}
}

func TestTemplatesCreateValidation(t *testing.T) {
	w := newTemplateWorld(t)
	editor := w.as(t, w.editor)
	long := strings.Repeat("a", 121)
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing name", map[string]any{"description": "x"}, 400},
		{"blank name", map[string]any{"name": "   "}, 400},
		{"too long", map[string]any{"name": long}, 400},
		{"max length", map[string]any{"name": strings.Repeat("a", 120)}, 201},
		{"minimal", map[string]any{"name": "x"}, 201},
		{"bad json", map[string]any{"name": "y", "content_json": "nope"}, 400},
	}
	for _, c := range cases {
		if r := editor.do("POST", "/api/v1/spaces/ENG/templates", c.body); r.status != c.want {
			t.Errorf("%s: got %d want %d (%s)", c.name, r.status, c.want, r.raw)
		}
	}
	r := editor.do("POST", "/api/v1/spaces/ENG/templates", map[string]any{"name": "nulljson", "content_json": nil})
	if r.status != 201 || r.body["content_json"] != nil || r.body["content_html"] != "" || r.body["description"] != "" {
		t.Fatalf("defaults: %d %s", r.status, r.raw)
	}
}

func TestTemplatesViewerAndOutsiderCannotCreate(t *testing.T) {
	w := newTemplateWorld(t)
	viewer := w.as(t, w.viewer)
	if r := viewer.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("nope")); r.status != 403 {
		t.Fatalf("viewer create: %d %s", r.status, r.raw)
	}
	outsider := w.as(t, w.outsider)
	if r := outsider.do("POST", "/api/v1/spaces/PRV/templates", templateTestInput("nope")); r.status != 404 {
		t.Fatalf("outsider on private space: %d", r.status)
	}
	if r := outsider.do("GET", "/api/v1/spaces/PRV/templates", nil); r.status != 404 {
		t.Fatalf("outsider list private space: %d", r.status)
	}
	if r := outsider.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("nope")); r.status != 403 {
		t.Fatalf("outsider on internal space: %d", r.status)
	}
	if r := w.e.client().do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("nope")); r.status != 401 {
		t.Fatalf("anonymous: %d", r.status)
	}

	admin := w.as(t, w.admin)
	created := admin.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("kept"))
	id := created.body["id"].(string)
	if r := viewer.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "x"}); r.status != 403 {
		t.Fatalf("viewer patch: %d", r.status)
	}
	if r := viewer.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 403 {
		t.Fatalf("viewer delete: %d", r.status)
	}
	if r := viewer.do("GET", "/api/v1/templates/"+id, nil); r.status != 200 {
		t.Fatalf("viewer may read: %d", r.status)
	}

	priv := w.as(t, w.editor).do("POST", "/api/v1/spaces/PRV/templates", templateTestInput("hidden"))
	if priv.status != 201 {
		t.Fatalf("space admin create: %d %s", priv.status, priv.raw)
	}
	pid := priv.body["id"].(string)
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		if r := outsider.do(method, "/api/v1/templates/"+pid, map[string]any{"name": "x"}); r.status != 404 {
			t.Errorf("outsider %s on private template: %d", method, r.status)
		}
	}
}

func TestTemplatesEditorOfOtherSpaceCannotManage(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	created := admin.do("POST", "/api/v1/spaces/OPS/templates", templateTestInput("ops only"))
	id := created.body["id"].(string)
	editor := w.as(t, w.editor)
	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "x"}); r.status != 403 {
		t.Fatalf("viewer-in-OPS patch: %d", r.status)
	}
	if r := editor.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 403 {
		t.Fatalf("viewer-in-OPS delete: %d", r.status)
	}
}

func TestTemplatesArchivedSpaceReadOnly(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	created := admin.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("frozen"))
	id := created.body["id"].(string)
	if _, err := w.e.store.Pool.Exec(t.Context(), `UPDATE spaces SET archived_at = now() WHERE id = $1`, w.eng); err != nil {
		t.Fatal(err)
	}
	editor := w.as(t, w.editor)
	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "x"}); r.status != 403 {
		t.Fatalf("editor patch archived: %d", r.status)
	}
	if r := editor.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("x")); r.status != 403 {
		t.Fatalf("editor create archived: %d", r.status)
	}
	if r := admin.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "ok"}); r.status != 200 {
		t.Fatalf("admin patch archived: %d", r.status)
	}
}

func TestTemplatesGlobalRequiresSystemAdmin(t *testing.T) {
	w := newTemplateWorld(t)
	editor := w.as(t, w.editor)
	if r := editor.do("POST", "/api/v1/admin/templates", templateTestInput("g")); r.status != 403 {
		t.Fatalf("non-admin create global: %d", r.status)
	}
	if r := w.e.client().do("POST", "/api/v1/admin/templates", templateTestInput("g")); r.status != 401 {
		t.Fatalf("anonymous create global: %d", r.status)
	}

	admin := w.as(t, w.admin)
	created := admin.do("POST", "/api/v1/admin/templates", templateTestInput("Company handbook"))
	if created.status != 201 || created.body["space_id"] != nil || created.body["space_key"] != nil {
		t.Fatalf("create global: %d %s", created.status, created.raw)
	}
	id := created.body["id"].(string)

	if r := editor.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "x"}); r.status != 403 {
		t.Fatalf("non-admin patch global: %d", r.status)
	}
	if r := editor.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 403 {
		t.Fatalf("non-admin delete global: %d", r.status)
	}
	if r := editor.do("GET", "/api/v1/templates/"+id, nil); r.status != 200 || r.body["name"] != "Company handbook" {
		t.Fatalf("non-admin read global: %d %s", r.status, r.raw)
	}
	if r := w.as(t, w.outsider).do("GET", "/api/v1/templates/"+id, nil); r.status != 200 {
		t.Fatalf("outsider read global: %d", r.status)
	}

	patched := admin.do("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "Handbook"})
	if patched.status != 200 || patched.body["name"] != "Handbook" {
		t.Fatalf("admin patch global: %d %s", patched.status, patched.raw)
	}
	if r := admin.do("DELETE", "/api/v1/templates/"+id, nil); r.status != 204 {
		t.Fatalf("admin delete global: %d", r.status)
	}
	if r := admin.do("GET", "/api/v1/templates/"+id, nil); r.status != 404 {
		t.Fatalf("get deleted: %d", r.status)
	}
	if r := admin.do("GET", "/api/v1/templates/not-a-uuid", nil); r.status != 400 {
		t.Fatalf("bad id: %d", r.status)
	}
	if r := admin.do("GET", "/api/v1/templates/"+uuid.NewString(), nil); r.status != 404 {
		t.Fatalf("unknown id: %d", r.status)
	}
}

func TestTemplatesSanitiserStripsScript(t *testing.T) {
	w := newTemplateWorld(t)
	editor := w.as(t, w.editor)
	in := templateTestInput("xss")
	in["content_html"] = `<p onclick="x()">hi</p><script>alert(1)</script><img src="javascript:alert(1)"><a href="javascript:alert(2)">l</a>`
	r := editor.do("POST", "/api/v1/spaces/ENG/templates", in)
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	html := r.body["content_html"].(string)
	for _, bad := range []string{"<script", "alert", "onclick", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("html still contains %q: %s", bad, html)
		}
	}
	if !strings.Contains(html, "<p>hi</p>") {
		t.Fatalf("html lost content: %s", html)
	}
	stored := editor.do("GET", "/api/v1/templates/"+r.body["id"].(string), nil)
	if stored.body["content_html"] != html {
		t.Fatalf("stored html differs: %v", stored.body["content_html"])
	}
}

func TestTemplatesCreatePageFromTemplate(t *testing.T) {
	w := newTemplateWorld(t)
	editor := w.as(t, w.editor)
	tpl := editor.do("POST", "/api/v1/spaces/ENG/templates", templateTestInput("Weekly sync"))
	tplID := tpl.body["id"].(string)

	r := editor.do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "", "template_id": tplID})
	if r.status != 201 {
		t.Fatalf("create page: %d %s", r.status, r.raw)
	}
	if r.body["title"] != "Weekly sync" || r.body["status"] != "draft" || r.body["content_html"] != "<p>hello body</p>" {
		t.Fatalf("page %v", r.body)
	}
	doc, ok := r.body["content_json"].(map[string]any)
	if !ok || doc["type"] != "doc" || !strings.Contains(string(r.raw), "hello body") {
		t.Fatalf("content_json %v", r.body["content_json"])
	}

	named := editor.do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "My title", "template_id": tplID})
	if named.status != 201 || named.body["title"] != "My title" || named.body["content_html"] != "<p>hello body</p>" {
		t.Fatalf("explicit title wins: %d %s", named.status, named.raw)
	}

	if r := editor.do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "template_id": uuid.NewString()}); r.status != 404 {
		t.Fatalf("unknown template: %d", r.status)
	}
}

func TestTemplatesCreatePageFromGlobalTemplate(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	list := admin.do("GET", "/api/v1/templates", nil)
	var meetingID string
	for _, item := range list.list {
		if item["name"] == "Meeting notes" {
			meetingID = item["id"].(string)
		}
	}
	if meetingID == "" {
		t.Fatalf("no starter: %s", list.raw)
	}
	editor := w.as(t, w.editor)
	r := editor.do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "", "template_id": meetingID})
	if r.status != 201 || r.body["title"] != "Meeting notes" {
		t.Fatalf("page from starter: %d %s", r.status, r.raw)
	}
	html := r.body["content_html"].(string)
	if !strings.Contains(html, `data-type="panel"`) || !strings.Contains(html, "<table>") || !strings.Contains(html, `data-type="taskList"`) {
		t.Fatalf("starter structure lost: %s", html)
	}
}

func TestTemplatesCrossSpaceTemplateRejectedOnPageCreate(t *testing.T) {
	w := newTemplateWorld(t)
	admin := w.as(t, w.admin)
	tpl := admin.do("POST", "/api/v1/spaces/OPS/templates", templateTestInput("ops tpl"))
	if tpl.status != 201 {
		t.Fatalf("create: %d %s", tpl.status, tpl.raw)
	}
	editor := w.as(t, w.editor)
	r := editor.do("POST", "/api/v1/spaces/ENG/pages", map[string]any{"title": "x", "template_id": tpl.body["id"]})
	if r.status != 404 {
		t.Fatalf("cross-space template: %d %s", r.status, r.raw)
	}
	same := admin.do("POST", "/api/v1/spaces/OPS/pages", map[string]any{"title": "ok", "template_id": tpl.body["id"]})
	if same.status != 201 {
		t.Fatalf("same-space template: %d %s", same.status, same.raw)
	}
}

func TestTemplatesSeedAtStartupVisibleToEveryone(t *testing.T) {
	w := newTemplateWorld(t)
	ctx := context.Background()
	if err := handlers.SeedStarterTemplates(ctx, w.e.store); err != nil {
		t.Fatal(err)
	}
	if err := handlers.SeedStarterTemplates(ctx, w.e.store); err != nil {
		t.Fatal(err)
	}
	want := "Decision record|How-to guide|Meeting notes|Retrospective"
	user := w.as(t, w.outsider)
	if got := templateTestNames(t, user.do("GET", "/api/v1/templates", nil)); templateTestJoin(got) != want {
		t.Fatalf("starters %v", got)
	}
}
