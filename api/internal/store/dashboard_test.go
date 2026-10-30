package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type dashFixture struct {
	t        *testing.T
	s        *store.Store
	ctx      context.Context
	me       domain.User
	author   domain.User
	admin    domain.User
	internal uuid.UUID
	private  uuid.UUID
	member   uuid.UUID
	deleted  uuid.UUID
}

func newDashFixture(t *testing.T) *dashFixture {
	t.Helper()
	s := testutil.NewDB(t)
	f := &dashFixture{t: t, s: s, ctx: context.Background()}
	f.me = testutil.CreateUser(t, s, "me@example.test", "", false)
	f.author = testutil.CreateUser(t, s, "author@example.test", "", false)
	f.admin = testutil.CreateUser(t, s, "root@example.test", "", true)
	f.internal = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "OPEN", Visibility: domain.VisibilityInternal, CreatedBy: f.author.ID})
	f.private = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "SECRET", Visibility: domain.VisibilityPrivate, CreatedBy: f.author.ID})
	f.member = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "TEAM", Visibility: domain.VisibilityPrivate, CreatedBy: f.author.ID})
	f.deleted = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "GONE", Visibility: domain.VisibilityInternal, Deleted: true, CreatedBy: f.author.ID})
	testutil.AddMember(t, s, f.member, f.me.ID, domain.RoleViewer)
	return f
}

func (f *dashFixture) page(space uuid.UUID, createdBy uuid.UUID, title string, age time.Duration) uuid.UUID {
	f.t.Helper()
	id := testutil.CreatePage(f.t, f.s, space, createdBy, title)
	f.exec(`UPDATE pages SET updated_at = now() - $1::interval, updated_by = $2 WHERE id = $3`, fmt.Sprintf("%d seconds", int(age.Seconds())), createdBy, id)
	return id
}

func (f *dashFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.s.Pool.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("exec: %v", err)
	}
}

func (f *dashFixture) dash(u domain.User) store.Dashboard {
	f.t.Helper()
	d, err := f.s.GetDashboard(f.ctx, u.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return d
}

func dashTitles(list []store.DashboardPage) []string {
	out := []string{}
	for _, p := range list {
		out = append(out, p.Title)
	}
	return out
}

func dashSameStrings(a, b []string) bool {
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

func TestDashboardRecentUpdatesAccessRules(t *testing.T) {
	f := newDashFixture(t)
	f.page(f.internal, f.author.ID, "old open", 3*time.Hour)
	f.page(f.internal, f.author.ID, "new open", time.Hour)
	f.page(f.member, f.author.ID, "team page", 2*time.Hour)
	f.page(f.private, f.author.ID, "secret page", time.Minute)
	f.page(f.deleted, f.author.ID, "deleted space page", time.Minute)
	trashed := f.page(f.internal, f.author.ID, "trashed", time.Minute)
	f.exec(`UPDATE pages SET status = 'trashed' WHERE id = $1`, trashed)
	draft := f.page(f.internal, f.author.ID, "someone else's draft", time.Minute)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, draft)

	d := f.dash(f.me)
	if got := dashTitles(d.RecentUpdates); !dashSameStrings(got, []string{"new open", "team page", "old open"}) {
		t.Fatalf("recent: %v", got)
	}
	first := d.RecentUpdates[0]
	if first.SpaceKey != "OPEN" || first.SpaceName != "OPEN" || first.Author == nil || first.Author.ID != f.author.ID || first.Status != domain.PageStatusPublished {
		t.Fatalf("fields: %+v", first)
	}
	if d.RecentlyViewed == nil || d.MyDrafts == nil || d.Starred == nil {
		t.Fatal("sections must never be nil")
	}

	root := f.dash(f.admin)
	if got := dashTitles(root.RecentUpdates); !dashSameStrings(got, []string{"secret page", "new open", "team page", "old open"}) {
		t.Fatalf("system admin recent: %v", got)
	}
}

func TestDashboardRecentUpdatesLimit(t *testing.T) {
	f := newDashFixture(t)
	for i := 0; i < 25; i++ {
		f.page(f.internal, f.author.ID, fmt.Sprintf("p%02d", i), time.Duration(i+1)*time.Minute)
	}
	d := f.dash(f.me)
	if len(d.RecentUpdates) != 20 || d.RecentUpdates[0].Title != "p00" {
		t.Fatalf("recent: %d %s", len(d.RecentUpdates), d.RecentUpdates[0].Title)
	}
}

func TestDashboardViewedAndStarred(t *testing.T) {
	f := newDashFixture(t)
	a := f.page(f.internal, f.author.ID, "a", time.Hour)
	b := f.page(f.internal, f.author.ID, "b", time.Hour)
	c := f.page(f.internal, f.author.ID, "c", time.Hour)
	hidden := f.page(f.private, f.author.ID, "hidden", time.Hour)
	trashed := f.page(f.internal, f.author.ID, "trashed", time.Hour)
	f.exec(`UPDATE pages SET status = 'trashed' WHERE id = $1`, trashed)
	draft := f.page(f.internal, f.author.ID, "their draft", time.Hour)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, draft)

	at := time.Now().UTC()
	for i, id := range []uuid.UUID{a, b, c, hidden, trashed, draft} {
		f.exec(`INSERT INTO page_views (user_id, page_id, viewed_at) VALUES ($1, $2, $3)`, f.me.ID, id, at.Add(time.Duration(i)*time.Minute))
		f.exec(`INSERT INTO page_stars (user_id, page_id, created_at) VALUES ($1, $2, $3)`, f.me.ID, id, at.Add(-time.Duration(i)*time.Minute))
	}
	f.exec(`INSERT INTO page_views (user_id, page_id) VALUES ($1, $2)`, f.author.ID, a)

	d := f.dash(f.me)
	if got := dashTitles(d.RecentlyViewed); !dashSameStrings(got, []string{"c", "b", "a"}) {
		t.Fatalf("viewed: %v", got)
	}
	if got := dashTitles(d.Starred); !dashSameStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("starred: %v", got)
	}
	if other := f.dash(f.author); len(other.RecentlyViewed) != 1 || len(other.Starred) != 0 {
		t.Fatalf("other user: %+v", other)
	}
}

func TestDashboardViewedLimit(t *testing.T) {
	f := newDashFixture(t)
	for i := 0; i < 12; i++ {
		id := f.page(f.internal, f.author.ID, fmt.Sprintf("v%02d", i), time.Hour)
		f.exec(`INSERT INTO page_views (user_id, page_id, viewed_at) VALUES ($1, $2, now() + $3::interval)`, f.me.ID, id, fmt.Sprintf("%d seconds", i))
	}
	d := f.dash(f.me)
	if len(d.RecentlyViewed) != 10 || d.RecentlyViewed[0].Title != "v11" {
		t.Fatalf("viewed: %v", dashTitles(d.RecentlyViewed))
	}
}

func TestDashboardMyDrafts(t *testing.T) {
	f := newDashFixture(t)
	own := f.page(f.internal, f.me.ID, "my unpublished", 2*time.Hour)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, own)
	editing := f.page(f.internal, f.author.ID, "editing published", 5*time.Hour)
	f.exec(`INSERT INTO page_drafts (page_id, user_id, title, updated_at) VALUES ($1, $2, 'wip', now() - interval '10 minutes')`, editing, f.me.ID)
	both := f.page(f.internal, f.me.ID, "both", 5*time.Hour)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, both)
	f.exec(`INSERT INTO page_drafts (page_id, user_id, title, updated_at) VALUES ($1, $2, 'wip', now() - interval '3 hours')`, both, f.me.ID)
	theirs := f.page(f.internal, f.author.ID, "their unpublished", time.Hour)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, theirs)
	otherDraft := f.page(f.internal, f.author.ID, "someone else editing", time.Hour)
	f.exec(`INSERT INTO page_drafts (page_id, user_id, title) VALUES ($1, $2, 'x')`, otherDraft, f.author.ID)
	trashed := f.page(f.internal, f.me.ID, "trashed draft", time.Hour)
	f.exec(`UPDATE pages SET status = 'trashed' WHERE id = $1`, trashed)
	lost := f.page(f.private, f.me.ID, "no longer accessible", time.Hour)
	f.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, lost)

	d := f.dash(f.me)
	if got := dashTitles(d.MyDrafts); !dashSameStrings(got, []string{"editing published", "my unpublished", "both"}) {
		t.Fatalf("drafts: %v", got)
	}
	for _, p := range d.MyDrafts {
		if p.Author == nil || p.Author.ID != f.me.ID {
			t.Fatalf("author: %+v", p.Author)
		}
	}
	if d.MyDrafts[0].Status != domain.PageStatusPublished || d.MyDrafts[1].Status != domain.PageStatusDraft {
		t.Fatalf("statuses: %+v", d.MyDrafts)
	}
	if time.Since(d.MyDrafts[0].UpdatedAt) > time.Hour {
		t.Fatalf("updated_at should come from the draft row: %v", d.MyDrafts[0].UpdatedAt)
	}
}

func TestDashboardHasChildren(t *testing.T) {
	f := newDashFixture(t)
	parent := f.page(f.internal, f.author.ID, "parent", time.Hour)
	child := f.page(f.internal, f.author.ID, "child", 2*time.Hour)
	f.exec(`UPDATE pages SET parent_id = $1 WHERE id = $2`, parent, child)
	d := f.dash(f.me)
	for _, p := range d.RecentUpdates {
		if p.Title == "parent" && !p.HasChildren {
			t.Fatal("parent should report has_children")
		}
		if p.Title == "child" && p.HasChildren {
			t.Fatal("child should not")
		}
	}
}
