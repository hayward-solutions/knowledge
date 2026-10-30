package handlers_test

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type searchWorld struct {
	e        *env
	admin    domain.User
	owner    domain.User
	member   domain.User
	outsider domain.User
	pub      uuid.UUID
	priv     uuid.UUID
	gone     uuid.UUID
	archived uuid.UUID
}

func newSearchWorld(t *testing.T) *searchWorld {
	t.Helper()
	e := newEnv(t, nil)
	w := &searchWorld{e: e}
	w.admin = testutil.CreateUser(t, e.store, "admin@example.test", "", true)
	w.owner = testutil.CreateUser(t, e.store, "owner@example.test", "", false)
	w.member = testutil.CreateUser(t, e.store, "member@example.test", "", false)
	w.outsider = testutil.CreateUser(t, e.store, "outsider@example.test", "", false)
	w.pub = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "PUB", CreatedBy: w.owner.ID})
	w.priv = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "PRV", Visibility: domain.VisibilityPrivate, CreatedBy: w.owner.ID})
	w.gone = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "DEL", Deleted: true, CreatedBy: w.owner.ID})
	w.archived = testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "OLD", Archived: true, CreatedBy: w.owner.ID})
	testutil.AddMember(t, e.store, w.priv, w.member.ID, domain.RoleViewer)
	return w
}

func (w *searchWorld) as(u domain.User) *client {
	c := w.e.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func searchTestSeed(t *testing.T, e *env, space, author uuid.UUID, title, content, status string) uuid.UUID {
	t.Helper()
	id := testutil.CreatePage(t, e.store, space, author, title)
	if _, err := e.store.Pool.Exec(context.Background(),
		`UPDATE pages SET content_text = $2, status = $3 WHERE id = $1`, id, content, status); err != nil {
		t.Fatal(err)
	}
	return id
}

func searchTestHitTitles(t *testing.T, r response) []string {
	t.Helper()
	if r.status != 200 {
		t.Fatalf("status %d: %s", r.status, r.raw)
	}
	hits, _ := r.body["hits"].([]any)
	out := []string{}
	for _, h := range hits {
		page := h.(map[string]any)["page"].(map[string]any)
		out = append(out, page["title"].(string))
	}
	return out
}

func searchTestSorted(in []string) string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return strings.Join(out, ",")
}

func searchTestURL(params map[string]string) string {
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	return "/api/v1/search?" + v.Encode()
}

func TestSearchRequiresAuthAndQuery(t *testing.T) {
	w := newSearchWorld(t)
	anon := w.e.client()
	for _, p := range []string{"/api/v1/search?q=x", "/api/v1/search/quick?q=x"} {
		if r := anon.do("GET", p, nil); r.status != 401 {
			t.Errorf("%s: %d", p, r.status)
		}
	}
	c := w.as(w.outsider)
	for _, p := range []string{"/api/v1/search", "/api/v1/search?q=", "/api/v1/search?q=%20%20"} {
		if r := c.do("GET", p, nil); r.status != 400 {
			t.Errorf("%s: %d %s", p, r.status, r.raw)
		}
	}
	if r := c.do("GET", "/api/v1/search?q="+strings.Repeat("a", 500), nil); r.status != 400 {
		t.Errorf("long query: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/search?q=x&author=not-a-uuid", nil); r.status != 400 {
		t.Errorf("bad author: %d", r.status)
	}
}

func TestSearchAccessRules(t *testing.T) {
	w := newSearchWorld(t)
	for _, sp := range []uuid.UUID{w.pub, w.priv, w.gone, w.archived} {
		searchTestSeed(t, w.e, sp, w.owner.ID, fmt.Sprintf("Page in %s", sp.String()[:8]), "kubernetes cluster upgrade", "published")
	}
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Hidden draft", "kubernetes cluster upgrade", "draft")
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Hidden trashed", "kubernetes cluster upgrade", "trashed")

	count := func(u domain.User) int {
		r := w.as(u).do("GET", searchTestURL(map[string]string{"q": "kubernetes"}), nil)
		titles := searchTestHitTitles(t, r)
		if int(r.body["total"].(float64)) != len(titles) {
			t.Errorf("total %v vs %d hits", r.body["total"], len(titles))
		}
		for _, title := range titles {
			if strings.Contains(title, "Hidden") {
				t.Errorf("%s saw %q", u.Email, title)
			}
		}
		return len(titles)
	}
	if n := count(w.outsider); n != 2 {
		t.Errorf("outsider (internal + archived): %d", n)
	}
	if n := count(w.member); n != 3 {
		t.Errorf("member (private too): %d", n)
	}
	if n := count(w.admin); n != 3 {
		t.Errorf("system admin (everything but deleted): %d", n)
	}

	r := w.as(w.outsider).do("GET", searchTestURL(map[string]string{"q": "kubernetes", "space": "prv"}), nil)
	if r.status != 200 || r.body["total"].(float64) != 0 {
		t.Errorf("filtering to an inaccessible space must not leak: %s", r.raw)
	}
	r = w.as(w.admin).do("GET", searchTestURL(map[string]string{"q": "kubernetes", "space": "del"}), nil)
	if r.status != 200 || r.body["total"].(float64) != 0 {
		t.Errorf("deleted space must not be searchable by admins: %s", r.raw)
	}
}

func TestSearchResponseShape(t *testing.T) {
	w := newSearchWorld(t)
	parent := searchTestSeed(t, w.e, w.pub, w.owner.ID, "Backup policy", "Nightly backups run at two and are verified by restoring them into a scratch database.", "published")
	child := searchTestSeed(t, w.e, w.pub, w.owner.ID, "Child", "nothing", "published")
	if _, err := w.e.store.Pool.Exec(context.Background(), `UPDATE pages SET parent_id = $2 WHERE id = $1`, child, parent); err != nil {
		t.Fatal(err)
	}

	r := w.as(w.outsider).do("GET", searchTestURL(map[string]string{"q": "backups"}), nil)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	for _, k := range []string{"hits", "total", "page", "limit"} {
		if _, ok := r.body[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
	if r.body["page"].(float64) != 1 || r.body["limit"].(float64) != 20 || r.body["total"].(float64) != 1 {
		t.Fatalf("envelope %s", r.raw)
	}
	hit := r.body["hits"].([]any)[0].(map[string]any)
	for _, k := range []string{"page", "space_key", "snippet", "updated_at", "author"} {
		if _, ok := hit[k]; !ok {
			t.Errorf("hit missing %q", k)
		}
	}
	page := hit["page"].(map[string]any)
	for _, k := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children"} {
		if _, ok := page[k]; !ok {
			t.Errorf("page summary missing %q", k)
		}
	}
	if page["id"] != parent.String() || page["has_children"] != true || page["status"] != "published" {
		t.Errorf("page %v", page)
	}
	if hit["space_key"] != "PUB" {
		t.Errorf("space_key %v", hit["space_key"])
	}
	if !strings.Contains(hit["snippet"].(string), "<mark>backups</mark>") {
		t.Errorf("snippet %q", hit["snippet"])
	}
	author := hit["author"].(map[string]any)
	if author["id"] != w.owner.ID.String() || author["display_name"] == "" {
		t.Errorf("author %v", author)
	}

	r = w.as(w.outsider).do("GET", searchTestURL(map[string]string{"q": "zzzzzz"}), nil)
	if r.status != 200 || !strings.Contains(string(r.raw), `"hits":[]`) || r.body["total"].(float64) != 0 {
		t.Errorf("empty result %s", r.raw)
	}
}

func TestSearchFilters(t *testing.T) {
	w := newSearchWorld(t)
	ctx := context.Background()
	other := testutil.CreateUser(t, w.e.store, "other@example.test", "", false)
	a := searchTestSeed(t, w.e, w.pub, w.owner.ID, "Alpha", "deployment guide", "published")
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Beta", "deployment guide", "published")
	c := searchTestSeed(t, w.e, w.archived, w.owner.ID, "Gamma", "deployment guide", "published")
	if _, err := w.e.store.Pool.Exec(ctx, `UPDATE pages SET updated_by = $2 WHERE id = $1`, c, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.e.store.SetPageLabels(ctx, a, []string{"ops"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.e.store.SetPageLabels(ctx, c, []string{"ops"}); err != nil {
		t.Fatal(err)
	}
	cl := w.as(w.outsider)

	cases := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"none", map[string]string{"q": "deployment"}, "Alpha,Beta,Gamma"},
		{"space", map[string]string{"q": "deployment", "space": "pub"}, "Alpha,Beta"},
		{"label", map[string]string{"q": "deployment", "label": "OPS"}, "Alpha,Gamma"},
		{"label+space", map[string]string{"q": "deployment", "label": "ops", "space": "OLD"}, "Gamma"},
		{"author", map[string]string{"q": "deployment", "author": other.ID.String()}, "Gamma"},
		{"author+space", map[string]string{"q": "deployment", "author": other.ID.String(), "space": "PUB"}, ""},
		{"unknown label", map[string]string{"q": "deployment", "label": "nope"}, ""},
	}
	for _, tc := range cases {
		r := cl.do("GET", searchTestURL(tc.params), nil)
		if got := searchTestSorted(searchTestHitTitles(t, r)); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSearchRanksTitleMatchesFirst(t *testing.T) {
	w := newSearchWorld(t)
	for i := 0; i < 3; i++ {
		searchTestSeed(t, w.e, w.pub, w.owner.ID, fmt.Sprintf("Handbook %d", i), "How to restore the database after a failure. The database is critical.", "published")
	}
	titleHit := searchTestSeed(t, w.e, w.pub, w.owner.ID, "Database", "Short.", "published")
	r := w.as(w.outsider).do("GET", searchTestURL(map[string]string{"q": "database"}), nil)
	titles := searchTestHitTitles(t, r)
	if len(titles) != 4 || titles[0] != "Database" {
		t.Fatalf("ranking %v (title page %s)", titles, titleHit)
	}
}

func TestSearchPaginationTotals(t *testing.T) {
	w := newSearchWorld(t)
	for i := 0; i < 5; i++ {
		id := searchTestSeed(t, w.e, w.pub, w.owner.ID, fmt.Sprintf("Incident %d", i), "postmortem template", "published")
		if _, err := w.e.store.Pool.Exec(context.Background(),
			`UPDATE pages SET updated_at = now() - ($2 || ' hours')::interval WHERE id = $1`, id, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	c := w.as(w.outsider)
	seen := map[string]bool{}
	for page, want := range map[string]int{"1": 2, "2": 2, "3": 1, "4": 0} {
		r := c.do("GET", searchTestURL(map[string]string{"q": "postmortem", "limit": "2", "page": page}), nil)
		titles := searchTestHitTitles(t, r)
		if len(titles) != want || r.body["total"].(float64) != 5 || r.body["limit"].(float64) != 2 || fmt.Sprint(r.body["page"]) != page {
			t.Errorf("page %s: %s", page, r.raw)
		}
		for _, title := range titles {
			if seen[title] {
				t.Errorf("repeat %s", title)
			}
			seen[title] = true
		}
	}
	r := c.do("GET", searchTestURL(map[string]string{"q": "postmortem", "limit": "500", "page": "-4"}), nil)
	if r.body["limit"].(float64) != 50 || r.body["page"].(float64) != 1 {
		t.Errorf("clamping %s", r.raw)
	}
}

func TestSearchWebsearchSyntaxIsSafe(t *testing.T) {
	w := newSearchWorld(t)
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "One", "red apples", "published")
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Two", "red cherries", "published")
	c := w.as(w.outsider)
	r := c.do("GET", searchTestURL(map[string]string{"q": "red -apples"}), nil)
	if got := searchTestHitTitles(t, r); len(got) != 1 || got[0] != "Two" {
		t.Errorf("negation %v", got)
	}
	for _, q := range []string{"'", "&", "((", "a:*", "x'); DROP TABLE pages;--", "\\"} {
		if r := c.do("GET", searchTestURL(map[string]string{"q": q}), nil); r.status != 200 {
			t.Errorf("q=%q -> %d %s", q, r.status, r.raw)
		}
	}
	if r := c.do("GET", searchTestURL(map[string]string{"q": "red"}), nil); r.body["total"].(float64) != 2 {
		t.Errorf("table intact: %s", r.raw)
	}
}

func TestQuickSearch(t *testing.T) {
	w := newSearchWorld(t)
	contains := searchTestSeed(t, w.e, w.pub, w.owner.ID, "The Release Guide", "", "published")
	oldPrefix := searchTestSeed(t, w.e, w.pub, w.owner.ID, "Release checklist", "", "published")
	newPrefix := searchTestSeed(t, w.e, w.priv, w.owner.ID, "release notes", "", "published")
	testutil.AddMember(t, w.e.store, w.priv, w.outsider.ID, domain.RoleViewer)
	searchTestSeed(t, w.e, w.gone, w.owner.ID, "Release ghost", "", "published")
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Release trashed", "", "trashed")
	searchTestSeed(t, w.e, w.pub, w.owner.ID, "Release their draft", "", "draft")
	mine := searchTestSeed(t, w.e, w.pub, w.outsider.ID, "Release my draft", "", "draft")
	for id, age := range map[uuid.UUID]time.Duration{contains: 0, oldPrefix: 72 * time.Hour, newPrefix: time.Hour, mine: 2 * time.Hour} {
		if _, err := w.e.store.Pool.Exec(context.Background(),
			`UPDATE pages SET updated_at = now() - $2::interval WHERE id = $1`, id, age.String()); err != nil {
			t.Fatal(err)
		}
	}

	c := w.as(w.outsider)
	r := c.do("GET", "/api/v1/search/quick?q=release", nil)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	var titles []string
	for _, h := range r.list {
		titles = append(titles, h["page"].(map[string]any)["title"].(string))
	}
	want := []string{"release notes", "Release my draft", "Release checklist", "The Release Guide"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("order %v, want %v", titles, want)
	}
	first := r.list[0]
	if first["space_key"] != "PRV" || first["space_name"] != "PRV" {
		t.Errorf("hit %v", first)
	}
	for _, k := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children"} {
		if _, ok := first["page"].(map[string]any)[k]; !ok {
			t.Errorf("page missing %q", k)
		}
	}
	if r.list[1]["page"].(map[string]any)["status"] != "draft" {
		t.Errorf("own draft status: %v", r.list[1])
	}

	r = c.do("GET", "/api/v1/search/quick?q=release&limit=2", nil)
	if len(r.list) != 2 {
		t.Errorf("limit: %s", r.raw)
	}
	r = c.do("GET", "/api/v1/search/quick?q=release&limit=500", nil)
	if r.status != 200 {
		t.Errorf("limit clamp: %d", r.status)
	}

	r = w.as(w.owner).do("GET", "/api/v1/search/quick?q=release%20my", nil)
	if len(r.list) != 0 {
		t.Errorf("other users' drafts must not leak: %s", r.raw)
	}
	r = w.as(w.admin).do("GET", "/api/v1/search/quick?q=ghost", nil)
	if len(r.list) != 0 {
		t.Errorf("deleted space leaked: %s", r.raw)
	}
	r = c.do("GET", "/api/v1/search/quick?q=zzzz", nil)
	if r.status != 200 || string(r.raw) != "[]\n" {
		t.Errorf("no match: %d %q", r.status, r.raw)
	}
	r = c.do("GET", "/api/v1/search/quick?q=%25", nil)
	if r.status != 200 || len(r.list) != 0 {
		t.Errorf("percent must be literal: %s", r.raw)
	}
}

func TestQuickSearchPrivateExcludedForNonMember(t *testing.T) {
	w := newSearchWorld(t)
	searchTestSeed(t, w.e, w.priv, w.owner.ID, "Roadmap", "", "published")
	r := w.as(w.outsider).do("GET", "/api/v1/search/quick?q=roadmap", nil)
	if r.status != 200 || len(r.list) != 0 {
		t.Fatalf("%s", r.raw)
	}
	r = w.as(w.member).do("GET", "/api/v1/search/quick?q=roadmap", nil)
	if len(r.list) != 1 {
		t.Fatalf("%s", r.raw)
	}
	r = w.as(w.admin).do("GET", "/api/v1/search/quick?q=roadmap", nil)
	if len(r.list) != 1 {
		t.Fatalf("%s", r.raw)
	}
}
