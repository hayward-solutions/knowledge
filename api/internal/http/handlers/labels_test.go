package handlers_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type labelWorld struct {
	e        *env
	admin    domain.User
	editor   domain.User
	viewer   domain.User
	outsider domain.User
	pub      uuid.UUID
	priv     uuid.UUID
}

func newLabelWorld(t *testing.T) *labelWorld {
	t.Helper()
	e := newEnv(t, nil)
	w := &labelWorld{e: e}
	w.admin = testutil.CreateUser(t, e.store, "admin@example.test", "", true)
	w.editor = testutil.CreateUser(t, e.store, "editor@example.test", "", false)
	w.viewer = testutil.CreateUser(t, e.store, "viewer@example.test", "", false)
	w.outsider = testutil.CreateUser(t, e.store, "outsider@example.test", "", false)
	w.pub = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "PUB", CreatedBy: w.admin.ID})
	w.priv = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "PRV", Visibility: domain.VisibilityPrivate, CreatedBy: w.admin.ID})
	testutil.AddMember(t, e.store, w.pub, w.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, e.store, w.priv, w.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, e.store, w.priv, w.viewer.ID, domain.RoleViewer)
	return w
}

func (w *labelWorld) as(u domain.User) *client {
	c := w.e.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func labelTestResponseNames(t *testing.T, r response) []string {
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

func labelTestJoin(names []string) string { return strings.Join(names, ",") }

func labelTestExec(t *testing.T, e *env, sql string, args ...any) {
	t.Helper()
	if _, err := e.store.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestLabelsRequireAuth(t *testing.T) {
	w := newLabelWorld(t)
	page := testutil.CreatePage(t, w.e.store, w.pub, w.admin.ID, "P")
	anon := w.e.client()
	for _, path := range []string{
		"/api/v1/labels",
		"/api/v1/pages/" + page.String() + "/labels",
		"/api/v1/spaces/PUB/labels",
		"/api/v1/spaces/PUB/labels/x/pages",
	} {
		if r := anon.do("GET", path, nil); r.status != 401 {
			t.Errorf("GET %s: %d", path, r.status)
		}
	}
	if r := anon.do("PUT", "/api/v1/pages/"+page.String()+"/labels", map[string]any{"labels": []string{"a"}}); r.status != 401 {
		t.Errorf("PUT: %d", r.status)
	}
}

func TestSetPageLabelsNormalisesAndReplaces(t *testing.T) {
	w := newLabelWorld(t)
	page := testutil.CreatePage(t, w.e.store, w.pub, w.admin.ID, "P")
	c := w.as(w.editor)
	path := "/api/v1/pages/" + page.String() + "/labels"

	r := c.do("PUT", path, map[string]any{"labels": []string{"  Runbook  ", "On Call", "v1.2_rc", "runbook", "ONCALL", "Multi   Space  Name"}})
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "multi-space-name,on-call,oncall,runbook,v1.2_rc" {
		t.Fatalf("names %q", got)
	}
	for _, item := range r.list {
		if item["id"] == "" || item["created_at"] == nil {
			t.Fatalf("label shape %v", item)
		}
	}

	r = c.do("PUT", path, map[string]any{"labels": []string{"oncall", "new"}})
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "new,oncall" {
		t.Fatalf("replace %q", got)
	}
	r = c.do("GET", path, nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "new,oncall" {
		t.Fatalf("get %q", got)
	}

	r = c.do("PUT", path, map[string]any{"labels": []string{}})
	if got := labelTestResponseNames(t, r); len(got) != 0 {
		t.Fatalf("clear %v", got)
	}
	r = c.do("GET", path, nil)
	if r.status != 200 || string(r.raw) != "[]\n" {
		t.Fatalf("empty list body %q", r.raw)
	}
}

func TestSetPageLabelsValidation(t *testing.T) {
	w := newLabelWorld(t)
	page := testutil.CreatePage(t, w.e.store, w.pub, w.admin.ID, "P")
	c := w.as(w.editor)
	path := "/api/v1/pages/" + page.String() + "/labels"

	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprintf("l%d", i)
	}
	twenty := many[:20]

	bad := map[string]any{
		"empty name":      map[string]any{"labels": []string{"ok", ""}},
		"blank name":      map[string]any{"labels": []string{"   "}},
		"uppercase ok?":   map[string]any{"labels": []string{"café"}},
		"slash":           map[string]any{"labels": []string{"a/b"}},
		"emoji":           map[string]any{"labels": []string{"\U0001F600"}},
		"colon":           map[string]any{"labels": []string{"team:ops"}},
		"too long":        map[string]any{"labels": []string{strings.Repeat("a", 51)}},
		"too many":        map[string]any{"labels": many},
		"missing field":   map[string]any{},
		"null field":      map[string]any{"labels": nil},
		"wrong type":      map[string]any{"labels": "x"},
		"number in array": map[string]any{"labels": []any{1}},
	}
	for name, body := range bad {
		if r := c.do("PUT", path, body); r.status != 400 {
			t.Errorf("%s: status %d: %s", name, r.status, r.raw)
		}
	}

	if r := c.do("PUT", path, map[string]any{"labels": []string{strings.Repeat("a", 50)}}); r.status != 200 {
		t.Errorf("50 chars: %d %s", r.status, r.raw)
	}
	if r := c.do("PUT", path, map[string]any{"labels": twenty}); r.status != 200 || len(r.list) != 20 {
		t.Errorf("20 labels: %d %s", r.status, r.raw)
	}
	if r := c.do("PUT", path, map[string]any{"labels": append(append([]string{}, twenty...), "L0", "l1")}); r.status != 200 {
		t.Errorf("duplicates collapse before the cap: %d %s", r.status, r.raw)
	}

	before := c.do("GET", path, nil)
	c.do("PUT", path, map[string]any{"labels": []string{"fine", "bad/name"}})
	after := c.do("GET", path, nil)
	if string(before.raw) != string(after.raw) {
		t.Fatalf("failed PUT must not change labels:\n%s\n%s", before.raw, after.raw)
	}
}

func TestPageLabelsPermissions(t *testing.T) {
	w := newLabelWorld(t)
	pubPage := testutil.CreatePage(t, w.e.store, w.pub, w.admin.ID, "Pub")
	privPage := testutil.CreatePage(t, w.e.store, w.priv, w.admin.ID, "Priv")
	body := map[string]any{"labels": []string{"x"}}

	viewer := w.as(w.viewer)
	if r := viewer.do("PUT", "/api/v1/pages/"+privPage.String()+"/labels", body); r.status != 403 {
		t.Errorf("viewer PUT: %d", r.status)
	}
	if r := viewer.do("GET", "/api/v1/pages/"+privPage.String()+"/labels", nil); r.status != 200 {
		t.Errorf("viewer GET: %d", r.status)
	}
	outsider := w.as(w.outsider)
	if r := outsider.do("PUT", "/api/v1/pages/"+pubPage.String()+"/labels", body); r.status != 403 {
		t.Errorf("internal viewer PUT: %d", r.status)
	}
	if r := outsider.do("GET", "/api/v1/pages/"+pubPage.String()+"/labels", nil); r.status != 200 {
		t.Errorf("internal viewer GET: %d", r.status)
	}
	if r := outsider.do("GET", "/api/v1/pages/"+privPage.String()+"/labels", nil); r.status != 404 {
		t.Errorf("non-member GET private: %d", r.status)
	}
	if r := outsider.do("PUT", "/api/v1/pages/"+privPage.String()+"/labels", body); r.status != 404 {
		t.Errorf("non-member PUT private: %d", r.status)
	}
	editor := w.as(w.editor)
	if r := editor.do("PUT", "/api/v1/pages/"+uuid.Must(uuid.NewV7()).String()+"/labels", body); r.status != 404 {
		t.Errorf("unknown page: %d", r.status)
	}
	if r := editor.do("PUT", "/api/v1/pages/not-a-uuid/labels", body); r.status != 400 {
		t.Errorf("bad id: %d", r.status)
	}
	if r := editor.do("PUT", "/api/v1/pages/"+privPage.String()+"/labels", body); r.status != 200 {
		t.Errorf("editor PUT: %d", r.status)
	}
	admin := w.as(w.admin)
	if r := admin.do("PUT", "/api/v1/pages/"+privPage.String()+"/labels", body); r.status != 200 {
		t.Errorf("system admin PUT: %d", r.status)
	}
}

func TestPageLabelsArchivedSpaceIsReadOnly(t *testing.T) {
	w := newLabelWorld(t)
	space := testutil.CreateSpace(t, w.e.store, testutil.SpaceOpts{Key: "ARC", Archived: true, CreatedBy: w.admin.ID})
	testutil.AddMember(t, w.e.store, space, w.editor.ID, domain.RoleEditor)
	page := testutil.CreatePage(t, w.e.store, space, w.admin.ID, "Old")
	path := "/api/v1/pages/" + page.String() + "/labels"
	body := map[string]any{"labels": []string{"x"}}
	if r := w.as(w.editor).do("PUT", path, body); r.status != 403 {
		t.Errorf("editor on archived: %d", r.status)
	}
	if r := w.as(w.editor).do("GET", path, nil); r.status != 200 {
		t.Errorf("read archived: %d", r.status)
	}
	if r := w.as(w.admin).do("PUT", path, body); r.status != 200 {
		t.Errorf("admin on archived: %d", r.status)
	}
}

func TestPageLabelsDraftVisibility(t *testing.T) {
	w := newLabelWorld(t)
	otherEditor := testutil.CreateUser(t, w.e.store, "editor2@example.test", "", false)
	testutil.AddMember(t, w.e.store, w.pub, otherEditor.ID, domain.RoleEditor)
	spaceAdmin := testutil.CreateUser(t, w.e.store, "spaceadmin@example.test", "", false)
	testutil.AddMember(t, w.e.store, w.pub, spaceAdmin.ID, domain.RoleAdmin)

	draft := testutil.CreatePage(t, w.e.store, w.pub, w.editor.ID, "Draft")
	labelTestExec(t, w.e, `UPDATE pages SET status = 'draft' WHERE id = $1`, draft)
	path := "/api/v1/pages/" + draft.String() + "/labels"
	body := map[string]any{"labels": []string{"wip"}}

	if r := w.as(w.editor).do("PUT", path, body); r.status != 200 {
		t.Errorf("creator PUT: %d", r.status)
	}
	if r := w.as(w.editor).do("GET", path, nil); r.status != 200 || len(r.list) != 1 {
		t.Errorf("creator GET: %d %s", r.status, r.raw)
	}
	for name, u := range map[string]domain.User{"other editor": otherEditor, "viewer-ish outsider": w.outsider} {
		c := w.as(u)
		if r := c.do("GET", path, nil); r.status != 404 {
			t.Errorf("%s GET: %d", name, r.status)
		}
		if r := c.do("PUT", path, body); r.status != 404 && r.status != 403 {
			t.Errorf("%s PUT: %d", name, r.status)
		}
	}
	if r := w.as(otherEditor).do("PUT", path, map[string]any{"labels": []string{"hijack"}}); r.status != 404 {
		t.Errorf("other editor PUT: %d", r.status)
	}
	for name, u := range map[string]domain.User{"space admin": spaceAdmin, "system admin": w.admin} {
		if r := w.as(u).do("GET", path, nil); r.status != 200 {
			t.Errorf("%s GET: %d", name, r.status)
		}
	}
	r := w.as(w.editor).do("GET", path, nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "wip" {
		t.Errorf("labels after rejected writes: %q", got)
	}
}

func TestListLabelsEndpoint(t *testing.T) {
	w := newLabelWorld(t)
	label := func(space uuid.UUID, title string, names ...string) uuid.UUID {
		id := testutil.CreatePage(t, w.e.store, space, w.admin.ID, title)
		if _, err := w.e.store.SetPageLabels(context.Background(), id, names); err != nil {
			t.Fatal(err)
		}
		return id
	}
	label(w.pub, "a", "runbook", "ops")
	label(w.pub, "b", "runbook")
	label(w.priv, "c", "runbook", "secret")
	label(w.priv, "d", "secret", "runbook-private")
	draft := label(w.pub, "e", "draftonly")
	labelTestExec(t, w.e, `UPDATE pages SET status = 'draft' WHERE id = $1`, draft)

	r := w.as(w.outsider).do("GET", "/api/v1/labels", nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "runbook,ops" {
		t.Fatalf("outsider %q", got)
	}
	if r.list[0]["page_count"].(float64) != 2 || r.list[1]["page_count"].(float64) != 1 {
		t.Fatalf("counts %v", r.list)
	}
	if r.list[0]["id"] == nil || r.list[0]["created_at"] == nil {
		t.Fatalf("shape %v", r.list[0])
	}

	r = w.as(w.viewer).do("GET", "/api/v1/labels", nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "runbook,secret,ops,runbook-private" {
		t.Fatalf("member %q", got)
	}
	if r.list[0]["page_count"].(float64) != 3 {
		t.Fatalf("member counts %v", r.list)
	}

	r = w.as(w.admin).do("GET", "/api/v1/labels?q=RUN", nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "runbook,runbook-private" {
		t.Fatalf("prefix %q", got)
	}
	r = w.as(w.admin).do("GET", "/api/v1/labels?q=book", nil)
	if len(r.list) != 0 {
		t.Fatalf("prefix must anchor: %s", r.raw)
	}
	r = w.as(w.admin).do("GET", "/api/v1/labels?limit=1", nil)
	if len(r.list) != 1 {
		t.Fatalf("limit: %s", r.raw)
	}
	r = w.as(w.admin).do("GET", "/api/v1/labels?limit=999", nil)
	if r.status != 200 {
		t.Fatalf("limit clamp: %d", r.status)
	}
}

func TestSpaceLabelsAndPagesEndpoints(t *testing.T) {
	w := newLabelWorld(t)
	ctx := context.Background()
	mk := func(space uuid.UUID, title string, names ...string) uuid.UUID {
		id := testutil.CreatePage(t, w.e.store, space, w.admin.ID, title)
		if _, err := w.e.store.SetPageLabels(ctx, id, names); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mk(w.pub, "Zebra", "animals", "stripes")
	mk(w.pub, "apple", "animals")
	mk(w.pub, "Mango", "fruit")
	mk(w.priv, "Hidden", "animals")
	trashed := mk(w.pub, "Gone", "animals")
	labelTestExec(t, w.e, `UPDATE pages SET status = 'trashed' WHERE id = $1`, trashed)

	c := w.as(w.outsider)
	r := c.do("GET", "/api/v1/spaces/pub/labels", nil)
	if got := labelTestJoin(labelTestResponseNames(t, r)); got != "animals,fruit,stripes" {
		t.Fatalf("space labels %q", got)
	}
	if r.list[0]["page_count"].(float64) != 2 {
		t.Fatalf("space-scoped count %v", r.list[0])
	}

	r = c.do("GET", "/api/v1/spaces/PUB/labels/animals/pages", nil)
	if r.status != 200 || len(r.list) != 2 || r.list[0]["title"] != "apple" || r.list[1]["title"] != "Zebra" {
		t.Fatalf("pages %s", r.raw)
	}
	for _, key := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children"} {
		if _, ok := r.list[0][key]; !ok {
			t.Errorf("PageSummary missing %q: %v", key, r.list[0])
		}
	}
	if r.list[0]["status"] != "published" {
		t.Errorf("status %v", r.list[0]["status"])
	}

	r = c.do("GET", "/api/v1/spaces/PUB/labels/"+url.PathEscape("Animals")+"/pages", nil)
	if r.status != 200 || len(r.list) != 2 {
		t.Fatalf("case-insensitive name: %s", r.raw)
	}
	r = c.do("GET", "/api/v1/spaces/PUB/labels/unknown/pages", nil)
	if r.status != 200 || string(r.raw) != "[]\n" {
		t.Fatalf("unknown label: %d %q", r.status, r.raw)
	}
	r = c.do("GET", "/api/v1/spaces/PUB/labels/bad%2Fname/pages", nil)
	if r.status != 200 || len(r.list) != 0 {
		t.Fatalf("invalid label name: %d %s", r.status, r.raw)
	}

	if r := c.do("GET", "/api/v1/spaces/PRV/labels", nil); r.status != 404 {
		t.Errorf("non-member private labels: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/spaces/PRV/labels/animals/pages", nil); r.status != 404 {
		t.Errorf("non-member private label pages: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/spaces/NOPE/labels", nil); r.status != 404 {
		t.Errorf("unknown space: %d", r.status)
	}
	r = w.as(w.viewer).do("GET", "/api/v1/spaces/PRV/labels/animals/pages", nil)
	if r.status != 200 || len(r.list) != 1 || r.list[0]["title"] != "Hidden" {
		t.Errorf("member private label pages: %d %s", r.status, r.raw)
	}
}

func TestDeletedSpaceLabelsHiddenFromMembers(t *testing.T) {
	w := newLabelWorld(t)
	space := testutil.CreateSpace(t, w.e.store, testutil.SpaceOpts{Key: "DEL", Deleted: true, CreatedBy: w.admin.ID})
	testutil.AddMember(t, w.e.store, space, w.editor.ID, domain.RoleEditor)
	page := testutil.CreatePage(t, w.e.store, space, w.admin.ID, "x")
	if _, err := w.e.store.SetPageLabels(context.Background(), page, []string{"ghost"}); err != nil {
		t.Fatal(err)
	}
	c := w.as(w.editor)
	if r := c.do("GET", "/api/v1/spaces/DEL/labels", nil); r.status != 404 {
		t.Errorf("deleted space: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/labels?q=ghost", nil); len(r.list) != 0 {
		t.Errorf("deleted space label leaked: %s", r.raw)
	}
}

func TestPageLabelsTrashedPage(t *testing.T) {
	w := newLabelWorld(t)
	page := testutil.CreatePage(t, w.e.store, w.priv, w.admin.ID, "Trashed")
	if _, err := w.e.store.SetPageLabels(context.Background(), page, []string{"keep"}); err != nil {
		t.Fatal(err)
	}
	labelTestExec(t, w.e, `UPDATE pages SET status = 'trashed' WHERE id = $1`, page)
	path := "/api/v1/pages/" + page.String() + "/labels"

	for name, u := range map[string]domain.User{"editor": w.editor, "system admin": w.admin} {
		r := w.as(u).do("PUT", path, map[string]any{"labels": []string{"new"}})
		if r.status != 409 || r.body["code"] != "page_trashed" || r.body["error"] != "page is trashed" {
			t.Errorf("%s PUT: %d %s", name, r.status, r.raw)
		}
		r = w.as(u).do("GET", path, nil)
		if got := labelTestJoin(labelTestResponseNames(t, r)); got != "keep" {
			t.Errorf("%s GET: %q", name, got)
		}
	}
	if r := w.as(w.viewer).do("GET", path, nil); r.status != 404 {
		t.Errorf("viewer GET: %d %s", r.status, r.raw)
	}
	if r := w.as(w.viewer).do("PUT", path, map[string]any{"labels": []string{"new"}}); r.status != 403 {
		t.Errorf("viewer PUT: %d", r.status)
	}
	if r := w.as(w.editor).do("PUT", path, map[string]any{"labels": []string{"bad/name"}}); r.status != 400 {
		t.Errorf("validation still precedes: %d", r.status)
	}
	labels, _ := w.e.store.ListPageLabels(context.Background(), page)
	if len(labels) != 1 || labels[0].Name != "keep" {
		t.Errorf("labels changed on trashed page: %v", labels)
	}

	pub := testutil.CreatePage(t, w.e.store, w.pub, w.admin.ID, "PubTrashed")
	labelTestExec(t, w.e, `UPDATE pages SET status = 'trashed' WHERE id = $1`, pub)
	if r := w.as(w.outsider).do("GET", "/api/v1/pages/"+pub.String()+"/labels", nil); r.status != 404 {
		t.Errorf("internal-space viewer GET: %d", r.status)
	}
}
