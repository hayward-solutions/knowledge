package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type commentFixture struct {
	s      *store.Store
	author domain.User
	other  domain.User
	space  uuid.UUID
	page   uuid.UUID
}

func newCommentFixture(t *testing.T) commentFixture {
	t.Helper()
	s := testutil.NewDB(t)
	author := testutil.CreateUser(t, s, "author@example.test", "", false)
	other := testutil.CreateUser(t, s, "other@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: author.ID})
	page := testutil.CreatePage(t, s, space, author.ID, "Page")
	return commentFixture{s: s, author: author, other: other, space: space, page: page}
}

func (f commentFixture) add(t *testing.T, authorID uuid.UUID, parent *uuid.UUID, html string, anchor json.RawMessage) domain.Comment {
	t.Helper()
	c, err := f.s.CreateComment(context.Background(), domain.Comment{
		ID:           uuid.Must(uuid.NewV7()),
		PageID:       f.page,
		ParentID:     parent,
		AuthorID:     authorID,
		BodyJSON:     json.RawMessage(`{"type":"doc","content":[]}`),
		BodyHTML:     html,
		InlineAnchor: anchor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return c
}

func TestCommentThreadTree(t *testing.T) {
	f := newCommentFixture(t)
	ctx := context.Background()
	root := f.add(t, f.author.ID, nil, "<p>root</p>", json.RawMessage(`{"quoted_text":"hello"}`))
	r1 := f.add(t, f.other.ID, &root.ID, "<p>r1</p>", nil)
	r2 := f.add(t, f.author.ID, &root.ID, "<p>r2</p>", nil)
	deep := f.add(t, f.other.ID, &r1.ID, "<p>deep</p>", nil)
	second := f.add(t, f.other.ID, nil, "<p>second</p>", nil)

	tree, err := f.s.ListPageComments(ctx, f.page)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || tree[0].ID != root.ID || tree[1].ID != second.ID {
		t.Fatalf("roots: %+v", tree)
	}
	if len(tree[0].Replies) != 2 || tree[0].Replies[0].ID != r1.ID || tree[0].Replies[1].ID != r2.ID {
		t.Fatalf("replies order: %+v", tree[0].Replies)
	}
	if len(tree[0].Replies[0].Replies) != 1 || tree[0].Replies[0].Replies[0].ID != deep.ID {
		t.Fatalf("deep nesting: %+v", tree[0].Replies[0].Replies)
	}
	if tree[1].Replies == nil || len(tree[1].Replies) != 0 {
		t.Fatalf("leaf replies must be an empty slice: %#v", tree[1].Replies)
	}
	if tree[0].Author == nil || tree[0].Author.ID != f.author.ID || tree[0].Author.Email != "author@example.test" {
		t.Fatalf("author: %+v", tree[0].Author)
	}
	if string(tree[0].InlineAnchor) != `{"quoted_text": "hello"}` && string(tree[0].InlineAnchor) != `{"quoted_text":"hello"}` {
		t.Fatalf("anchor: %s", tree[0].InlineAnchor)
	}
	if tree[1].InlineAnchor != nil {
		t.Fatalf("anchor should be nil: %s", tree[1].InlineAnchor)
	}

	view, err := f.s.GetCommentView(ctx, r1.ID)
	if err != nil || len(view.Replies) != 1 || view.Author == nil {
		t.Fatalf("view: %+v %v", view, err)
	}
}

func TestCommentSoftDeleteKeepsShape(t *testing.T) {
	f := newCommentFixture(t)
	ctx := context.Background()
	root := f.add(t, f.author.ID, nil, "<p>root</p>", nil)
	reply := f.add(t, f.other.ID, &root.ID, "<p>reply</p>", nil)
	lone := f.add(t, f.author.ID, nil, "<p>lone</p>", nil)

	if err := f.s.SoftDeleteComment(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SoftDeleteComment(ctx, lone.ID); err != nil {
		t.Fatal(err)
	}
	tree, err := f.s.ListPageComments(ctx, f.page)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].ID != root.ID {
		t.Fatalf("expected only the deleted root with a reply: %+v", tree)
	}
	if tree[0].DeletedAt == nil || tree[0].BodyHTML != "" || tree[0].BodyJSON != nil {
		t.Fatalf("deleted root must be blanked: %+v", tree[0])
	}
	if len(tree[0].Replies) != 1 || tree[0].Replies[0].ID != reply.ID || tree[0].Replies[0].BodyHTML != "<p>reply</p>" {
		t.Fatalf("reply: %+v", tree[0].Replies)
	}

	if err := f.s.SoftDeleteComment(ctx, reply.ID); err != nil {
		t.Fatal(err)
	}
	tree, _ = f.s.ListPageComments(ctx, f.page)
	if len(tree) != 0 {
		t.Fatalf("fully deleted thread must vanish: %+v", tree)
	}

	if err := f.s.UpdateCommentBody(ctx, root.ID, nil, "<p>x</p>"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update deleted: %v", err)
	}
}

func TestCommentUpdateResolveReopen(t *testing.T) {
	f := newCommentFixture(t)
	ctx := context.Background()
	root := f.add(t, f.author.ID, nil, "<p>root</p>", nil)
	reply := f.add(t, f.other.ID, &root.ID, "<p>reply</p>", nil)

	if err := f.s.UpdateCommentBody(ctx, root.ID, json.RawMessage(`{"type":"doc"}`), "<p>edited</p>"); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.GetComment(ctx, root.ID)
	if err != nil || got.BodyHTML != "<p>edited</p>" || !got.UpdatedAt.After(root.UpdatedAt) {
		t.Fatalf("update: %+v %v", got, err)
	}

	changed, err := f.s.ResolveComment(ctx, root.ID, f.other.ID)
	if err != nil || !changed {
		t.Fatalf("resolve: %v %v", changed, err)
	}
	changed, _ = f.s.ResolveComment(ctx, root.ID, f.other.ID)
	if changed {
		t.Fatal("second resolve must be a no-op")
	}
	got, _ = f.s.GetComment(ctx, root.ID)
	if got.ResolvedAt == nil || got.ResolvedBy == nil || *got.ResolvedBy != f.other.ID {
		t.Fatalf("resolved fields: %+v", got)
	}
	if changed, _ := f.s.ResolveComment(ctx, reply.ID, f.other.ID); changed {
		t.Fatal("replies cannot be resolved")
	}
	changed, err = f.s.ReopenComment(ctx, root.ID)
	if err != nil || !changed {
		t.Fatalf("reopen: %v %v", changed, err)
	}
	got, _ = f.s.GetComment(ctx, root.ID)
	if got.ResolvedAt != nil || got.ResolvedBy != nil {
		t.Fatalf("reopened fields: %+v", got)
	}
}

func TestCommentSpaceIDAndCounts(t *testing.T) {
	f := newCommentFixture(t)
	ctx := context.Background()
	root := f.add(t, f.author.ID, nil, "<p>root</p>", nil)
	f.add(t, f.other.ID, &root.ID, "<p>reply</p>", nil)
	gone := f.add(t, f.other.ID, nil, "<p>gone</p>", nil)
	if err := f.s.SoftDeleteComment(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	emptyPage := testutil.CreatePage(t, f.s, f.space, f.author.ID, "Empty")

	sid, err := f.s.GetCommentSpaceID(ctx, root.ID)
	if err != nil || sid != f.space {
		t.Fatalf("space id: %v %v", sid, err)
	}
	if _, err := f.s.GetCommentSpaceID(ctx, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	counts, err := f.s.ListPageCommentCounts(ctx, []uuid.UUID{f.page, emptyPage})
	if err != nil {
		t.Fatal(err)
	}
	if counts[f.page] != 2 || counts[emptyPage] != 0 {
		t.Fatalf("counts: %v", counts)
	}
	if m, err := f.s.ListPageCommentCounts(ctx, nil); err != nil || len(m) != 0 {
		t.Fatalf("empty: %v %v", m, err)
	}
}

func TestCommentCascadeOnPageDelete(t *testing.T) {
	f := newCommentFixture(t)
	ctx := context.Background()
	root := f.add(t, f.author.ID, nil, "<p>root</p>", nil)
	f.add(t, f.other.ID, &root.ID, "<p>reply</p>", nil)
	if _, err := f.s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1`, f.page); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.s.Pool.QueryRow(ctx, `SELECT count(*) FROM comments`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("comments should cascade: %d %v", n, err)
	}
}
