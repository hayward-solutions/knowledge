package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestWatchSetRemoveIdempotent(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "u@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: u.ID})
	page := testutil.CreatePage(t, s, space, u.ID, "P")

	if ok, err := s.IsWatching(ctx, u.ID, domain.WatchTargetPage, page); err != nil || ok {
		t.Fatalf("initial: %v %v", ok, err)
	}
	for i := 0; i < 2; i++ {
		if err := s.SetWatch(ctx, u.ID, domain.WatchTargetPage, page); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := s.IsWatching(ctx, u.ID, domain.WatchTargetPage, page); !ok {
		t.Fatal("expected watching")
	}
	if ok, _ := s.IsWatching(ctx, u.ID, domain.WatchTargetSpace, page); ok {
		t.Fatal("target types must be distinct")
	}
	list, err := s.ListWatches(ctx, u.ID)
	if err != nil || len(list) != 1 || list[0].TargetType != domain.WatchTargetPage {
		t.Fatalf("list: %+v %v", list, err)
	}
	for i := 0; i < 2; i++ {
		if err := s.RemoveWatch(ctx, u.ID, domain.WatchTargetPage, page); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := s.IsWatching(ctx, u.ID, domain.WatchTargetPage, page); ok {
		t.Fatal("expected not watching")
	}
}

func TestWatchTargetTypeConstraint(t *testing.T) {
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "u@example.test", "", false)
	_, err := s.Pool.Exec(context.Background(),
		`INSERT INTO watches (user_id, target_type, target_id) VALUES ($1, 'comment', $2)`, u.ID, uuid.Must(uuid.NewV7()))
	if err == nil {
		t.Fatal("expected check violation")
	}
}

func TestListPublishWatcherIDsWalksAncestors(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	owner := testutil.CreateUser(t, s, "owner@example.test", "", false)
	a := testutil.CreateUser(t, s, "a@example.test", "", false)
	b := testutil.CreateUser(t, s, "b@example.test", "", false)
	c := testutil.CreateUser(t, s, "c@example.test", "", false)
	d := testutil.CreateUser(t, s, "d@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: owner.ID})
	otherSpace := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "OPS", CreatedBy: owner.ID})
	root := testutil.CreatePage(t, s, space, owner.ID, "root")
	mid := testutil.CreatePage(t, s, space, owner.ID, "mid")
	leaf := testutil.CreatePage(t, s, space, owner.ID, "leaf")
	sibling := testutil.CreatePage(t, s, space, owner.ID, "sibling")
	for child, parent := range map[uuid.UUID]uuid.UUID{mid: root, leaf: mid, sibling: root} {
		if _, err := s.Pool.Exec(ctx, `UPDATE pages SET parent_id = $1 WHERE id = $2`, parent, child); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.SetWatch(ctx, a.ID, domain.WatchTargetPage, leaf)
	_ = s.SetWatch(ctx, b.ID, domain.WatchTargetPage, root)
	_ = s.SetWatch(ctx, c.ID, domain.WatchTargetSpace, space)
	_ = s.SetWatch(ctx, d.ID, domain.WatchTargetPage, sibling)
	_ = s.SetWatch(ctx, d.ID, domain.WatchTargetSpace, otherSpace)
	_ = s.SetWatch(ctx, a.ID, domain.WatchTargetPage, root)

	got, err := s.ListPublishWatcherIDs(ctx, leaf, space)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID]bool{a.ID: true, b.ID: true, c.ID: true}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected watcher %s", id)
		}
	}

	direct, err := s.ListPageWatcherIDs(ctx, leaf)
	if err != nil || len(direct) != 1 || direct[0] != a.ID {
		t.Fatalf("direct: %v %v", direct, err)
	}
}
