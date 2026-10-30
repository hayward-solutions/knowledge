package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type pagesFx struct {
	t     *testing.T
	s     *store.Store
	ctx   context.Context
	user  domain.User
	space uuid.UUID
}

func newPagesFx(t *testing.T) *pagesFx {
	t.Helper()
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "author@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: u.ID})
	return &pagesFx{t: t, s: s, ctx: context.Background(), user: u, space: space}
}

func (f *pagesFx) newPage(parent *uuid.UUID, title string) domain.Page {
	f.t.Helper()
	p, err := f.s.CreatePage(f.ctx, store.NewPage{SpaceID: f.space, ParentID: parent, Title: title, CreatedBy: f.user.ID})
	if err != nil {
		f.t.Fatalf("create page %q: %v", title, err)
	}
	return p
}

func (f *pagesFx) publish(id uuid.UUID, title string, base int) domain.Page {
	f.t.Helper()
	p, err := f.s.PublishPage(f.ctx, store.PublishParams{
		PageID: id, ActorID: f.user.ID, Title: title, ContentHTML: "<p>" + title + "</p>", BaseVersion: base,
	})
	if err != nil {
		f.t.Fatalf("publish %q: %v", title, err)
	}
	return p
}

func (f *pagesFx) setHome(id uuid.UUID) {
	f.t.Helper()
	if _, err := f.s.Pool.Exec(f.ctx, `UPDATE spaces SET home_page_id = $2 WHERE id = $1`, f.space, id); err != nil {
		f.t.Fatal(err)
	}
}

func (f *pagesFx) children(parent uuid.UUID) []string {
	f.t.Helper()
	list, err := f.s.ListChildren(f.ctx, parent, f.user.ID, true)
	if err != nil {
		f.t.Fatal(err)
	}
	out := []string{}
	for i, c := range list {
		if c.Position != i {
			f.t.Fatalf("positions must be contiguous: %+v", list)
		}
		out = append(out, c.Title)
	}
	return out
}

func sameStrings(a, b []string) bool {
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

func TestPageCreateAssignsPositionAndSlug(t *testing.T) {
	f := newPagesFx(t)
	a := f.newPage(nil, "Hello, World!")
	b := f.newPage(nil, "Second")
	if a.Status != domain.PageStatusDraft || a.Version != 0 || a.Slug != "hello-world" || a.Position != 0 || b.Position != 1 {
		t.Fatalf("unexpected: %+v %+v", a, b)
	}
	if a.PublishedAt != nil || a.CreatedBy != f.user.ID || a.UpdatedBy != f.user.ID {
		t.Fatalf("unexpected: %+v", a)
	}
	c := f.newPage(&a.ID, "Child one")
	d := f.newPage(&a.ID, "Child two")
	if c.Position != 0 || d.Position != 1 || c.ParentID == nil || *c.ParentID != a.ID {
		t.Fatalf("children: %+v %+v", c, d)
	}

	other := testutil.CreateSpace(t, f.s, testutil.SpaceOpts{Key: "OPS", CreatedBy: f.user.ID})
	foreign := testutil.CreatePage(t, f.s, other, f.user.ID, "Foreign")
	missing := uuid.Must(uuid.NewV7())
	for name, parent := range map[string]*uuid.UUID{"foreign": &foreign, "missing": &missing} {
		_, err := f.s.CreatePage(f.ctx, store.NewPage{SpaceID: f.space, ParentID: parent, Title: "x", CreatedBy: f.user.ID})
		if !errors.Is(err, store.ErrInvalidParent) {
			t.Errorf("%s parent: %v", name, err)
		}
	}
	if err := f.s.TrashPage(f.ctx, d.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreatePage(f.ctx, store.NewPage{SpaceID: f.space, ParentID: &d.ID, Title: "x", CreatedBy: f.user.ID}); !errors.Is(err, store.ErrInvalidParent) {
		t.Errorf("trashed parent: %v", err)
	}
}

func TestPageCreateStoresContent(t *testing.T) {
	f := newPagesFx(t)
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`)
	p, err := f.s.CreatePage(f.ctx, store.NewPage{
		SpaceID: f.space, Title: "T", ContentJSON: doc, ContentHTML: "<p>hi</p>", ContentText: "hi", CreatedBy: f.user.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(p.ContentJSON, &got); err != nil || got["type"] != "doc" {
		t.Fatalf("content_json: %s %v", p.ContentJSON, err)
	}
	if p.ContentHTML != "<p>hi</p>" || p.ContentText != "hi" {
		t.Fatalf("content: %+v", p)
	}
	bare := f.newPage(nil, "Bare")
	if len(bare.ContentJSON) != 0 {
		t.Fatalf("nil content_json must stay NULL: %q", bare.ContentJSON)
	}
}

func TestPagePublishVersioning(t *testing.T) {
	f := newPagesFx(t)
	p := f.newPage(nil, "Draft title")
	if _, err := f.s.UpsertDraft(f.ctx, store.DraftParams{PageID: p.ID, UserID: f.user.ID, Title: "wip"}); err != nil {
		t.Fatal(err)
	}

	msg := "first"
	doc := json.RawMessage(`{"type":"doc","content":[]}`)
	v1, err := f.s.PublishPage(f.ctx, store.PublishParams{
		PageID: p.ID, ActorID: f.user.ID, Title: "Published", ContentJSON: doc, ContentHTML: "<p>a</p>",
		ContentText: "a", BaseVersion: 0, Message: &msg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v1.Status != domain.PageStatusPublished || v1.PublishedAt == nil || v1.Slug != "published" || v1.ContentText != "a" {
		t.Fatalf("v1: %+v", v1)
	}
	if ref, err := f.s.GetDraftRef(f.ctx, p.ID, f.user.ID); err != nil || ref != nil {
		t.Fatalf("draft must be deleted: %v %v", ref, err)
	}

	v2 := f.publish(p.ID, "Second", 1)
	if v2.Version != 2 || !v2.PublishedAt.Equal(*v1.PublishedAt) {
		t.Fatalf("v2: %+v", v2)
	}

	_, err = f.s.PublishPage(f.ctx, store.PublishParams{PageID: p.ID, ActorID: f.user.ID, Title: "Stale", BaseVersion: 1})
	var conflict *store.VersionConflictError
	if !errors.As(err, &conflict) || conflict.Current != 2 {
		t.Fatalf("expected version conflict: %v", err)
	}
	if got, _ := f.s.GetPage(f.ctx, p.ID); got.Title != "Second" || got.Version != 2 {
		t.Fatalf("conflict must not write: %+v", got)
	}

	forced, err := f.s.PublishPage(f.ctx, store.PublishParams{PageID: p.ID, ActorID: f.user.ID, Title: "Forced", BaseVersion: 0, Force: true})
	if err != nil || forced.Version != 3 {
		t.Fatalf("force: %v %+v", err, forced)
	}

	versions, err := f.s.ListPageVersions(f.ctx, p.ID)
	if err != nil || len(versions) != 3 || versions[0].Version != 3 || versions[2].Version != 1 {
		t.Fatalf("versions: %v %+v", err, versions)
	}
	if versions[2].Message == nil || *versions[2].Message != "first" || versions[2].Author == nil || versions[2].Author.ID != f.user.ID {
		t.Fatalf("version 1: %+v", versions[2])
	}
	full, err := f.s.GetPageVersion(f.ctx, p.ID, 1)
	if err != nil || full.Title != "Published" || full.ContentHTML != "<p>a</p>" || full.Author == nil {
		t.Fatalf("get version: %v %+v", err, full)
	}
	if _, err := f.s.GetPageVersion(f.ctx, p.ID, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}

	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.PublishPage(f.ctx, store.PublishParams{PageID: p.ID, ActorID: f.user.ID, Title: "x", BaseVersion: 3, Force: true}); !errors.Is(err, store.ErrPageTrashed) {
		t.Fatalf("trashed publish: %v", err)
	}
}

func TestPageUpdateTitle(t *testing.T) {
	f := newPagesFx(t)
	p := f.publish(f.newPage(nil, "Old").ID, "Old", 0)
	got, err := f.s.UpdatePageTitle(f.ctx, p.ID, f.user.ID, "New Title")
	if err != nil || got.Title != "New Title" || got.Slug != "new-title" || got.Version != p.Version {
		t.Fatalf("update: %v %+v", err, got)
	}
	versions, _ := f.s.ListPageVersions(f.ctx, p.ID)
	if len(versions) != 1 {
		t.Fatalf("title change must not create a version: %+v", versions)
	}
	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.UpdatePageTitle(f.ctx, p.ID, f.user.ID, "x"); !errors.Is(err, store.ErrPageTrashed) {
		t.Fatalf("trashed: %v", err)
	}
	if _, err := f.s.UpdatePageTitle(f.ctx, uuid.Must(uuid.NewV7()), f.user.ID, "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestPageMove(t *testing.T) {
	f := newPagesFx(t)
	root := f.newPage(nil, "Root")
	a := f.newPage(&root.ID, "A")
	b := f.newPage(&root.ID, "B")
	c := f.newPage(&root.ID, "C")
	other := f.newPage(nil, "Other")

	pos := 0
	res, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: c.ID, ParentID: &root.ID, Position: &pos})
	if err != nil || res.Page.Position != 0 || res.OldParentID == nil || *res.OldParentID != root.ID {
		t.Fatalf("move to front: %v %+v", err, res)
	}
	if got := f.children(root.ID); !sameStrings(got, []string{"C", "A", "B"}) {
		t.Fatalf("order: %v", got)
	}

	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: c.ID, ParentID: &root.ID, AfterID: &b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := f.children(root.ID); !sameStrings(got, []string{"A", "B", "C"}) {
		t.Fatalf("after b: %v", got)
	}

	big := 99
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &root.ID, Position: &big}); err != nil {
		t.Fatal(err)
	}
	if got := f.children(root.ID); !sameStrings(got, []string{"B", "C", "A"}) {
		t.Fatalf("clamped: %v", got)
	}

	res, err = f.s.MovePage(f.ctx, store.MoveParams{PageID: b.ID, ParentID: &other.ID})
	if err != nil || res.Page.ParentID == nil || *res.Page.ParentID != other.ID || res.Page.Position != 0 {
		t.Fatalf("reparent: %v %+v", err, res)
	}
	if got := f.children(root.ID); !sameStrings(got, []string{"C", "A"}) {
		t.Fatalf("old parent must be renumbered: %v", got)
	}

	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: b.ID, ParentID: nil}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.s.GetPage(f.ctx, b.ID)
	if got.ParentID != nil || got.Position != 2 {
		t.Fatalf("to root: %+v", got)
	}

	grand := f.newPage(&a.ID, "Grand")
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &grand.ID}); !errors.Is(err, store.ErrPageCycle) {
		t.Fatalf("descendant cycle: %v", err)
	}
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &a.ID}); !errors.Is(err, store.ErrPageCycle) {
		t.Fatalf("self cycle: %v", err)
	}

	otherSpace := testutil.CreateSpace(t, f.s, testutil.SpaceOpts{Key: "OPS", CreatedBy: f.user.ID})
	foreign := testutil.CreatePage(t, f.s, otherSpace, f.user.ID, "Foreign")
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &foreign}); !errors.Is(err, store.ErrInvalidParent) {
		t.Fatalf("cross-space: %v", err)
	}
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &root.ID, AfterID: &foreign}); !errors.Is(err, store.ErrInvalidAnchor) {
		t.Fatalf("bad anchor: %v", err)
	}
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &root.ID, AfterID: &a.ID}); !errors.Is(err, store.ErrInvalidAnchor) {
		t.Fatalf("self anchor: %v", err)
	}

	if err := f.s.TrashPage(f.ctx, other.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: a.ID, ParentID: &other.ID}); !errors.Is(err, store.ErrInvalidParent) {
		t.Fatalf("trashed target: %v", err)
	}
	if _, err := f.s.MovePage(f.ctx, store.MoveParams{PageID: other.ID, ParentID: &root.ID}); !errors.Is(err, store.ErrPageTrashed) {
		t.Fatalf("trashed source: %v", err)
	}
}

func TestPageTrashCascadesAndRestoreReattaches(t *testing.T) {
	f := newPagesFx(t)
	home := f.publish(f.newPage(nil, "Home").ID, "Home", 0)
	f.setHome(home.ID)
	p := f.publish(f.newPage(&home.ID, "P").ID, "P", 0)
	c := f.publish(f.newPage(&p.ID, "C").ID, "C", 0)
	g := f.newPage(&c.ID, "G")
	early := f.publish(f.newPage(&p.ID, "Early").ID, "Early", 0)

	if err := f.s.TrashPage(f.ctx, early.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	earlyAfter, _ := f.s.GetPage(f.ctx, early.ID)
	time.Sleep(5 * time.Millisecond)

	if err := f.s.TrashPage(f.ctx, home.ID, f.user.ID); !errors.Is(err, store.ErrHomePage) {
		t.Fatalf("home: %v", err)
	}
	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); !errors.Is(err, store.ErrPageTrashed) {
		t.Fatalf("twice: %v", err)
	}
	pp, _ := f.s.GetPage(f.ctx, p.ID)
	cc, _ := f.s.GetPage(f.ctx, c.ID)
	gg, _ := f.s.GetPage(f.ctx, g.ID)
	for _, x := range []domain.Page{pp, cc, gg} {
		if x.Status != domain.PageStatusTrashed || x.TrashedAt == nil || x.TrashedBy == nil || *x.TrashedBy != f.user.ID {
			t.Fatalf("not trashed: %+v", x)
		}
		if !x.TrashedAt.Equal(*pp.TrashedAt) {
			t.Fatalf("subtree must share trashed_at: %v vs %v", x.TrashedAt, pp.TrashedAt)
		}
	}
	earlyNow, _ := f.s.GetPage(f.ctx, early.ID)
	if earlyNow.TrashedAt == nil || !earlyNow.TrashedAt.Equal(*earlyAfter.TrashedAt) {
		t.Fatalf("earlier trash must keep its timestamp: %+v", earlyNow)
	}
	if got := f.children(home.ID); len(got) != 0 {
		t.Fatalf("home children: %v", got)
	}

	restoredC, err := f.s.RestorePage(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredC.Status != domain.PageStatusPublished || restoredC.ParentID == nil || *restoredC.ParentID != home.ID || restoredC.TrashedAt != nil || restoredC.TrashedBy != nil {
		t.Fatalf("reattach to home: %+v", restoredC)
	}
	gg, _ = f.s.GetPage(f.ctx, g.ID)
	if gg.Status != domain.PageStatusDraft || gg.TrashedAt != nil {
		t.Fatalf("version 0 descendant must restore as draft: %+v", gg)
	}
	pp, _ = f.s.GetPage(f.ctx, p.ID)
	if pp.Status != domain.PageStatusTrashed {
		t.Fatalf("parent must stay trashed: %+v", pp)
	}

	restoredP, err := f.s.RestorePage(f.ctx, p.ID)
	if err != nil || restoredP.Status != domain.PageStatusPublished || restoredP.ParentID == nil || *restoredP.ParentID != home.ID {
		t.Fatalf("restore p: %v %+v", err, restoredP)
	}
	earlyNow, _ = f.s.GetPage(f.ctx, early.ID)
	if earlyNow.Status != domain.PageStatusTrashed {
		t.Fatalf("separately trashed child must stay trashed: %+v", earlyNow)
	}
	if _, err := f.s.RestorePage(f.ctx, p.ID); !errors.Is(err, store.ErrNotTrashed) {
		t.Fatalf("restore live page: %v", err)
	}

	if err := f.s.TrashPage(f.ctx, c.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	back, err := f.s.RestorePage(f.ctx, c.ID)
	if err != nil || back.ParentID == nil || *back.ParentID != home.ID {
		t.Fatalf("restore to live parent: %v %+v", err, back)
	}
}

func TestPageRestoreWithoutHomeGoesToRoot(t *testing.T) {
	f := newPagesFx(t)
	p := f.newPage(nil, "P")
	c := f.newPage(&p.ID, "C")
	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.RestorePage(f.ctx, c.ID)
	if err != nil || got.ParentID != nil || got.Status != domain.PageStatusDraft {
		t.Fatalf("root fallback: %v %+v", err, got)
	}
}

func TestPagePurge(t *testing.T) {
	f := newPagesFx(t)
	p := f.publish(f.newPage(nil, "P").ID, "P", 0)
	c := f.newPage(&p.ID, "C")
	if _, err := f.s.UpsertDraft(f.ctx, store.DraftParams{PageID: c.ID, UserID: f.user.ID, Title: "d"}); err != nil {
		t.Fatal(err)
	}
	other := f.newPage(nil, "Other")
	for _, id := range []uuid.UUID{p.ID, c.ID, other.ID} {
		if err := f.s.SetWatch(f.ctx, f.user.ID, domain.WatchTargetPage, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.PurgePage(f.ctx, p.ID); !errors.Is(err, store.ErrNotTrashed) {
		t.Fatalf("live purge: %v", err)
	}
	if ok, _ := f.s.IsWatching(f.ctx, f.user.ID, domain.WatchTargetPage, p.ID); !ok {
		t.Fatal("rejected purge must keep watches")
	}
	if err := f.s.TrashPage(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.PurgePage(f.ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{p.ID, c.ID} {
		if ok, _ := f.s.IsWatching(f.ctx, f.user.ID, domain.WatchTargetPage, id); ok {
			t.Fatalf("watch on purged page %s must be removed", id)
		}
	}
	if ok, _ := f.s.IsWatching(f.ctx, f.user.ID, domain.WatchTargetPage, other.ID); !ok {
		t.Fatal("unrelated watch must remain")
	}
	for _, id := range []uuid.UUID{p.ID, c.ID} {
		if _, err := f.s.GetPage(f.ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("page %s must be gone: %v", id, err)
		}
	}
	var n int
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM page_versions WHERE page_id = $1`, p.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("versions must cascade: %d %v", n, err)
	}
	if err := f.s.PurgePage(f.ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing purge: %v", err)
	}
}

func TestPageDraftsStarsViews(t *testing.T) {
	f := newPagesFx(t)
	other := testutil.CreateUser(t, f.s, "other@example.test", "", false)
	p := f.newPage(nil, "P")

	if _, err := f.s.GetDraft(f.ctx, p.ID, f.user.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("no draft: %v", err)
	}
	doc := json.RawMessage(`{"type":"doc"}`)
	d1, err := f.s.UpsertDraft(f.ctx, store.DraftParams{PageID: p.ID, UserID: f.user.ID, Title: "one", ContentJSON: doc, ContentHTML: "<p>1</p>", BaseVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := f.s.UpsertDraft(f.ctx, store.DraftParams{PageID: p.ID, UserID: f.user.ID, Title: "two", BaseVersion: 3})
	if err != nil || d2.Title != "two" || d2.BaseVersion != 3 || len(d2.ContentJSON) != 0 || d2.UpdatedAt.Before(d1.UpdatedAt) {
		t.Fatalf("upsert: %v %+v", err, d2)
	}
	if _, err := f.s.UpsertDraft(f.ctx, store.DraftParams{PageID: p.ID, UserID: other.ID, Title: "theirs"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM page_drafts WHERE page_id = $1`, p.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("one row per user: %d %v", n, err)
	}
	ref, err := f.s.GetDraftRef(f.ctx, p.ID, f.user.ID)
	if err != nil || ref == nil || ref.BaseVersion != 3 {
		t.Fatalf("ref: %v %+v", err, ref)
	}
	if err := f.s.DeleteDraft(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteDraft(f.ctx, p.ID, f.user.ID); err != nil {
		t.Fatalf("delete is idempotent: %v", err)
	}
	if ref, _ := f.s.GetDraftRef(f.ctx, p.ID, other.ID); ref == nil {
		t.Fatal("other user's draft must survive")
	}

	if ok, _ := f.s.IsPageStarred(f.ctx, f.user.ID, p.ID); ok {
		t.Fatal("not starred yet")
	}
	for i := 0; i < 2; i++ {
		if err := f.s.StarPage(f.ctx, f.user.ID, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := f.s.IsPageStarred(f.ctx, f.user.ID, p.ID); !ok {
		t.Fatal("starred")
	}
	if ok, _ := f.s.IsPageStarred(f.ctx, other.ID, p.ID); ok {
		t.Fatal("stars are per user")
	}
	if err := f.s.UnstarPage(f.ctx, f.user.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := f.s.IsPageStarred(f.ctx, f.user.ID, p.ID); ok {
		t.Fatal("unstarred")
	}

	if err := f.s.RecordPageView(f.ctx, f.user.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	var first time.Time
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT viewed_at FROM page_views WHERE user_id = $1 AND page_id = $2`, f.user.ID, p.ID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := f.s.RecordPageView(f.ctx, f.user.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	var second time.Time
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT viewed_at FROM page_views WHERE user_id = $1 AND page_id = $2`, f.user.ID, p.ID).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if !second.After(first) {
		t.Fatalf("view must be bumped: %v %v", first, second)
	}
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM page_views WHERE page_id = $1`, p.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("one view row: %d %v", n, err)
	}
}

func TestPageAncestorsAndChildren(t *testing.T) {
	f := newPagesFx(t)
	other := testutil.CreateUser(t, f.s, "other@example.test", "", false)
	root := f.newPage(nil, "Root")
	a := f.newPage(&root.ID, "A")
	b := f.newPage(&a.ID, "B")
	c := f.newPage(&b.ID, "C")

	anc, err := f.s.ListAncestors(f.ctx, c.ID)
	if err != nil || len(anc) != 3 || anc[0].ID != root.ID || anc[1].ID != a.ID || anc[2].ID != b.ID {
		t.Fatalf("ancestors root-first: %v %+v", err, anc)
	}
	if !anc[0].HasChildren || anc[2].Slug != "b" {
		t.Fatalf("summary fields: %+v", anc)
	}
	if anc, err := f.s.ListAncestors(f.ctx, root.ID); err != nil || anc == nil || len(anc) != 0 {
		t.Fatalf("root has no ancestors: %v %#v", err, anc)
	}

	sum, err := f.s.GetPageSummary(f.ctx, c.ID)
	if err != nil || sum.HasChildren || sum.ParentID == nil || *sum.ParentID != b.ID {
		t.Fatalf("summary: %v %+v", err, sum)
	}
	if _, err := f.s.GetPageSummary(f.ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing summary: %v", err)
	}

	pub := f.publish(f.newPage(&root.ID, "Pub").ID, "Pub", 0)
	gone := f.newPage(&root.ID, "Gone")
	if err := f.s.TrashPage(f.ctx, gone.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	asOther, err := f.s.ListChildren(f.ctx, root.ID, other.ID, false)
	if err != nil || len(asOther) != 1 || asOther[0].ID != pub.ID {
		t.Fatalf("other must see only published: %v %+v", err, asOther)
	}
	asCreator, _ := f.s.ListChildren(f.ctx, root.ID, f.user.ID, false)
	if len(asCreator) != 2 || asCreator[0].ID != a.ID {
		t.Fatalf("creator sees own drafts in order: %+v", asCreator)
	}
	asAdmin, _ := f.s.ListChildren(f.ctx, root.ID, other.ID, true)
	if len(asAdmin) != 2 {
		t.Fatalf("admin sees drafts: %+v", asAdmin)
	}
	if kids, err := f.s.ListChildren(f.ctx, c.ID, f.user.ID, true); err != nil || kids == nil || len(kids) != 0 {
		t.Fatalf("leaf children must be []: %v %#v", err, kids)
	}
}

func TestIsPageVisible(t *testing.T) {
	creator, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	published := time.Now()
	draft := domain.Page{Status: domain.PageStatusDraft, CreatedBy: creator}
	live := domain.Page{Status: domain.PageStatusPublished, CreatedBy: creator}
	trashed := domain.Page{Status: domain.PageStatusTrashed, CreatedBy: creator, Version: 2, PublishedAt: &published}
	trashedDraft := domain.Page{Status: domain.PageStatusTrashed, CreatedBy: creator}
	cases := []struct {
		name string
		page domain.Page
		user uuid.UUID
		role domain.Role
		want bool
	}{
		{"draft creator", draft, creator, domain.RoleEditor, true},
		{"draft other editor", draft, other, domain.RoleEditor, false},
		{"draft other viewer", draft, other, domain.RoleViewer, false},
		{"draft admin", draft, other, domain.RoleAdmin, true},
		{"published viewer", live, other, domain.RoleViewer, true},
		{"trashed viewer", trashed, other, domain.RoleViewer, false},
		{"trashed creator viewer", trashed, creator, domain.RoleViewer, false},
		{"trashed editor", trashed, other, domain.RoleEditor, true},
		{"trashed admin", trashed, other, domain.RoleAdmin, true},
		{"trashed draft creator", trashedDraft, creator, domain.RoleEditor, true},
		{"trashed draft other editor", trashedDraft, other, domain.RoleEditor, false},
		{"trashed draft admin", trashedDraft, other, domain.RoleAdmin, true},
	}
	for _, c := range cases {
		if got := store.IsPageVisible(c.page, c.user, c.role); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestPageVisibleAncestors(t *testing.T) {
	f := newPagesFx(t)
	other := testutil.CreateUser(t, f.s, "other@example.test", "", false)
	root := f.publish(f.newPage(nil, "Root").ID, "Root", 0)
	hidden := f.newPage(&root.ID, "Hidden draft")
	child, err := f.s.CreatePage(f.ctx, store.NewPage{SpaceID: f.space, ParentID: &hidden.ID, Title: "Child", CreatedBy: f.user.ID})
	if err != nil {
		t.Fatal(err)
	}
	child = f.publish(child.ID, "Child", 0)

	ids := func(list []store.PageSummary) []uuid.UUID {
		out := []uuid.UUID{}
		for _, p := range list {
			out = append(out, p.ID)
		}
		return out
	}
	check := func(name string, viewer uuid.UUID, role domain.Role, want ...uuid.UUID) {
		t.Helper()
		got, err := f.s.ListVisibleAncestors(f.ctx, child.ID, viewer, role)
		if err != nil {
			t.Fatal(err)
		}
		g := ids(got)
		if len(g) != len(want) {
			t.Fatalf("%s: got %v want %v", name, g, want)
		}
		for i := range g {
			if g[i] != want[i] {
				t.Fatalf("%s: got %v want %v", name, g, want)
			}
		}
	}
	check("other editor skips hidden draft", other.ID, domain.RoleEditor, root.ID)
	check("creator sees draft", f.user.ID, domain.RoleEditor, root.ID, hidden.ID)
	check("admin sees draft", other.ID, domain.RoleAdmin, root.ID, hidden.ID)
	all, err := f.s.ListAncestors(f.ctx, child.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("unfiltered ancestors: %v %+v", err, all)
	}

	mid := f.publish(f.newPage(&root.ID, "Mid").ID, "Mid", 0)
	leaf := f.publish(f.newPage(&mid.ID, "Leaf").ID, "Leaf", 0)
	if err := f.s.TrashPage(f.ctx, mid.ID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := f.s.ListVisibleAncestors(f.ctx, leaf.ID, other.ID, domain.RoleViewer)
	if len(got) != 1 || got[0].ID != root.ID {
		t.Fatalf("viewer must not see trashed ancestor: %+v", got)
	}
	got, _ = f.s.ListVisibleAncestors(f.ctx, leaf.ID, other.ID, domain.RoleEditor)
	if len(got) != 2 || got[1].ID != mid.ID {
		t.Fatalf("editor sees trashed ancestor: %+v", got)
	}
}
