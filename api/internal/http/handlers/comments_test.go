package handlers_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type commentEvents struct {
	mu   sync.Mutex
	list []any
	bus  *events.Bus
}

func (c *commentEvents) snapshot() []any {
	c.bus.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]any(nil), c.list...)
}

type commentWorld struct {
	*env
	events    *commentEvents
	sysAdmin  domain.User
	spaceAdm  domain.User
	editor    domain.User
	viewer    domain.User
	outsider  domain.User
	space     uuid.UUID
	page      uuid.UUID
	otherPage uuid.UUID
}

func newCommentWorld(t *testing.T) *commentWorld {
	t.Helper()
	s := testutil.NewDB(t)
	bus := events.NewBus()
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store:    s,
		Bus:      bus,
		Hub:      events.NewHub(),
		Presence: presence,
		Config:   &config.Config{AppOrigins: []string{"http://app.test"}},
		Version:  "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	ev := &commentEvents{bus: bus}
	bus.Subscribe(func(e any) {
		ev.mu.Lock()
		defer ev.mu.Unlock()
		ev.list = append(ev.list, e)
	})

	w := &commentWorld{env: &env{t: t, store: s, server: srv}, events: ev}
	w.sysAdmin = testutil.CreateUser(t, s, "root@example.test", "", true)
	w.spaceAdm = testutil.CreateUser(t, s, "sadmin@example.test", "", false)
	w.editor = testutil.CreateUser(t, s, "editor@example.test", "", false)
	w.viewer = testutil.CreateUser(t, s, "viewer@example.test", "", false)
	w.outsider = testutil.CreateUser(t, s, "outsider@example.test", "", false)
	w.space = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PRIV", Visibility: domain.VisibilityPrivate, CreatedBy: w.spaceAdm.ID})
	testutil.AddMember(t, s, w.space, w.spaceAdm.ID, domain.RoleAdmin)
	testutil.AddMember(t, s, w.space, w.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, s, w.space, w.viewer.ID, domain.RoleViewer)
	w.page = testutil.CreatePage(t, s, w.space, w.editor.ID, "Page one")
	w.otherPage = testutil.CreatePage(t, s, w.space, w.editor.ID, "Page two")
	return w
}

func (w *commentWorld) as(u domain.User) *client {
	w.t.Helper()
	c := w.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func commentBody(text string) map[string]any {
	return map[string]any{
		"body_json": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}},
			},
		},
		"body_html": "<p>" + text + "</p>",
	}
}

func (w *commentWorld) post(c *client, pageID uuid.UUID, body map[string]any) response {
	w.t.Helper()
	return c.do("POST", fmt.Sprintf("/api/v1/pages/%s/comments", pageID), body)
}

func (w *commentWorld) mustPost(c *client, pageID uuid.UUID, body map[string]any) string {
	w.t.Helper()
	r := w.post(c, pageID, body)
	if r.status != 201 {
		w.t.Fatalf("create comment: %d %s", r.status, r.raw)
	}
	return r.body["id"].(string)
}

func (w *commentWorld) list(c *client, pageID uuid.UUID) response {
	w.t.Helper()
	return c.do("GET", fmt.Sprintf("/api/v1/pages/%s/comments", pageID), nil)
}

func TestCommentViewerCanCommentAndList(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)

	r := w.post(viewer, w.page, commentBody("Looks good"))
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	for _, k := range []string{"id", "page_id", "parent_id", "author_id", "author", "body_json", "body_html", "inline_anchor", "resolved_at", "resolved_by", "created_at", "updated_at", "deleted_at", "replies"} {
		if _, ok := r.body[k]; !ok {
			t.Fatalf("missing key %s in %s", k, r.raw)
		}
	}
	if r.body["author_id"] != w.viewer.ID.String() || r.body["page_id"] != w.page.String() {
		t.Fatalf("ids: %s", r.raw)
	}
	author := r.body["author"].(map[string]any)
	if author["email"] != w.viewer.Email || author["id"] != w.viewer.ID.String() {
		t.Fatalf("author: %v", author)
	}
	if r.body["body_html"] != "<p>Looks good</p>" || r.body["inline_anchor"] != nil || r.body["deleted_at"] != nil {
		t.Fatalf("body: %s", r.raw)
	}
	if replies, ok := r.body["replies"].([]any); !ok || len(replies) != 0 {
		t.Fatalf("replies must be []: %s", r.raw)
	}

	l := w.list(w.as(w.editor), w.page)
	if l.status != 200 || len(l.list) != 1 || l.list[0]["id"] != r.body["id"] {
		t.Fatalf("list: %d %s", l.status, l.raw)
	}
	if l := w.list(viewer, w.otherPage); l.status != 200 || string(l.raw) != "[]\n" {
		t.Fatalf("empty list must be []: %q", l.raw)
	}
	if r := w.post(w.client(), w.page, commentBody("anon")); r.status != 401 {
		t.Fatalf("anon: %d", r.status)
	}
}

func TestCommentNonMemberOnPrivateSpaceHidden(t *testing.T) {
	w := newCommentWorld(t)
	id := w.mustPost(w.as(w.viewer), w.page, commentBody("hi"))
	out := w.as(w.outsider)

	if r := w.post(out, w.page, commentBody("sneaky")); r.status != 404 {
		t.Fatalf("post: %d %s", r.status, r.raw)
	}
	if r := w.list(out, w.page); r.status != 404 {
		t.Fatalf("list: %d %s", r.status, r.raw)
	}
	if r := out.do("PATCH", "/api/v1/comments/"+id, commentBody("x")); r.status != 404 {
		t.Fatalf("patch: %d", r.status)
	}
	if r := out.do("DELETE", "/api/v1/comments/"+id, nil); r.status != 404 {
		t.Fatalf("delete: %d", r.status)
	}
	if r := out.do("POST", "/api/v1/comments/"+id+"/resolve", nil); r.status != 404 {
		t.Fatalf("resolve: %d", r.status)
	}
}

func TestCommentInternalSpaceViewerImplicit(t *testing.T) {
	w := newCommentWorld(t)
	space := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "OPEN", CreatedBy: w.spaceAdm.ID})
	page := testutil.CreatePage(t, w.store, space, w.spaceAdm.ID, "Open page")
	out := w.as(w.outsider)
	if r := w.post(out, page, commentBody("anyone")); r.status != 201 {
		t.Fatalf("implicit viewer comment: %d %s", r.status, r.raw)
	}
}

func TestCommentBadIDsAndUnknowns(t *testing.T) {
	w := newCommentWorld(t)
	c := w.as(w.viewer)
	if r := c.do("GET", "/api/v1/pages/nope/comments", nil); r.status != 400 {
		t.Fatalf("bad page id: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/pages/"+uuid.NewString()+"/comments", nil); r.status != 404 {
		t.Fatalf("unknown page: %d", r.status)
	}
	if r := c.do("PATCH", "/api/v1/comments/nope", commentBody("x")); r.status != 400 {
		t.Fatalf("bad comment id: %d", r.status)
	}
	if r := c.do("DELETE", "/api/v1/comments/"+uuid.NewString(), nil); r.status != 404 {
		t.Fatalf("unknown comment: %d", r.status)
	}
}

func TestCommentValidation(t *testing.T) {
	w := newCommentWorld(t)
	c := w.as(w.viewer)

	if r := w.post(c, w.page, map[string]any{"body_json": nil, "body_html": ""}); r.status != 400 {
		t.Fatalf("empty: %d", r.status)
	}
	if r := w.post(c, w.page, map[string]any{"body_json": nil, "body_html": "<p>   </p>"}); r.status != 400 {
		t.Fatalf("whitespace: %d", r.status)
	}
	if r := w.post(c, w.page, map[string]any{"body_json": nil, "body_html": "<script>alert(1)</script>"}); r.status != 400 {
		t.Fatalf("script only: %d", r.status)
	}
	if r := w.post(c, w.page, map[string]any{"body_json": []any{1}, "body_html": "<p>x</p>"}); r.status != 400 {
		t.Fatalf("array json: %d", r.status)
	}
	body := commentBody("x")
	body["parent_id"] = "not-a-uuid"
	if r := w.post(c, w.page, body); r.status != 400 {
		t.Fatalf("bad parent: %d", r.status)
	}
	body["parent_id"] = uuid.NewString()
	if r := w.post(c, w.page, body); r.status != 400 {
		t.Fatalf("unknown parent: %d", r.status)
	}

	other := w.mustPost(c, w.otherPage, commentBody("elsewhere"))
	body["parent_id"] = other
	if r := w.post(c, w.page, body); r.status != 400 {
		t.Fatalf("parent on another page: %d", r.status)
	}

	jsonOnly := commentBody("json only")
	jsonOnly["body_html"] = ""
	if r := w.post(c, w.page, jsonOnly); r.status != 201 {
		t.Fatalf("json-only body: %d %s", r.status, r.raw)
	}
}

func TestCommentReplyThreading(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	editor := w.as(w.editor)

	root := w.mustPost(viewer, w.page, commentBody("root"))
	reply := commentBody("reply")
	reply["parent_id"] = root
	r1 := w.post(editor, w.page, reply)
	if r1.status != 201 || r1.body["parent_id"] != root {
		t.Fatalf("reply: %d %s", r1.status, r1.raw)
	}
	deeper := commentBody("reply to reply")
	deeper["parent_id"] = r1.body["id"]
	r2 := w.post(viewer, w.page, deeper)
	if r2.status != 201 {
		t.Fatalf("nested reply: %d %s", r2.status, r2.raw)
	}
	second := w.mustPost(viewer, w.page, commentBody("second root"))

	l := w.list(viewer, w.page)
	if l.status != 200 || len(l.list) != 2 || l.list[0]["id"] != root || l.list[1]["id"] != second {
		t.Fatalf("roots: %s", l.raw)
	}
	replies := l.list[0]["replies"].([]any)
	if len(replies) != 1 {
		t.Fatalf("replies: %s", l.raw)
	}
	first := replies[0].(map[string]any)
	if first["id"] != r1.body["id"] {
		t.Fatalf("first reply: %v", first)
	}
	nested := first["replies"].([]any)
	if len(nested) != 1 || nested[0].(map[string]any)["id"] != r2.body["id"] {
		t.Fatalf("nested: %v", first)
	}
}

func TestCommentAuthorOnlyEdit(t *testing.T) {
	w := newCommentWorld(t)
	id := w.mustPost(w.as(w.viewer), w.page, commentBody("original"))
	path := "/api/v1/comments/" + id

	for name, u := range map[string]domain.User{"editor": w.editor, "space admin": w.spaceAdm, "system admin": w.sysAdmin} {
		if r := w.as(u).do("PATCH", path, commentBody("hijack")); r.status != 403 {
			t.Fatalf("%s edit: %d %s", name, r.status, r.raw)
		}
	}

	r := w.as(w.viewer).do("PATCH", path, commentBody("edited"))
	if r.status != 200 {
		t.Fatalf("author edit: %d %s", r.status, r.raw)
	}
	if r.body["body_html"] != "<p>edited</p>" || r.body["id"] != id {
		t.Fatalf("edited: %s", r.raw)
	}
	if r.body["updated_at"] == r.body["created_at"] {
		t.Fatalf("updated_at must change: %s", r.raw)
	}
	if r := w.as(w.viewer).do("PATCH", path, map[string]any{"body_json": nil, "body_html": ""}); r.status != 400 {
		t.Fatalf("empty edit: %d", r.status)
	}
	l := w.list(w.as(w.editor), w.page)
	if l.list[0]["body_html"] != "<p>edited</p>" {
		t.Fatalf("persisted: %s", l.raw)
	}
}

func TestCommentDeletePermissions(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)

	own := w.mustPost(viewer, w.page, commentBody("mine"))
	if r := viewer.do("DELETE", "/api/v1/comments/"+own, nil); r.status != 204 {
		t.Fatalf("author delete: %d %s", r.status, r.raw)
	}

	theirs := w.mustPost(viewer, w.page, commentBody("viewer wrote this"))
	if r := w.as(w.editor).do("DELETE", "/api/v1/comments/"+theirs, nil); r.status != 403 {
		t.Fatalf("editor delete of another's comment: %d %s", r.status, r.raw)
	}
	if r := w.as(w.spaceAdm).do("DELETE", "/api/v1/comments/"+theirs, nil); r.status != 204 {
		t.Fatalf("space admin delete: %d %s", r.status, r.raw)
	}

	again := w.mustPost(viewer, w.page, commentBody("again"))
	if r := w.as(w.sysAdmin).do("DELETE", "/api/v1/comments/"+again, nil); r.status != 204 {
		t.Fatalf("system admin delete: %d %s", r.status, r.raw)
	}
	if r := w.as(w.sysAdmin).do("DELETE", "/api/v1/comments/"+again, nil); r.status != 204 {
		t.Fatalf("repeat delete is idempotent: %d", r.status)
	}
	if r := w.list(viewer, w.page); string(r.raw) != "[]\n" {
		t.Fatalf("all deleted leaves must vanish: %s", r.raw)
	}
	if r := viewer.do("PATCH", "/api/v1/comments/"+own, commentBody("zombie")); r.status != 404 {
		t.Fatalf("edit deleted: %d", r.status)
	}
}

func TestCommentSoftDeleteKeepsThreadShape(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	editor := w.as(w.editor)

	root := w.mustPost(viewer, w.page, commentBody("root"))
	reply := commentBody("reply")
	reply["parent_id"] = root
	replyID := w.mustPost(editor, w.page, reply)

	if r := viewer.do("DELETE", "/api/v1/comments/"+root, nil); r.status != 204 {
		t.Fatalf("delete root: %d", r.status)
	}
	l := w.list(editor, w.page)
	if l.status != 200 || len(l.list) != 1 {
		t.Fatalf("list: %s", l.raw)
	}
	got := l.list[0]
	if got["id"] != root || got["body_html"] != "" || got["body_json"] != nil || got["deleted_at"] == nil {
		t.Fatalf("deleted root shape: %s", l.raw)
	}
	replies := got["replies"].([]any)
	if len(replies) != 1 {
		t.Fatalf("reply must survive: %s", l.raw)
	}
	kept := replies[0].(map[string]any)
	if kept["id"] != replyID || kept["body_html"] != "<p>reply</p>" || kept["deleted_at"] != nil {
		t.Fatalf("kept reply: %v", kept)
	}

	late := commentBody("late")
	late["parent_id"] = root
	if r := w.post(editor, w.page, late); r.status != 400 {
		t.Fatalf("reply to deleted: %d", r.status)
	}

	if r := editor.do("DELETE", "/api/v1/comments/"+replyID, nil); r.status != 204 {
		t.Fatalf("delete reply: %d", r.status)
	}
	if r := w.list(editor, w.page); string(r.raw) != "[]\n" {
		t.Fatalf("fully deleted thread must vanish: %s", r.raw)
	}
}

func TestCommentResolveAndReopen(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	editor := w.as(w.editor)

	root := w.mustPost(viewer, w.page, commentBody("question"))
	reply := commentBody("answer")
	reply["parent_id"] = root
	replyID := w.mustPost(editor, w.page, reply)

	if r := editor.do("POST", "/api/v1/comments/"+replyID+"/resolve", nil); r.status != 400 {
		t.Fatalf("resolve reply: %d %s", r.status, r.raw)
	}
	if r := editor.do("POST", "/api/v1/comments/"+replyID+"/reopen", nil); r.status != 400 {
		t.Fatalf("reopen reply: %d", r.status)
	}

	r := editor.do("POST", "/api/v1/comments/"+root+"/resolve", nil)
	if r.status != 200 || r.body["resolved_at"] == nil || r.body["resolved_by"] != w.editor.ID.String() {
		t.Fatalf("resolve: %d %s", r.status, r.raw)
	}
	if len(r.body["replies"].([]any)) != 1 {
		t.Fatalf("resolve response keeps replies: %s", r.raw)
	}
	l := w.list(viewer, w.page)
	if len(l.list) != 1 || l.list[0]["resolved_at"] == nil {
		t.Fatalf("resolved threads stay listed: %s", l.raw)
	}

	editor.do("POST", "/api/v1/comments/"+root+"/resolve", nil)

	var resolved []domain.CommentResolved
	for _, e := range w.events.snapshot() {
		if ev, ok := e.(domain.CommentResolved); ok {
			resolved = append(resolved, ev)
		}
	}
	if len(resolved) != 1 {
		t.Fatalf("exactly one CommentResolved expected, got %d", len(resolved))
	}
	if resolved[0].CommentID.String() != root || resolved[0].PageID != w.page || resolved[0].SpaceID != w.space || resolved[0].ActorID != w.editor.ID {
		t.Fatalf("resolved event: %+v", resolved[0])
	}

	r = viewer.do("POST", "/api/v1/comments/"+root+"/reopen", nil)
	if r.status != 200 || r.body["resolved_at"] != nil || r.body["resolved_by"] != nil {
		t.Fatalf("reopen: %d %s", r.status, r.raw)
	}

	if r := viewer.do("DELETE", "/api/v1/comments/"+root, nil); r.status != 204 {
		t.Fatalf("delete root: %d", r.status)
	}
	if r := viewer.do("POST", "/api/v1/comments/"+root+"/resolve", nil); r.status != 404 {
		t.Fatalf("resolve deleted: %d", r.status)
	}
}

func TestCommentInlineAnchor(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)

	body := commentBody("Why this wording?")
	body["inline_anchor"] = map[string]any{"quoted_text": "  the quick brown fox  "}
	r := w.post(viewer, w.page, body)
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	anchor, ok := r.body["inline_anchor"].(map[string]any)
	if !ok || anchor["quoted_text"] != "the quick brown fox" || len(anchor) != 1 {
		t.Fatalf("anchor: %s", r.raw)
	}
	l := w.list(viewer, w.page)
	if got := l.list[0]["inline_anchor"].(map[string]any); got["quoted_text"] != "the quick brown fox" {
		t.Fatalf("list anchor: %s", l.raw)
	}
	plain := w.mustPost(viewer, w.page, commentBody("page level"))
	l = w.list(viewer, w.page)
	if l.list[1]["id"] != plain || l.list[1]["inline_anchor"] != nil {
		t.Fatalf("plain comment anchor must be null: %s", l.raw)
	}

	reply := commentBody("reply")
	reply["parent_id"] = r.body["id"]
	reply["inline_anchor"] = map[string]any{"quoted_text": "nope"}
	if r := w.post(viewer, w.page, reply); r.status != 400 {
		t.Fatalf("anchor on reply: %d", r.status)
	}
	empty := commentBody("x")
	empty["inline_anchor"] = map[string]any{"quoted_text": "   "}
	if r := w.post(viewer, w.page, empty); r.status != 400 {
		t.Fatalf("empty quote: %d", r.status)
	}
	long := commentBody("x")
	long["inline_anchor"] = map[string]any{"quoted_text": strings.Repeat("a", 2001)}
	if r := w.post(viewer, w.page, long); r.status != 400 {
		t.Fatalf("long quote: %d", r.status)
	}

	var created []domain.CommentCreated
	for _, e := range w.events.snapshot() {
		if ev, ok := e.(domain.CommentCreated); ok {
			created = append(created, ev)
		}
	}
	if len(created) < 2 || !created[0].Inline || created[1].Inline {
		t.Fatalf("Inline flag on events: %+v", created)
	}
}

func TestCommentInlineAnchorContext(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)

	body := commentBody("which one?")
	body["inline_anchor"] = map[string]any{"quoted_text": "fox", "prefix": " the quick brown ", "suffix": " jumps over"}
	r := w.post(viewer, w.page, body)
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	anchor := r.body["inline_anchor"].(map[string]any)
	if len(anchor) != 3 || anchor["quoted_text"] != "fox" || anchor["prefix"] != "the quick brown" || anchor["suffix"] != "jumps over" {
		t.Fatalf("anchor: %s", r.raw)
	}
	l := w.list(viewer, w.page)
	if got := l.list[0]["inline_anchor"].(map[string]any); got["prefix"] != "the quick brown" || got["suffix"] != "jumps over" {
		t.Fatalf("list anchor: %s", l.raw)
	}

	only := commentBody("no context")
	only["inline_anchor"] = map[string]any{"quoted_text": "fox", "prefix": "   "}
	r = w.post(viewer, w.page, only)
	if a := r.body["inline_anchor"].(map[string]any); r.status != 201 || len(a) != 1 || a["quoted_text"] != "fox" {
		t.Fatalf("blank context dropped: %d %s", r.status, r.raw)
	}

	exact := commentBody("limit")
	exact["inline_anchor"] = map[string]any{"quoted_text": "fox", "prefix": strings.Repeat("p", 64), "suffix": strings.Repeat("s", 64)}
	if r := w.post(viewer, w.page, exact); r.status != 201 {
		t.Fatalf("64 chars allowed: %d %s", r.status, r.raw)
	}
	for _, field := range []string{"prefix", "suffix"} {
		long := commentBody("too long")
		long["inline_anchor"] = map[string]any{"quoted_text": "fox", field: strings.Repeat("x", 65)}
		if r := w.post(viewer, w.page, long); r.status != 400 {
			t.Fatalf("%s over 64: %d", field, r.status)
		}
	}
}

func TestCommentCreatedEventCarriesMentions(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	root := w.mustPost(viewer, w.page, commentBody("root"))

	body := map[string]any{
		"body_json": map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{"type": "paragraph", "content": []any{
					map[string]any{"type": "text", "text": "ping "},
					map[string]any{"type": "mention", "attrs": map[string]any{"id": w.editor.ID.String(), "label": "Editor"}},
					map[string]any{"type": "text", "text": " and "},
					map[string]any{"type": "mention", "attrs": map[string]any{"id": w.spaceAdm.ID.String(), "label": "Admin"}},
					map[string]any{"type": "mention", "attrs": map[string]any{"id": w.editor.ID.String(), "label": "Editor"}},
				}},
			},
		},
		"body_html": "<p>ping</p>",
		"parent_id": root,
	}
	r := w.post(viewer, w.page, body)
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}

	var created []domain.CommentCreated
	for _, e := range w.events.snapshot() {
		if ev, ok := e.(domain.CommentCreated); ok {
			created = append(created, ev)
		}
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 CommentCreated events, got %d", len(created))
	}
	first, ev := created[0], created[1]
	if first.ParentID != nil || len(first.MentionedUserIDs) != 0 {
		t.Fatalf("first event: %+v", first)
	}
	if ev.CommentID.String() != r.body["id"] || ev.PageID != w.page || ev.SpaceID != w.space || ev.ActorID != w.viewer.ID {
		t.Fatalf("event ids: %+v", ev)
	}
	if ev.ParentID == nil || ev.ParentID.String() != root || ev.Inline {
		t.Fatalf("event parent/inline: %+v", ev)
	}
	if len(ev.MentionedUserIDs) != 2 || ev.MentionedUserIDs[0] != w.editor.ID || ev.MentionedUserIDs[1] != w.spaceAdm.ID {
		t.Fatalf("mentions: %v", ev.MentionedUserIDs)
	}
}

func TestCommentDraftPageHiddenFromNonCreators(t *testing.T) {
	w := newCommentWorld(t)
	draft := testutil.CreatePage(t, w.store, w.space, w.editor.ID, "Secret draft")
	if _, err := w.store.Pool.Exec(context.Background(), `UPDATE pages SET status = 'draft' WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	editor := w.as(w.editor)
	id := w.mustPost(editor, draft, commentBody("note to self"))

	viewer := w.as(w.viewer)
	if r := w.list(viewer, draft); r.status != 404 {
		t.Fatalf("viewer list: %d %s", r.status, r.raw)
	}
	if r := w.post(viewer, draft, commentBody("peek")); r.status != 404 {
		t.Fatalf("viewer post: %d", r.status)
	}
	if r := viewer.do("POST", "/api/v1/comments/"+id+"/resolve", nil); r.status != 404 {
		t.Fatalf("viewer resolve: %d", r.status)
	}
	if r := viewer.do("DELETE", "/api/v1/comments/"+id, nil); r.status != 404 {
		t.Fatalf("viewer delete: %d", r.status)
	}
	otherEditor := testutil.CreateUser(t, w.store, "editor2@example.test", "", false)
	testutil.AddMember(t, w.store, w.space, otherEditor.ID, domain.RoleEditor)
	if r := w.list(w.as(otherEditor), draft); r.status != 404 {
		t.Fatalf("other editor list: %d", r.status)
	}

	if r := w.list(editor, draft); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("creator list: %d %s", r.status, r.raw)
	}
	if r := w.list(w.as(w.spaceAdm), draft); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("space admin list: %d %s", r.status, r.raw)
	}
	if r := w.list(w.as(w.sysAdmin), draft); r.status != 200 {
		t.Fatalf("system admin list: %d", r.status)
	}
}

func TestCommentTrashedPage(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	id := w.mustPost(viewer, w.page, commentBody("before trash"))
	if _, err := w.store.Pool.Exec(context.Background(), `UPDATE pages SET status = 'trashed', trashed_at = now() WHERE id = $1`, w.page); err != nil {
		t.Fatal(err)
	}

	if r := w.list(viewer, w.page); r.status != 404 {
		t.Fatalf("viewer list: %d %s", r.status, r.raw)
	}
	for _, u := range []domain.User{w.editor, w.spaceAdm, w.sysAdmin} {
		if r := w.list(w.as(u), w.page); r.status != 200 || len(r.list) != 1 {
			t.Fatalf("%s list: %d %s", u.Email, r.status, r.raw)
		}
	}

	if r := w.post(viewer, w.page, commentBody("viewer write")); r.status != 404 {
		t.Fatalf("viewer post: %d", r.status)
	}

	if r := viewer.do("PATCH", "/api/v1/comments/"+id, commentBody("late")); r.status != 404 {
		t.Fatalf("viewer patch: %d", r.status)
	}
	if r := viewer.do("DELETE", "/api/v1/comments/"+id, nil); r.status != 404 {
		t.Fatalf("viewer delete: %d", r.status)
	}
}

func TestCommentWritesOnTrashedPageConflict(t *testing.T) {
	w := newCommentWorld(t)
	editor := w.as(w.editor)
	admin := w.as(w.spaceAdm)
	id := w.mustPost(editor, w.page, commentBody("mine"))
	if _, err := w.store.Pool.Exec(context.Background(), `UPDATE pages SET status = 'trashed', trashed_at = now() WHERE id = $1`, w.page); err != nil {
		t.Fatal(err)
	}
	check := func(name string, r response) {
		t.Helper()
		if r.status != 409 || r.body["code"] != "page_trashed" || r.body["error"] != "page is trashed" {
			t.Fatalf("%s: %d %s", name, r.status, r.raw)
		}
	}
	check("post", w.post(editor, w.page, commentBody("late")))
	check("patch", editor.do("PATCH", "/api/v1/comments/"+id, commentBody("late")))
	check("delete", admin.do("DELETE", "/api/v1/comments/"+id, nil))
	check("resolve", editor.do("POST", "/api/v1/comments/"+id+"/resolve", nil))
	check("reopen", editor.do("POST", "/api/v1/comments/"+id+"/reopen", nil))
}

func TestCommentReplyNestingCap(t *testing.T) {
	w := newCommentWorld(t)
	c := w.as(w.viewer)
	root := w.mustPost(c, w.page, commentBody("root"))
	reply := func(parent string) response {
		body := commentBody("reply")
		body["parent_id"] = parent
		return w.post(c, w.page, body)
	}
	l1 := reply(root)
	if l1.status != 201 {
		t.Fatalf("level 1: %d %s", l1.status, l1.raw)
	}
	l2 := reply(l1.body["id"].(string))
	if l2.status != 201 {
		t.Fatalf("level 2: %d %s", l2.status, l2.raw)
	}
	l3 := reply(l2.body["id"].(string))
	if l3.status != 400 || l3.body["error"] != "reply nesting too deep" {
		t.Fatalf("level 3: %d %s", l3.status, l3.raw)
	}
	if r := reply(root); r.status != 201 {
		t.Fatalf("sibling at level 1 still allowed: %d", r.status)
	}
}

func TestCommentSanitisesBodyHTML(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)

	body := commentBody("safe")
	body["body_html"] = `<p onclick="steal()">safe <script>alert(1)</script><a href="javascript:alert(2)">x</a></p>`
	r := w.post(viewer, w.page, body)
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	html := r.body["body_html"].(string)
	for _, bad := range []string{"<script", "alert(1)", "onclick", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("sanitiser let %q through: %s", bad, html)
		}
	}
	if !strings.Contains(html, "safe") {
		t.Fatalf("text lost: %s", html)
	}

	edit := commentBody("safe")
	edit["body_html"] = `<p>edited</p><img src="x" onerror="alert(3)"><script>alert(4)</script>`
	e := viewer.do("PATCH", "/api/v1/comments/"+r.body["id"].(string), edit)
	if e.status != 200 {
		t.Fatalf("edit: %d %s", e.status, e.raw)
	}
	html = e.body["body_html"].(string)
	if strings.Contains(html, "<script") || strings.Contains(html, "onerror") || !strings.Contains(html, "edited") {
		t.Fatalf("edit not sanitised: %s", html)
	}
}

func TestCommentArchivedSpaceIsReadOnly(t *testing.T) {
	w := newCommentWorld(t)
	viewer := w.as(w.viewer)
	id := w.mustPost(viewer, w.page, commentBody("before archive"))
	if _, err := w.store.Pool.Exec(context.Background(), `UPDATE spaces SET archived_at = now() WHERE id = $1`, w.space); err != nil {
		t.Fatal(err)
	}
	if r := w.post(viewer, w.page, commentBody("late")); r.status != 403 {
		t.Fatalf("post: %d", r.status)
	}
	if r := viewer.do("PATCH", "/api/v1/comments/"+id, commentBody("late")); r.status != 403 {
		t.Fatalf("patch: %d", r.status)
	}
	if r := viewer.do("DELETE", "/api/v1/comments/"+id, nil); r.status != 403 {
		t.Fatalf("delete: %d", r.status)
	}
	if r := w.list(viewer, w.page); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("list stays readable: %d", r.status)
	}
}
