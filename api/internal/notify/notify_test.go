package notify_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	"github.com/hayward-solutions/knowledge/api/internal/notify"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type world struct {
	t        *testing.T
	s        *store.Store
	bus      *events.Bus
	hub      *events.Hub
	ctx      context.Context
	created  *busCollector
	actor    domain.User
	pageW    domain.User
	ancW     domain.User
	spaceW   domain.User
	mention  domain.User
	other    domain.User
	outsider domain.User
	space    uuid.UUID
	parent   uuid.UUID
	page     uuid.UUID
}

type busCollector struct {
	mu   sync.Mutex
	list []domain.NotificationCreated
}

func (c *busCollector) all() []domain.NotificationCreated {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]domain.NotificationCreated(nil), c.list...)
}

func newWorld(t *testing.T) *world {
	t.Helper()
	s := testutil.NewDB(t)
	w := &world{t: t, s: s, bus: events.NewBus(), hub: events.NewHub(), ctx: context.Background(), created: &busCollector{}}
	w.bus.Subscribe(func(e any) {
		if nc, ok := e.(domain.NotificationCreated); ok {
			w.created.mu.Lock()
			w.created.list = append(w.created.list, nc)
			w.created.mu.Unlock()
		}
	})
	notify.New(s, w.bus, w.hub).Subscribe()

	w.actor = testutil.CreateUser(t, s, "actor@example.test", "", false)
	w.pageW = testutil.CreateUser(t, s, "pagew@example.test", "", false)
	w.ancW = testutil.CreateUser(t, s, "ancw@example.test", "", false)
	w.spaceW = testutil.CreateUser(t, s, "spacew@example.test", "", false)
	w.mention = testutil.CreateUser(t, s, "mention@example.test", "", false)
	w.other = testutil.CreateUser(t, s, "other@example.test", "", false)
	w.outsider = testutil.CreateUser(t, s, "outsider@example.test", "", false)
	w.space = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", Visibility: domain.VisibilityPrivate, CreatedBy: w.actor.ID})
	for _, u := range []domain.User{w.actor, w.pageW, w.ancW, w.spaceW, w.mention, w.other} {
		testutil.AddMember(t, s, w.space, u.ID, domain.RoleEditor)
	}
	w.parent = testutil.CreatePage(t, s, w.space, w.actor.ID, "Parent page")
	w.page = testutil.CreatePage(t, s, w.space, w.actor.ID, "Child page")
	w.exec(`UPDATE pages SET parent_id = $1 WHERE id = $2`, w.parent, w.page)
	return w
}

func (w *world) exec(sql string, args ...any) {
	w.t.Helper()
	if _, err := w.s.Pool.Exec(w.ctx, sql, args...); err != nil {
		w.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (w *world) watch(u domain.User, kind domain.WatchTarget, id uuid.UUID) {
	w.t.Helper()
	if err := w.s.SetWatch(w.ctx, u.ID, kind, id); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) publish(e any) {
	w.bus.Publish(e)
	w.bus.Wait()
}

func (w *world) notifications(u domain.User) []store.NotificationView {
	w.t.Helper()
	list, err := w.s.ListNotifications(w.ctx, store.NotificationFilter{UserID: u.ID, Limit: 100})
	if err != nil {
		w.t.Fatal(err)
	}
	return list
}

func (w *world) activity(spaceID uuid.UUID) []store.ActivityView {
	w.t.Helper()
	list, err := w.s.ListSpaceActivity(w.ctx, spaceID, w.actor.ID, domain.RoleAdmin, nil, 100)
	if err != nil {
		w.t.Fatal(err)
	}
	return list
}

func (w *world) typesFor(u domain.User) []string {
	var out []string
	for _, n := range w.notifications(u) {
		out = append(out, n.Type)
	}
	return out
}

func (w *world) isWatching(u domain.User, id uuid.UUID) bool {
	w.t.Helper()
	ok, err := w.s.IsWatching(w.ctx, u.ID, domain.WatchTargetPage, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return ok
}

func (w *world) comment(author domain.User, page uuid.UUID, parent *uuid.UUID, html string) domain.Comment {
	w.t.Helper()
	c, err := w.s.CreateComment(w.ctx, domain.Comment{
		ID:       uuid.Must(uuid.NewV7()),
		PageID:   page,
		ParentID: parent,
		AuthorID: author.ID,
		BodyHTML: html,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return c
}

func drain(ch chan events.Event) []events.Event {
	var out []events.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func equalStrings(a, b []string) bool {
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

func TestPagePublishedRecipients(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	w.watch(w.ancW, domain.WatchTargetPage, w.parent)
	w.watch(w.spaceW, domain.WatchTargetSpace, w.space)
	w.watch(w.actor, domain.WatchTargetPage, w.page)
	w.watch(w.mention, domain.WatchTargetPage, w.page)
	w.watch(w.other, domain.WatchTargetPage, w.page)
	w.watch(w.other, domain.WatchTargetPage, w.parent)
	w.watch(w.other, domain.WatchTargetSpace, w.space)

	w.publish(domain.PagePublished{
		PageID:           w.page,
		SpaceID:          w.space,
		ActorID:          w.actor.ID,
		Version:          2,
		MentionedUserIDs: []uuid.UUID{w.mention.ID, w.actor.ID, w.outsider.ID},
	})

	if got := w.typesFor(w.pageW); !equalStrings(got, []string{"page.published"}) {
		t.Fatalf("page watcher: %v", got)
	}
	if got := w.typesFor(w.ancW); !equalStrings(got, []string{"page.published"}) {
		t.Fatalf("ancestor watcher: %v", got)
	}
	if got := w.typesFor(w.spaceW); !equalStrings(got, []string{"page.published"}) {
		t.Fatalf("space watcher: %v", got)
	}
	if got := w.typesFor(w.mention); !equalStrings(got, []string{"page.mentioned"}) {
		t.Fatalf("mention should win over watch: %v", got)
	}
	if got := w.typesFor(w.other); !equalStrings(got, []string{"page.published"}) {
		t.Fatalf("triple watcher must be deduped: %v", got)
	}
	if got := w.typesFor(w.actor); len(got) != 0 {
		t.Fatalf("actor must not be notified: %v", got)
	}
	if got := w.typesFor(w.outsider); len(got) != 0 {
		t.Fatalf("mentioned non-member of a private space must not be notified: %v", got)
	}
	if n := len(w.created.all()); n != 5 {
		t.Fatalf("NotificationCreated events: %d", n)
	}
}

func TestPagePublishedActivityAndPayload(t *testing.T) {
	w := newWorld(t)
	w.exec(`INSERT INTO page_versions (id, page_id, version, title, author_id, message) VALUES ($1, $2, 3, 'Child page', $3, 'Fixed typos')`,
		uuid.Must(uuid.NewV7()), w.page, w.actor.ID)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)

	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 3})

	acts := w.activity(w.space)
	if len(acts) != 1 || acts[0].Type != "page.published" {
		t.Fatalf("activity: %+v", acts)
	}
	if acts[0].PageID == nil || *acts[0].PageID != w.page || acts[0].ActorID != w.actor.ID {
		t.Fatalf("activity ids: %+v", acts[0])
	}
	if acts[0].PageTitle == nil || *acts[0].PageTitle != "Child page" {
		t.Fatalf("activity page_title: %v", acts[0].PageTitle)
	}
	var payload map[string]any
	if err := json.Unmarshal(acts[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "Child page" || payload["version"] != float64(3) || payload["message"] != "Fixed typos" {
		t.Fatalf("activity payload: %v", payload)
	}

	ns := w.notifications(w.pageW)
	if len(ns) != 1 {
		t.Fatalf("notifications: %d", len(ns))
	}
	n := ns[0]
	if n.ReadAt != nil || n.ActorID != w.actor.ID || n.PageID == nil || *n.PageID != w.page || n.SpaceID == nil || *n.SpaceID != w.space {
		t.Fatalf("notification fields: %+v", n)
	}
	if n.SpaceKey == nil || *n.SpaceKey != "ENG" || n.PageTitle == nil || *n.PageTitle != "Child page" {
		t.Fatalf("view fields: %v %v", n.SpaceKey, n.PageTitle)
	}
	if n.Actor == nil || n.Actor.ID != w.actor.ID {
		t.Fatalf("actor: %+v", n.Actor)
	}
	var np map[string]any
	if err := json.Unmarshal(n.Payload, &np); err != nil {
		t.Fatal(err)
	}
	if np["page_title"] != "Child page" || np["space_key"] != "ENG" || np["actor_name"] != w.actor.DisplayName ||
		np["version"] != float64(3) || np["message"] != "Fixed typos" {
		t.Fatalf("payload: %v", np)
	}
}

func TestHubEventsAndBusNotificationCreated(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	ch, unsub := w.hub.Subscribe(w.pageW.ID)
	defer unsub()
	other, unsubOther := w.hub.Subscribe(w.other.ID)
	defer unsubOther()

	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2})

	got := drain(ch)
	if len(got) != 2 || got[0].Kind != "notification" || got[1].Kind != "unread_count" {
		t.Fatalf("hub events: %+v", got)
	}
	view, ok := got[0].Payload.(store.NotificationView)
	if !ok || view.UserID != w.pageW.ID || view.Type != "page.published" {
		t.Fatalf("notification payload: %#v", got[0].Payload)
	}
	count, ok := got[1].Payload.(map[string]int)
	if !ok || count["count"] != 1 {
		t.Fatalf("unread payload: %#v", got[1].Payload)
	}
	if extra := drain(other); len(extra) != 0 {
		t.Fatalf("unrelated user got events: %+v", extra)
	}
	created := w.created.all()
	if len(created) != 1 || created[0].UserID != w.pageW.ID || created[0].NotificationID != view.ID {
		t.Fatalf("NotificationCreated: %+v", created)
	}

	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 3})
	got = drain(ch)
	if len(got) != 2 || got[1].Payload.(map[string]int)["count"] != 2 {
		t.Fatalf("second round: %+v", got)
	}
}

func TestPagePublishedAutoWatch(t *testing.T) {
	w := newWorld(t)
	fresh := testutil.CreatePage(t, w.s, w.space, w.actor.ID, "Fresh")
	w.publish(domain.PagePublished{PageID: fresh, SpaceID: w.space, ActorID: w.actor.ID, Version: 1})
	if !w.isWatching(w.actor, fresh) {
		t.Fatal("creator publishing version 1 should auto-watch")
	}

	second := testutil.CreatePage(t, w.s, w.space, w.actor.ID, "Second")
	w.publish(domain.PagePublished{PageID: second, SpaceID: w.space, ActorID: w.actor.ID, Version: 2})
	if w.isWatching(w.actor, second) {
		t.Fatal("version 2 must not auto-watch")
	}

	theirs := testutil.CreatePage(t, w.s, w.space, w.other.ID, "Theirs")
	w.publish(domain.PagePublished{PageID: theirs, SpaceID: w.space, ActorID: w.actor.ID, Version: 1})
	if w.isWatching(w.actor, theirs) {
		t.Fatal("publisher who did not create the page must not auto-watch")
	}
}

func TestCommentCreatedTopLevel(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	w.watch(w.actor, domain.WatchTargetPage, w.parent)
	w.watch(w.ancW, domain.WatchTargetPage, w.parent)
	w.watch(w.spaceW, domain.WatchTargetSpace, w.space)
	w.watch(w.mention, domain.WatchTargetPage, w.page)

	c := w.comment(w.actor, w.page, nil, "<p>Hello <b>team</b></p>")
	w.publish(domain.CommentCreated{
		CommentID:        c.ID,
		PageID:           w.page,
		SpaceID:          w.space,
		ActorID:          w.actor.ID,
		Inline:           true,
		MentionedUserIDs: []uuid.UUID{w.mention.ID, w.actor.ID},
	})

	if got := w.typesFor(w.pageW); !equalStrings(got, []string{"comment.created"}) {
		t.Fatalf("page watcher: %v", got)
	}
	if got := w.typesFor(w.mention); !equalStrings(got, []string{"comment.mentioned"}) {
		t.Fatalf("mention: %v", got)
	}
	for _, u := range []domain.User{w.actor, w.ancW, w.spaceW, w.other} {
		if got := w.typesFor(u); len(got) != 0 {
			t.Fatalf("%s should not be notified: %v", u.Email, got)
		}
	}
	if !w.isWatching(w.actor, w.page) {
		t.Fatal("commenter should auto-watch the page")
	}

	acts := w.activity(w.space)
	if len(acts) != 1 || acts[0].Type != "comment.created" {
		t.Fatalf("activity: %+v", acts)
	}
	var ap map[string]any
	_ = json.Unmarshal(acts[0].Payload, &ap)
	if ap["title"] != "Child page" || ap["excerpt"] != "Hello team" || ap["inline"] != true || ap["comment_id"] != c.ID.String() {
		t.Fatalf("activity payload: %v", ap)
	}

	n := w.notifications(w.pageW)[0]
	if n.CommentID == nil || *n.CommentID != c.ID {
		t.Fatalf("comment id: %+v", n.CommentID)
	}
	var np map[string]any
	_ = json.Unmarshal(n.Payload, &np)
	if np["comment_excerpt"] != "Hello team" || np["page_title"] != "Child page" || np["space_key"] != "ENG" || np["actor_name"] != w.actor.DisplayName {
		t.Fatalf("payload: %v", np)
	}
}

func TestCommentCreatedReplyThread(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	w.watch(w.other, domain.WatchTargetPage, w.page)

	root := w.comment(w.ancW, w.page, nil, "<p>Root</p>")
	first := w.comment(w.spaceW, w.page, &root.ID, "<p>First reply</p>")
	second := w.comment(w.other, w.page, &first.ID, "<p>Second reply</p>")
	reply := w.comment(w.actor, w.page, &second.ID, "<p>Mine</p>")

	w.publish(domain.CommentCreated{
		CommentID:        reply.ID,
		PageID:           w.page,
		SpaceID:          w.space,
		ActorID:          w.actor.ID,
		ParentID:         &second.ID,
		MentionedUserIDs: []uuid.UUID{w.spaceW.ID},
	})

	want := map[string][]string{
		w.other.Email:  {"comment.replied"},
		w.ancW.Email:   {"comment.replied"},
		w.spaceW.Email: {"comment.mentioned"},
		w.pageW.Email:  {"comment.created"},
		w.actor.Email:  nil,
	}
	for _, u := range []domain.User{w.other, w.ancW, w.spaceW, w.pageW, w.actor} {
		got := w.typesFor(u)
		if !equalStrings(got, want[u.Email]) {
			t.Fatalf("%s: got %v want %v", u.Email, got, want[u.Email])
		}
	}
	var ap map[string]any
	_ = json.Unmarshal(w.activity(w.space)[0].Payload, &ap)
	if ap["reply"] != true {
		t.Fatalf("activity payload: %v", ap)
	}
}

func TestCommentExcerptTruncated(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	long := strings.Repeat("é", 300)
	c := w.comment(w.actor, w.page, nil, "<p>"+long+"</p>")
	w.publish(domain.CommentCreated{CommentID: c.ID, PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})
	n := w.notifications(w.pageW)
	if len(n) != 1 {
		t.Fatalf("notifications: %d", len(n))
	}
	var np map[string]any
	_ = json.Unmarshal(n[0].Payload, &np)
	if got := np["comment_excerpt"].(string); len([]rune(got)) != 140 {
		t.Fatalf("excerpt length %d", len([]rune(got)))
	}
}

func TestCommentOnDraftPageHiddenFromWatchers(t *testing.T) {
	w := newWorld(t)
	draft := testutil.CreatePage(t, w.s, w.space, w.actor.ID, "Secret draft")
	w.exec(`UPDATE pages SET status = 'draft' WHERE id = $1`, draft)
	w.watch(w.pageW, domain.WatchTargetPage, draft)
	w.watch(w.other, domain.WatchTargetPage, draft)
	admin := testutil.CreateUser(t, w.s, "spaceadmin@example.test", "", false)
	testutil.AddMember(t, w.s, w.space, admin.ID, domain.RoleAdmin)
	w.watch(admin, domain.WatchTargetPage, draft)

	c := w.comment(w.other, draft, nil, "<p>hm</p>")
	w.publish(domain.CommentCreated{CommentID: c.ID, PageID: draft, SpaceID: w.space, ActorID: w.other.ID})

	if got := w.typesFor(w.pageW); len(got) != 0 {
		t.Fatalf("non-creator editor must not hear about a draft: %v", got)
	}
	if got := w.typesFor(admin); !equalStrings(got, []string{"comment.created"}) {
		t.Fatalf("space admin: %v", got)
	}
	if got := w.typesFor(w.actor); len(got) != 0 {
		t.Fatalf("creator not watching: %v", got)
	}
}

func TestCommentResolved(t *testing.T) {
	w := newWorld(t)
	root := w.comment(w.ancW, w.page, nil, "<p>Please fix</p>")
	w.publish(domain.CommentResolved{CommentID: root.ID, PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})
	if got := w.typesFor(w.ancW); !equalStrings(got, []string{"comment.resolved"}) {
		t.Fatalf("author: %v", got)
	}
	n := w.notifications(w.ancW)[0]
	var np map[string]any
	_ = json.Unmarshal(n.Payload, &np)
	if np["comment_excerpt"] != "Please fix" || np["page_title"] != "Child page" {
		t.Fatalf("payload: %v", np)
	}

	own := w.comment(w.actor, w.page, nil, "<p>mine</p>")
	w.publish(domain.CommentResolved{CommentID: own.ID, PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})
	if got := w.typesFor(w.actor); len(got) != 0 {
		t.Fatalf("resolving own comment should not notify: %v", got)
	}
}

func TestPageLifecycleActivityOnly(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	old := w.parent
	w.publish(domain.PageMoved{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, OldParentID: &old})
	w.publish(domain.PageTrashed{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})
	w.publish(domain.PageRestored{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})

	acts := w.activity(w.space)
	var types []string
	for _, a := range acts {
		types = append(types, a.Type)
	}
	if len(acts) != 3 {
		t.Fatalf("activity: %v", types)
	}
	seen := map[string]store.ActivityView{}
	for _, a := range acts {
		seen[a.Type] = a
	}
	for _, typ := range []string{"page.moved", "page.trashed", "page.restored"} {
		a, ok := seen[typ]
		if !ok || a.PageTitle == nil || *a.PageTitle != "Child page" {
			t.Fatalf("missing %s: %v", typ, types)
		}
	}
	var mp map[string]any
	_ = json.Unmarshal(seen["page.moved"].Payload, &mp)
	if mp["old_parent_id"] != old.String() || mp["new_parent_id"] != nil {
		t.Fatalf("moved payload: %v", mp)
	}
	if got := w.typesFor(w.pageW); len(got) != 0 {
		t.Fatalf("lifecycle events must not notify: %v", got)
	}
}

func TestSpaceMemberAdded(t *testing.T) {
	w := newWorld(t)
	newcomer := testutil.CreateUser(t, w.s, "newcomer@example.test", "", false)
	testutil.AddMember(t, w.s, w.space, newcomer.ID, domain.RoleViewer)
	ch, unsub := w.hub.Subscribe(newcomer.ID)
	defer unsub()

	w.publish(domain.SpaceMemberAdded{SpaceID: w.space, UserID: newcomer.ID, ActorID: w.actor.ID, Role: domain.RoleViewer})

	ns := w.notifications(newcomer)
	if len(ns) != 1 || ns[0].Type != "space.added" || ns[0].PageID != nil {
		t.Fatalf("notifications: %+v", ns)
	}
	if ns[0].SpaceKey == nil || *ns[0].SpaceKey != "ENG" {
		t.Fatalf("space key: %v", ns[0].SpaceKey)
	}
	var np map[string]any
	_ = json.Unmarshal(ns[0].Payload, &np)
	if np["space_key"] != "ENG" || np["role"] != "viewer" || np["actor_name"] != w.actor.DisplayName {
		t.Fatalf("payload: %v", np)
	}
	if got := drain(ch); len(got) != 2 {
		t.Fatalf("hub events: %+v", got)
	}

	self := testutil.CreateUser(t, w.s, "self@example.test", "", false)
	testutil.AddMember(t, w.s, w.space, self.ID, domain.RoleAdmin)
	w.publish(domain.SpaceMemberAdded{SpaceID: w.space, UserID: self.ID, ActorID: self.ID, Role: domain.RoleAdmin})
	if got := w.typesFor(self); len(got) != 0 {
		t.Fatalf("self-add must not notify: %v", got)
	}
}

func TestDisabledAndRemovedWatchersSkipped(t *testing.T) {
	w := newWorld(t)
	w.watch(w.pageW, domain.WatchTargetPage, w.page)
	w.watch(w.outsider, domain.WatchTargetPage, w.page)
	w.watch(w.other, domain.WatchTargetPage, w.page)
	w.exec(`UPDATE users SET disabled_at = now() WHERE id = $1`, w.other.ID)

	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2})

	if got := w.typesFor(w.pageW); len(got) != 1 {
		t.Fatalf("active watcher: %v", got)
	}
	if got := w.typesFor(w.outsider); len(got) != 0 {
		t.Fatalf("non-member watcher: %v", got)
	}
	if got := w.typesFor(w.other); len(got) != 0 {
		t.Fatalf("disabled watcher: %v", got)
	}
}

func TestInternalSpaceNonMemberWatcherNotified(t *testing.T) {
	w := newWorld(t)
	w.exec(`UPDATE spaces SET visibility = 'internal' WHERE id = $1`, w.space)
	w.watch(w.outsider, domain.WatchTargetSpace, w.space)
	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2})
	if got := w.typesFor(w.outsider); len(got) != 1 {
		t.Fatalf("internal space watcher: %v", got)
	}
}

func TestHandleIgnoresUnknownEventsAndMissingRows(t *testing.T) {
	w := newWorld(t)
	w.publish("not an event")
	w.publish(domain.PageTrashed{PageID: uuid.Must(uuid.NewV7()), SpaceID: w.space, ActorID: w.actor.ID})
	w.publish(domain.CommentCreated{CommentID: uuid.Must(uuid.NewV7()), PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID})
	if n := len(w.activity(w.space)); n != 0 {
		t.Fatalf("activity rows: %d", n)
	}
}

func TestPageMentionNotRepeatedWhileUnread(t *testing.T) {
	w := newWorld(t)
	ev := domain.PagePublished{
		PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2,
		MentionedUserIDs: []uuid.UUID{w.mention.ID},
	}
	w.publish(ev)
	ev.Version = 3
	w.publish(ev)
	if got := w.typesFor(w.mention); !equalStrings(got, []string{"page.mentioned"}) {
		t.Fatalf("repeat publish should not re-mention: %v", got)
	}
	if n := len(w.created.all()); n != 1 {
		t.Fatalf("NotificationCreated events: %d", n)
	}

	other := testutil.CreatePage(t, w.s, w.space, w.actor.ID, "Another page")
	w.publish(domain.PagePublished{PageID: other, SpaceID: w.space, ActorID: w.actor.ID, Version: 2, MentionedUserIDs: []uuid.UUID{w.mention.ID}})
	if got := w.typesFor(w.mention); len(got) != 2 {
		t.Fatalf("mention on a different page is independent: %v", got)
	}
}

func TestPageMentionedWatcherFallsBackToWatchNotification(t *testing.T) {
	w := newWorld(t)
	w.watch(w.mention, domain.WatchTargetPage, w.page)
	ev := domain.PagePublished{
		PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2,
		MentionedUserIDs: []uuid.UUID{w.mention.ID},
	}
	w.publish(ev)
	ev.Version = 3
	w.publish(ev)
	got := w.typesFor(w.mention)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	counts := map[string]int{}
	for _, typ := range got {
		counts[typ]++
	}
	if counts["page.mentioned"] != 1 || counts["page.published"] != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestPageMentionRenotifiedAfterRead(t *testing.T) {
	w := newWorld(t)
	ev := domain.PagePublished{
		PageID: w.page, SpaceID: w.space, ActorID: w.actor.ID, Version: 2,
		MentionedUserIDs: []uuid.UUID{w.mention.ID},
	}
	w.publish(ev)
	if _, err := w.s.MarkAllNotificationsRead(w.ctx, w.mention.ID); err != nil {
		t.Fatal(err)
	}
	ev.Version = 3
	w.publish(ev)
	if got := w.typesFor(w.mention); len(got) != 2 {
		t.Fatalf("after reading, a fresh mention event notifies again: %v", got)
	}
}
