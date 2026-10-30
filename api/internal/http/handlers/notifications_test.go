package handlers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type nwWorld struct {
	*env
	bus      *events.Bus
	hub      *events.Hub
	deps     handlers.Deps
	sysAdmin domain.User
	spaceAdm domain.User
	editor   domain.User
	viewer   domain.User
	outsider domain.User
	space    uuid.UUID
	internal uuid.UUID
	page     uuid.UUID
}

func newNWWorld(t *testing.T) *nwWorld {
	t.Helper()
	s := testutil.NewDB(t)
	bus := events.NewBus()
	hub := events.NewHub()
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	deps := handlers.Deps{
		Store:    s,
		Bus:      bus,
		Hub:      hub,
		Presence: presence,
		Config:   &config.Config{AppOrigins: []string{"http://app.test"}},
		Version:  "test",
	}
	srv := httptest.NewServer(apphttp.NewRouter(deps))
	t.Cleanup(srv.Close)

	w := &nwWorld{env: &env{t: t, store: s, server: srv}, bus: bus, hub: hub, deps: deps}
	w.sysAdmin = testutil.CreateUser(t, s, "root@example.test", "", true)
	w.spaceAdm = testutil.CreateUser(t, s, "sadmin@example.test", "", false)
	w.editor = testutil.CreateUser(t, s, "editor@example.test", "", false)
	w.viewer = testutil.CreateUser(t, s, "viewer@example.test", "", false)
	w.outsider = testutil.CreateUser(t, s, "outsider@example.test", "", false)
	w.space = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PRIV", Visibility: domain.VisibilityPrivate, CreatedBy: w.spaceAdm.ID})
	testutil.AddMember(t, s, w.space, w.spaceAdm.ID, domain.RoleAdmin)
	testutil.AddMember(t, s, w.space, w.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, s, w.space, w.viewer.ID, domain.RoleViewer)
	w.internal = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "OPEN", Visibility: domain.VisibilityInternal, CreatedBy: w.spaceAdm.ID})
	w.page = testutil.CreatePage(t, s, w.space, w.editor.ID, "Roadmap")
	return w
}

func (w *nwWorld) as(u domain.User) *client {
	w.t.Helper()
	c := w.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func (w *nwWorld) publish(e any) {
	w.bus.Publish(e)
	w.bus.Wait()
}

func (w *nwWorld) watchPage(u domain.User, pageID uuid.UUID) {
	w.t.Helper()
	if err := w.store.SetWatch(context.Background(), u.ID, domain.WatchTargetPage, pageID); err != nil {
		w.t.Fatal(err)
	}
}

func (w *nwWorld) notifyViewer(n int) {
	w.t.Helper()
	w.watchPage(w.viewer, w.page)
	for i := 0; i < n; i++ {
		w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID, Version: i + 2})
	}
}

type nwSSE struct {
	t      *testing.T
	cancel context.CancelFunc
	resp   *http.Response
	events chan nwSSEEvent
}

type nwSSEEvent struct {
	name string
	data string
}

func nwOpenStream(t *testing.T, baseURL, token string) *nwSSE {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v1/notifications/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "sid", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &nwSSE{t: t, cancel: cancel, resp: resp, events: make(chan nwSSEEvent, 64)}
	t.Cleanup(func() {
		cancel()
		resp.Body.Close()
	})
	go func() {
		defer close(s.events)
		sc := bufio.NewScanner(resp.Body)
		var cur nwSSEEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.name != "" || cur.data != "" {
					s.events <- cur
				}
				cur = nwSSEEvent{}
			case strings.HasPrefix(line, ": "):
				s.events <- nwSSEEvent{name: "comment", data: strings.TrimPrefix(line, ": ")}
			case strings.HasPrefix(line, "event: "):
				cur.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return s
}

func (s *nwSSE) next(d time.Duration) (nwSSEEvent, bool) {
	s.t.Helper()
	select {
	case ev, ok := <-s.events:
		return ev, ok
	case <-time.After(d):
		s.t.Fatal("timed out waiting for SSE event")
		return nwSSEEvent{}, false
	}
}

func TestNotificationsRequireAuth(t *testing.T) {
	w := newNWWorld(t)
	anon := w.client()
	for _, tc := range [][2]string{
		{"GET", "/api/v1/notifications"},
		{"POST", "/api/v1/notifications/read"},
		{"GET", "/api/v1/notifications/stream"},
		{"GET", "/api/v1/me/dashboard"},
		{"GET", "/api/v1/watch/page/" + w.page.String()},
		{"PUT", "/api/v1/watch/page/" + w.page.String()},
		{"DELETE", "/api/v1/watch/page/" + w.page.String()},
		{"GET", "/api/v1/spaces/PRIV/activity"},
	} {
		if r := anon.do(tc[0], tc[1], nil); r.status != 401 {
			t.Fatalf("%s %s: %d", tc[0], tc[1], r.status)
		}
	}
}

func TestNotificationsListAndUnreadHeader(t *testing.T) {
	w := newNWWorld(t)
	w.notifyViewer(3)
	c := w.as(w.viewer)

	r := c.do("GET", "/api/v1/notifications", nil)
	if r.status != 200 || len(r.list) != 3 {
		t.Fatalf("list: %d %s", r.status, r.raw)
	}
	if r.header.Get("X-Unread-Count") != "3" {
		t.Fatalf("header: %q", r.header.Get("X-Unread-Count"))
	}
	n := r.list[0]
	for _, k := range []string{"id", "user_id", "type", "actor_id", "actor", "space_id", "space_key", "page_id", "page_title", "comment_id", "payload", "read_at", "created_at"} {
		if _, ok := n[k]; !ok {
			t.Fatalf("missing %s in %v", k, n)
		}
	}
	if n["type"] != "page.published" || n["space_key"] != "PRIV" || n["page_title"] != "Roadmap" || n["read_at"] != nil {
		t.Fatalf("fields: %v", n)
	}
	actor := n["actor"].(map[string]any)
	if actor["id"] != w.editor.ID.String() {
		t.Fatalf("actor: %v", actor)
	}
	payload := n["payload"].(map[string]any)
	if payload["version"] != float64(4) || payload["actor_name"] == "" {
		t.Fatalf("payload: %v", payload)
	}
	if r.list[0]["created_at"].(string) < r.list[2]["created_at"].(string) {
		t.Fatal("expected newest first")
	}

	r = c.do("GET", "/api/v1/notifications?limit=2", nil)
	if len(r.list) != 2 {
		t.Fatalf("limit: %d", len(r.list))
	}
	before := r.list[1]["created_at"].(string)
	r = c.do("GET", "/api/v1/notifications?limit=2&before="+url.QueryEscape(before), nil)
	if len(r.list) != 1 {
		t.Fatalf("before: %d %s", len(r.list), r.raw)
	}
	if r := c.do("GET", "/api/v1/notifications?before=yesterday", nil); r.status != 400 {
		t.Fatalf("bad before: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/notifications?unread=maybe", nil); r.status != 400 {
		t.Fatalf("bad unread: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/notifications?limit=1000", nil); len(r.list) != 3 {
		t.Fatalf("limit clamp: %d", len(r.list))
	}

	empty := w.as(w.outsider).do("GET", "/api/v1/notifications", nil)
	if empty.status != 200 || string(empty.raw) != "[]\n" || empty.header.Get("X-Unread-Count") != "0" {
		t.Fatalf("empty: %d %q %q", empty.status, empty.raw, empty.header.Get("X-Unread-Count"))
	}
}

func TestNotificationsMarkRead(t *testing.T) {
	w := newNWWorld(t)
	w.notifyViewer(4)
	c := w.as(w.viewer)
	list := c.do("GET", "/api/v1/notifications", nil).list

	r := c.do("POST", "/api/v1/notifications/read", map[string]any{"ids": []string{list[0]["id"].(string), list[1]["id"].(string)}})
	if r.status != 204 {
		t.Fatalf("mark ids: %d %s", r.status, r.raw)
	}
	r = c.do("GET", "/api/v1/notifications?unread=true", nil)
	if len(r.list) != 2 || r.header.Get("X-Unread-Count") != "2" {
		t.Fatalf("unread: %d %q", len(r.list), r.header.Get("X-Unread-Count"))
	}
	r = c.do("GET", "/api/v1/notifications?unread=false", nil)
	if len(r.list) != 2 || r.list[0]["read_at"] == nil {
		t.Fatalf("read: %d %s", len(r.list), r.raw)
	}

	editor := w.as(w.editor)
	if r := editor.do("POST", "/api/v1/notifications/read", map[string]any{"ids": []string{list[2]["id"].(string)}}); r.status != 204 {
		t.Fatalf("foreign ids: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/notifications?unread=true", nil); len(r.list) != 2 {
		t.Fatal("another user's request must not mark my notifications")
	}

	for _, body := range []any{
		map[string]any{},
		map[string]any{"ids": []string{}},
		map[string]any{"ids": []string{"not-a-uuid"}},
	} {
		if r := c.do("POST", "/api/v1/notifications/read", body); r.status != 400 {
			t.Fatalf("body %v: %d", body, r.status)
		}
	}

	if r := c.do("POST", "/api/v1/notifications/read", map[string]any{"all": true}); r.status != 204 {
		t.Fatalf("mark all: %d", r.status)
	}
	r = c.do("GET", "/api/v1/notifications?unread=true", nil)
	if len(r.list) != 0 || r.header.Get("X-Unread-Count") != "0" {
		t.Fatalf("after all: %d %q", len(r.list), r.header.Get("X-Unread-Count"))
	}
}

func TestNotificationsFromRealPublishFlow(t *testing.T) {
	w := newNWWorld(t)
	editor := w.as(w.editor)
	viewer := w.as(w.viewer)
	if r := viewer.do("PUT", fmt.Sprintf("/api/v1/watch/page/%s", w.page), nil); r.status != 200 {
		t.Fatalf("watch: %d %s", r.status, r.raw)
	}
	r := editor.do("POST", fmt.Sprintf("/api/v1/pages/%s/publish", w.page), map[string]any{
		"title":        "Roadmap v2",
		"content_json": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}},
		"content_html": "<p>hello</p>",
		"base_version": 0,
		"message":      "first real publish",
	})
	if r.status != 200 {
		t.Fatalf("publish: %d %s", r.status, r.raw)
	}
	w.bus.Wait()
	list := viewer.do("GET", "/api/v1/notifications", nil).list
	if len(list) != 1 || list[0]["page_title"] != "Roadmap v2" {
		t.Fatalf("notifications: %v", list)
	}
	if got := editor.do("GET", "/api/v1/notifications", nil); len(got.list) != 0 {
		t.Fatalf("actor notified: %s", got.raw)
	}
	if r := editor.do("GET", fmt.Sprintf("/api/v1/pages/%s", w.page), nil); r.status != 200 || r.body["watching"] != true {
		t.Fatalf("creator auto-watch flag: %v", r.body["watching"])
	}
	if r := viewer.do("GET", fmt.Sprintf("/api/v1/pages/%s", w.page), nil); r.status != 200 || r.body["watching"] != true {
		t.Fatalf("viewer watching flag: %v", r.body["watching"])
	}
	acts := editor.do("GET", "/api/v1/spaces/PRIV/activity", nil)
	if acts.status != 200 || len(acts.list) != 1 || acts.list[0]["type"] != "page.published" {
		t.Fatalf("activity: %d %s", acts.status, acts.raw)
	}
}

func TestNotificationStreamHelloAndForward(t *testing.T) {
	w := newNWWorld(t)
	w.notifyViewer(2)
	token := testutil.NewSession(t, w.store, w.viewer.ID)

	stream := nwOpenStream(t, w.server.URL, token)
	if ct := stream.resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type: %q", ct)
	}
	hello, ok := stream.next(5 * time.Second)
	if !ok || hello.name != "hello" {
		t.Fatalf("hello: %+v", hello)
	}
	var h map[string]float64
	if err := json.Unmarshal([]byte(hello.data), &h); err != nil || h["unread_count"] != 2 {
		t.Fatalf("hello data: %s", hello.data)
	}
	if got := w.hub.Subscribers(w.viewer.ID); got != 1 {
		t.Fatalf("subscribers: %d", got)
	}

	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID, Version: 9})
	first, _ := stream.next(5 * time.Second)
	second, _ := stream.next(5 * time.Second)
	if first.name != "notification" || second.name != "unread_count" {
		t.Fatalf("forwarded: %+v %+v", first, second)
	}
	var n map[string]any
	if err := json.Unmarshal([]byte(first.data), &n); err != nil || n["type"] != "page.published" || n["page_title"] != "Roadmap" || n["user_id"] != w.viewer.ID.String() {
		t.Fatalf("notification data: %s", first.data)
	}
	var cnt map[string]float64
	if err := json.Unmarshal([]byte(second.data), &cnt); err != nil || cnt["count"] != 3 {
		t.Fatalf("unread data: %s", second.data)
	}

	c := w.as(w.viewer)
	if r := c.do("POST", "/api/v1/notifications/read", map[string]any{"all": true}); r.status != 204 {
		t.Fatalf("mark all: %d", r.status)
	}
	third, _ := stream.next(5 * time.Second)
	if third.name != "unread_count" || third.data != `{"count":0}` {
		t.Fatalf("after read: %+v", third)
	}

	stream.cancel()
	deadline := time.Now().Add(5 * time.Second)
	for w.hub.Subscribers(w.viewer.ID) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("stream did not unsubscribe on disconnect")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNotificationStreamEndsWhenSessionInvalid(t *testing.T) {
	w := newNWWorld(t)
	h := &handlers.NotificationHandler{Deps: w.deps, Heartbeat: 40 * time.Millisecond}
	r := chi.NewRouter()
	r.Route("/api/v1", h.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	token := testutil.NewSession(t, w.store, w.viewer.ID)
	stream := nwOpenStream(t, srv.URL, token)
	if ev, _ := stream.next(5 * time.Second); ev.name != "hello" {
		t.Fatalf("hello: %+v", ev)
	}
	if ev, _ := stream.next(5 * time.Second); ev.name != "comment" || ev.data != "ping" {
		t.Fatalf("expected a ping, got %+v", ev)
	}
	if err := w.store.DeleteUserSessions(context.Background(), w.viewer.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, open := <-stream.events:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("stream stayed open after the session was deleted")
		}
	}
}
