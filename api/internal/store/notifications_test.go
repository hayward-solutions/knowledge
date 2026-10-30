package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestNotificationStoreListAndRead(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	actor := testutil.CreateUser(t, s, "actor@example.test", "", false)
	me := testutil.CreateUser(t, s, "me@example.test", "", false)
	other := testutil.CreateUser(t, s, "other@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: actor.ID})
	page := testutil.CreatePage(t, s, space, actor.ID, "P")

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		n := domain.Notification{
			ID:        uuid.Must(uuid.NewV7()),
			UserID:    me.ID,
			Type:      "page.published",
			ActorID:   actor.ID,
			SpaceID:   &space,
			PageID:    &page,
			Payload:   json.RawMessage(`{"page_title":"Hello"}`),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := s.InsertNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	if err := s.InsertNotification(ctx, domain.Notification{ID: uuid.Must(uuid.NewV7()), UserID: other.ID, Type: "x", ActorID: actor.ID}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListNotifications(ctx, store.NotificationFilter{UserID: me.ID, Limit: 10})
	if err != nil || len(all) != 5 {
		t.Fatalf("all: %d %v", len(all), err)
	}
	if all[0].ID != ids[4] || all[4].ID != ids[0] {
		t.Fatal("expected newest first")
	}
	if all[0].SpaceKey == nil || *all[0].SpaceKey != "ENG" || all[0].PageTitle == nil || *all[0].PageTitle != "Hello" || all[0].Actor == nil || all[0].Actor.ID != actor.ID {
		t.Fatalf("view fields: %+v", all[0])
	}

	page2, err := s.ListNotifications(ctx, store.NotificationFilter{UserID: me.ID, Limit: 2, Before: &all[1].CreatedAt})
	if err != nil || len(page2) != 2 || page2[0].ID != ids[2] || page2[1].ID != ids[1] {
		t.Fatalf("cursor page: %+v %v", page2, err)
	}

	if n, _ := s.CountUnreadNotifications(ctx, me.ID); n != 5 {
		t.Fatalf("unread: %d", n)
	}
	changed, err := s.MarkNotificationsRead(ctx, me.ID, []uuid.UUID{ids[0], ids[1]})
	if err != nil || changed != 2 {
		t.Fatalf("mark: %d %v", changed, err)
	}
	changed, _ = s.MarkNotificationsRead(ctx, other.ID, []uuid.UUID{ids[2]})
	if changed != 0 {
		t.Fatal("must not mark another user's notification")
	}
	changed, _ = s.MarkNotificationsRead(ctx, me.ID, []uuid.UUID{ids[0]})
	if changed != 0 {
		t.Fatal("already read must not change")
	}

	yes, no := true, false
	unread, _ := s.ListNotifications(ctx, store.NotificationFilter{UserID: me.ID, Unread: &yes, Limit: 10})
	read, _ := s.ListNotifications(ctx, store.NotificationFilter{UserID: me.ID, Unread: &no, Limit: 10})
	if len(unread) != 3 || len(read) != 2 || read[0].ReadAt == nil {
		t.Fatalf("filters: %d %d", len(unread), len(read))
	}
	if n, _ := s.CountUnreadNotifications(ctx, me.ID); n != 3 {
		t.Fatalf("unread after: %d", n)
	}
	if n, err := s.MarkAllNotificationsRead(ctx, me.ID); err != nil || n != 3 {
		t.Fatalf("mark all: %d %v", n, err)
	}
	if n, _ := s.CountUnreadNotifications(ctx, other.ID); n != 1 {
		t.Fatalf("other untouched: %d", n)
	}
}

func TestNotificationsSurvivePagePurge(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	actor := testutil.CreateUser(t, s, "actor@example.test", "", false)
	me := testutil.CreateUser(t, s, "me@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: actor.ID})
	page := testutil.CreatePage(t, s, space, actor.ID, "P")
	comment := uuid.Must(uuid.NewV7())
	if err := s.InsertNotification(ctx, domain.Notification{
		ID: uuid.Must(uuid.NewV7()), UserID: me.ID, Type: "comment.created", ActorID: actor.ID,
		SpaceID: &space, PageID: &page, CommentID: &comment, Payload: json.RawMessage(`{"page_title":"P"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertActivity(ctx, domain.Activity{ID: uuid.Must(uuid.NewV7()), SpaceID: space, PageID: &page, ActorID: actor.ID, Type: "page.published", Payload: json.RawMessage(`{"title":"P"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1`, page); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListNotifications(ctx, store.NotificationFilter{UserID: me.ID})
	if err != nil || len(list) != 1 || list[0].PageTitle == nil || *list[0].PageTitle != "P" {
		t.Fatalf("notifications after purge: %+v %v", list, err)
	}
	acts, err := s.ListSpaceActivity(ctx, space, me.ID, domain.RoleViewer, nil, 10)
	if err != nil || len(acts) != 0 {
		t.Fatalf("activity of a purged page must be hidden: %+v %v", acts, err)
	}
	var rows int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM activity`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("activity row should survive purge: %d", rows)
	}
}

func TestNotifyMigrationCascades(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	actor := testutil.CreateUser(t, s, "actor@example.test", "", false)
	me := testutil.CreateUser(t, s, "me@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: actor.ID})
	_ = s.SetWatch(ctx, me.ID, domain.WatchTargetSpace, space)
	_ = s.InsertNotification(ctx, domain.Notification{ID: uuid.Must(uuid.NewV7()), UserID: me.ID, Type: "x", ActorID: actor.ID})
	_ = s.InsertActivity(ctx, domain.Activity{ID: uuid.Must(uuid.NewV7()), SpaceID: space, ActorID: actor.ID, Type: "x"})
	if _, err := s.Pool.Exec(ctx, `DELETE FROM spaces WHERE id = $1`, space); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM activity`).Scan(&n)
	if n != 0 {
		t.Fatalf("activity should cascade with the space: %d", n)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, me.ID); err != nil {
		t.Fatal(err)
	}
	_ = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM watches) + (SELECT count(*) FROM notifications)`).Scan(&n)
	if n != 0 {
		t.Fatalf("watches and notifications should cascade with the user: %d", n)
	}
}

func TestPerfIndexesExist(t *testing.T) {
	s := testutil.NewDB(t)
	for _, name := range []string{"idx_page_views_page", "idx_pages_recent", "idx_pages_draft_owner", "idx_notifications_user_created"} {
		var n int
		if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_indexes WHERE indexname = $1`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s: %d %v", name, n, err)
		}
	}
}

func TestRecursiveQueriesTerminateOnCycles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "u@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: u.ID})
	a := testutil.CreatePage(t, s, space, u.ID, "a")
	b := testutil.CreatePage(t, s, space, u.ID, "b")
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, b, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatch(ctx, u.ID, domain.WatchTargetPage, b); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListPublishWatcherIDs(ctx, a, space)
	if err != nil || len(got) != 1 || got[0] != u.ID {
		t.Fatalf("ancestor watchers on a cycle: %v %v", got, err)
	}

	c1 := uuid.Must(uuid.NewV7())
	c2 := uuid.Must(uuid.NewV7())
	for _, id := range []uuid.UUID{c1, c2} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO comments (id, page_id, author_id) VALUES ($1, $2, $3)`, id, a, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE comments SET parent_id = $2 WHERE id = $1`, c1, c2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE comments SET parent_id = $2 WHERE id = $1`, c2, c1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListCommentThreadAuthorIDs(ctx, c1); err != nil {
		t.Fatalf("thread authors on a cycle: %v", err)
	}
}
