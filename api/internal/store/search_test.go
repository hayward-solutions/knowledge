package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/search"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func searchTestPage(t *testing.T, s *store.Store, spaceID, author uuid.UUID, title, content, status string) uuid.UUID {
	t.Helper()
	id := testutil.CreatePage(t, s, spaceID, author, title)
	if _, err := s.Pool.Exec(context.Background(),
		`UPDATE pages SET content_text = $2, status = $3 WHERE id = $1`, id, content, status); err != nil {
		t.Fatal(err)
	}
	return id
}

func searchTestAge(t *testing.T, s *store.Store, pageID uuid.UUID, ago time.Duration) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(),
		`UPDATE pages SET updated_at = now() - $2::interval WHERE id = $1`, pageID, ago.String()); err != nil {
		t.Fatal(err)
	}
}

func searchTestTitles(res store.SearchResult) []string {
	out := make([]string, len(res.Hits))
	for i, h := range res.Hits {
		out[i] = h.Page.Title
	}
	return out
}

type searchWorld struct {
	s        *store.Store
	owner    domain.User
	member   domain.User
	outsider domain.User
	admin    domain.User
	pub      uuid.UUID
	priv     uuid.UUID
	gone     uuid.UUID
	archived uuid.UUID
}

func newSearchWorld(t *testing.T) *searchWorld {
	t.Helper()
	s := testutil.NewDB(t)
	w := &searchWorld{s: s}
	w.owner = testutil.CreateUser(t, s, "owner@example.test", "", false)
	w.member = testutil.CreateUser(t, s, "member@example.test", "", false)
	w.outsider = testutil.CreateUser(t, s, "outsider@example.test", "", false)
	w.admin = testutil.CreateUser(t, s, "admin@example.test", "", true)
	w.pub = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PUB", CreatedBy: w.owner.ID})
	w.priv = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PRV", Visibility: domain.VisibilityPrivate, CreatedBy: w.owner.ID})
	w.gone = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "DEL", Deleted: true, CreatedBy: w.owner.ID})
	w.archived = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "OLD", Archived: true, CreatedBy: w.owner.ID})
	testutil.AddMember(t, s, w.priv, w.member.ID, domain.RoleViewer)
	return w
}

func (w *searchWorld) search(u domain.User, q string) store.SearchResult {
	return w.searchWith(search.Params{Q: q, UserID: u.ID, IsAdmin: u.IsSystemAdmin})
}

func (w *searchWorld) searchWith(p search.Params) store.SearchResult {
	res, err := w.s.SearchPages(context.Background(), p)
	if err != nil {
		panic(err)
	}
	return res
}

func TestSearchPagesAccessRules(t *testing.T) {
	w := newSearchWorld(t)
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Public", "kubernetes cluster", "published")
	searchTestPage(t, w.s, w.priv, w.owner.ID, "Private", "kubernetes cluster", "published")
	searchTestPage(t, w.s, w.gone, w.owner.ID, "Deleted", "kubernetes cluster", "published")
	searchTestPage(t, w.s, w.archived, w.owner.ID, "Archived", "kubernetes cluster", "published")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Draft", "kubernetes cluster", "draft")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Trashed", "kubernetes cluster", "trashed")

	sorted := func(res store.SearchResult) string {
		titles := searchTestTitles(res)
		for i := range titles {
			for j := i + 1; j < len(titles); j++ {
				if titles[j] < titles[i] {
					titles[i], titles[j] = titles[j], titles[i]
				}
			}
		}
		return strings.Join(titles, ",")
	}

	cases := []struct {
		name string
		user domain.User
		want string
	}{
		{"outsider", w.outsider, "Archived,Public"},
		{"member", w.member, "Archived,Private,Public"},
		{"admin", w.admin, "Archived,Private,Public"},
	}
	for _, c := range cases {
		res := w.search(c.user, "kubernetes")
		if got := sorted(res); got != c.want || res.Total != len(res.Hits) {
			t.Errorf("%s: got %q total=%d, want %q", c.name, got, res.Total, c.want)
		}
	}
}

func TestSearchPagesFilters(t *testing.T) {
	w := newSearchWorld(t)
	ctx := context.Background()
	other := testutil.CreateUser(t, w.s, "other@example.test", "", false)
	a := searchTestPage(t, w.s, w.pub, w.owner.ID, "Alpha", "deploy guide", "published")
	b := searchTestPage(t, w.s, w.pub, w.owner.ID, "Beta", "deploy guide", "published")
	c := searchTestPage(t, w.s, w.archived, w.owner.ID, "Gamma", "deploy guide", "published")
	if _, err := w.s.Pool.Exec(ctx, `UPDATE pages SET updated_by = $2 WHERE id = $1`, c, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.s.SetPageLabels(ctx, a, []string{"ops", "guide"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.s.SetPageLabels(ctx, c, []string{"ops"}); err != nil {
		t.Fatal(err)
	}
	_ = b

	base := search.Params{Q: "deploy", UserID: w.outsider.ID}

	p := base
	p.SpaceKey = "pub"
	if res := w.searchWith(p); res.Total != 2 {
		t.Errorf("space filter: %v", searchTestTitles(res))
	}
	p = base
	p.Label = "OPS"
	if res := w.searchWith(p); res.Total != 2 {
		t.Errorf("label filter: %v", searchTestTitles(res))
	}
	p = base
	p.Label = "ops"
	p.SpaceKey = "PUB"
	if res := w.searchWith(p); res.Total != 1 || res.Hits[0].Page.Title != "Alpha" {
		t.Errorf("label+space filter: %v", searchTestTitles(res))
	}
	p = base
	p.AuthorID = other.ID
	res := w.searchWith(p)
	if res.Total != 1 || res.Hits[0].Page.Title != "Gamma" || res.Hits[0].Author == nil || res.Hits[0].Author.ID != other.ID {
		t.Errorf("author filter: %+v", res)
	}
	p = base
	p.SpaceKey = "PRV"
	if res := w.searchWith(p); res.Total != 0 || res.Hits == nil {
		t.Errorf("inaccessible space filter: %+v", res)
	}
}

func TestSearchPagesRankingAndSnippet(t *testing.T) {
	w := newSearchWorld(t)
	body := searchTestPage(t, w.s, w.pub, w.owner.ID, "Operations handbook", "When the pager fires, restart the database and check replication lag before closing the incident.", "published")
	title := searchTestPage(t, w.s, w.pub, w.owner.ID, "Database recovery", "Short note.", "published")
	_ = body

	res := w.search(w.outsider, "database")
	if len(res.Hits) != 2 || res.Hits[0].Page.ID != title {
		t.Fatalf("title match must rank first: %v", searchTestTitles(res))
	}
	var bodyHit store.SearchHit
	for _, h := range res.Hits {
		if h.Page.Title == "Operations handbook" {
			bodyHit = h
		}
	}
	if !strings.Contains(bodyHit.Snippet, "<mark>database</mark>") {
		t.Fatalf("snippet %q", bodyHit.Snippet)
	}
	if bodyHit.SpaceKey != "PUB" || bodyHit.Author == nil || bodyHit.Author.ID != w.owner.ID || bodyHit.UpdatedAt.IsZero() {
		t.Fatalf("hit %+v", bodyHit)
	}
	if bodyHit.Page.Status != domain.PageStatusPublished {
		t.Fatalf("status %v", bodyHit.Page.Status)
	}
}

func TestSearchPagesWebsearchSyntax(t *testing.T) {
	w := newSearchWorld(t)
	searchTestPage(t, w.s, w.pub, w.owner.ID, "One", "red apples and green pears", "published")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Two", "red cherries", "published")

	if res := w.search(w.outsider, "red -apples"); res.Total != 1 || res.Hits[0].Page.Title != "Two" {
		t.Errorf("negation: %v", searchTestTitles(res))
	}
	if res := w.search(w.outsider, `"red apples"`); res.Total != 1 || res.Hits[0].Page.Title != "One" {
		t.Errorf("phrase: %v", searchTestTitles(res))
	}
	if res := w.search(w.outsider, "apples or cherries"); res.Total != 2 {
		t.Errorf("or: %v", searchTestTitles(res))
	}
	for _, q := range []string{"the", "!!!", "'", "a & | b", "(("} {
		if _, err := w.s.SearchPages(context.Background(), search.Params{Q: q, UserID: w.outsider.ID}); err != nil {
			t.Errorf("q=%q errored: %v", q, err)
		}
	}
}

func TestSearchPagesPagination(t *testing.T) {
	w := newSearchWorld(t)
	for i := 0; i < 7; i++ {
		id := searchTestPage(t, w.s, w.pub, w.owner.ID, "Runbook "+string(rune('A'+i)), "incident response", "published")
		searchTestAge(t, w.s, id, time.Duration(i)*time.Hour)
	}
	seen := map[uuid.UUID]bool{}
	for page, wantLen := range map[int]int{1: 3, 2: 3, 3: 1, 4: 0} {
		res := w.searchWith(search.Params{Q: "incident", Page: page, Limit: 3, UserID: w.outsider.ID})
		if res.Total != 7 || res.Page != page || res.Limit != 3 || len(res.Hits) != wantLen {
			t.Fatalf("page %d: total=%d page=%d limit=%d hits=%d", page, res.Total, res.Page, res.Limit, len(res.Hits))
		}
		for _, h := range res.Hits {
			if seen[h.Page.ID] {
				t.Fatalf("page %d repeated %s", page, h.Page.Title)
			}
			seen[h.Page.ID] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("saw %d pages", len(seen))
	}
}

func TestSearchPagesHasChildrenAndEmpty(t *testing.T) {
	w := newSearchWorld(t)
	parent := searchTestPage(t, w.s, w.pub, w.owner.ID, "Parent", "tree", "published")
	child := searchTestPage(t, w.s, w.pub, w.owner.ID, "Child", "leaf", "published")
	if _, err := w.s.Pool.Exec(context.Background(), `UPDATE pages SET parent_id = $2 WHERE id = $1`, child, parent); err != nil {
		t.Fatal(err)
	}
	res := w.search(w.outsider, "tree")
	if len(res.Hits) != 1 || !res.Hits[0].Page.HasChildren {
		t.Fatalf("%+v", res)
	}
	res = w.search(w.outsider, "nonexistentterm")
	if res.Total != 0 || res.Hits == nil || len(res.Hits) != 0 || res.Page != 1 || res.Limit != 20 {
		t.Fatalf("%+v", res)
	}
	if _, err := w.s.SearchPages(context.Background(), search.Params{UserID: w.outsider.ID}); err == nil {
		t.Fatal("empty query must error")
	}
}

func TestQuickSearchPages(t *testing.T) {
	w := newSearchWorld(t)
	ctx := context.Background()
	contains := searchTestPage(t, w.s, w.pub, w.owner.ID, "The Deploy Guide", "", "published")
	prefixOld := searchTestPage(t, w.s, w.pub, w.owner.ID, "Deploy checklist", "", "published")
	prefixNew := searchTestPage(t, w.s, w.pub, w.owner.ID, "deployment notes", "", "published")
	searchTestAge(t, w.s, prefixOld, 48*time.Hour)
	searchTestAge(t, w.s, prefixNew, 1*time.Hour)
	searchTestAge(t, w.s, contains, 0)
	searchTestPage(t, w.s, w.priv, w.owner.ID, "Deploy secrets", "", "published")
	searchTestPage(t, w.s, w.gone, w.owner.ID, "Deploy ghost", "", "published")
	searchTestPage(t, w.s, w.archived, w.owner.ID, "Deploy archive", "", "published")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Deploy trashed", "", "trashed")
	ownDraft := searchTestPage(t, w.s, w.pub, w.outsider.ID, "Deploy my draft", "", "draft")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Deploy their draft", "", "draft")

	hits, err := w.s.QuickSearchPages(ctx, search.QuickParams{Q: "deploy", UserID: w.outsider.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, h := range hits {
		titles = append(titles, h.Page.Title)
	}
	want := []string{"deployment notes", "Deploy my draft", "Deploy archive", "Deploy checklist", "The Deploy Guide"}
	if len(titles) != len(want) {
		t.Fatalf("titles %v", titles)
	}
	prefixCount := 0
	for _, title := range titles {
		if strings.HasPrefix(strings.ToLower(title), "deploy") {
			prefixCount++
		}
	}
	if prefixCount != 4 || !strings.HasPrefix(strings.ToLower(titles[0]), "deploy") || titles[len(titles)-1] != "The Deploy Guide" {
		t.Fatalf("prefix matches must come first: %v", titles)
	}
	var idx = map[string]int{}
	for i, title := range titles {
		idx[title] = i
	}
	if !(idx["deployment notes"] < idx["Deploy checklist"]) {
		t.Fatalf("recency inside the prefix group: %v", titles)
	}
	for _, h := range hits {
		if h.Page.ID == ownDraft && (h.Page.Status != domain.PageStatusDraft || h.SpaceKey != "PUB" || h.SpaceName != "PUB") {
			t.Fatalf("draft hit %+v", h)
		}
		if strings.Contains(h.Page.Title, "secrets") || strings.Contains(h.Page.Title, "ghost") || strings.Contains(h.Page.Title, "trashed") || strings.Contains(h.Page.Title, "their") {
			t.Fatalf("leaked %q", h.Page.Title)
		}
	}

	hits, err = w.s.QuickSearchPages(ctx, search.QuickParams{Q: "DEPLOY", UserID: w.member.ID, Limit: 2})
	if err != nil || len(hits) != 2 {
		t.Fatalf("limit: %v %d", err, len(hits))
	}
	hits, err = w.s.QuickSearchPages(ctx, search.QuickParams{Q: "deploy", UserID: w.admin.ID, IsAdmin: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if strings.Contains(h.Page.Title, "ghost") {
			t.Fatal("deleted space leaked to admin")
		}
	}
}

func TestQuickSearchWildcardsAndEmpty(t *testing.T) {
	w := newSearchWorld(t)
	ctx := context.Background()
	searchTestPage(t, w.s, w.pub, w.owner.ID, "100% coverage", "", "published")
	searchTestPage(t, w.s, w.pub, w.owner.ID, "plain page", "", "published")

	hits, err := w.s.QuickSearchPages(ctx, search.QuickParams{Q: "%", UserID: w.outsider.ID})
	if err != nil || len(hits) != 1 || hits[0].Page.Title != "100% coverage" {
		t.Fatalf("percent: %v %+v", err, hits)
	}
	hits, err = w.s.QuickSearchPages(ctx, search.QuickParams{Q: "_", UserID: w.outsider.ID})
	if err != nil || len(hits) != 0 {
		t.Fatalf("underscore: %v %+v", err, hits)
	}
	hits, err = w.s.QuickSearchPages(ctx, search.QuickParams{UserID: w.outsider.ID})
	if err != nil || len(hits) != 2 {
		t.Fatalf("empty q: %v %+v", err, hits)
	}
	hits, err = w.s.QuickSearchPages(ctx, search.QuickParams{Q: "zzz", UserID: w.outsider.ID})
	if err != nil || hits == nil || len(hits) != 0 {
		t.Fatalf("no match: %v %+v", err, hits)
	}
}

func TestSearchPagesSnippetIsHTMLSafe(t *testing.T) {
	w := newSearchWorld(t)
	searchTestPage(t, w.s, w.pub, w.owner.ID, "Payload", "Before <img src=x onerror=alert(1)> the widget is broken & <script>alert(2)</script> after widget text.", "published")
	res := w.search(w.outsider, "widget")
	if len(res.Hits) != 1 {
		t.Fatalf("hits %d", len(res.Hits))
	}
	snippet := res.Hits[0].Snippet
	if strings.Contains(snippet, "<img") || strings.Contains(snippet, "<script") {
		t.Fatalf("raw markup in snippet: %q", snippet)
	}
	if !strings.Contains(snippet, "&lt;img") || !strings.Contains(snippet, "&amp;") {
		t.Fatalf("expected escaped markup: %q", snippet)
	}
	if !strings.Contains(snippet, "<mark>widget</mark>") {
		t.Fatalf("match not wrapped: %q", snippet)
	}
	stripped := strings.NewReplacer("<mark>", "", "</mark>", "").Replace(snippet)
	if strings.Contains(stripped, "<") || strings.Contains(stripped, ">") {
		t.Fatalf("only mark tags may remain: %q", snippet)
	}
}
