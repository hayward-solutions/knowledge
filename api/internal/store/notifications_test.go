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
	acts, err := s.ListSpaceActivity(ctx, space, me.ID, false, nil, 10)
	if err != nil || len(acts) != 1 || acts[0].PageTitle == nil || *acts[0].PageTitle != "P" {
		t.Fatalf("activity after purge: %+v %v", acts, err)
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
