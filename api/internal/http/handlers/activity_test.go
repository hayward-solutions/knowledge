package handlers_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestSpaceActivityFeed(t *testing.T) {
	w := newNWWorld(t)
	for i := 0; i < 3; i++ {
		w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID, Version: i + 1})
		time.Sleep(5 * time.Millisecond)
	}
	w.publish(domain.PageTrashed{PageID: w.page, SpaceID: w.space, ActorID: w.spaceAdm.ID})

	c := w.as(w.viewer)
	r := c.do("GET", "/api/v1/spaces/priv/activity", nil)
	if r.status != 200 || len(r.list) != 4 {
		t.Fatalf("feed: %d %s", r.status, r.raw)
	}
	top := r.list[0]
	for _, k := range []string{"id", "space_id", "page_id", "page_title", "actor_id", "actor", "type", "payload", "created_at"} {
		if _, ok := top[k]; !ok {
			t.Fatalf("missing %s in %v", k, top)
		}
	}
	if top["type"] != "page.trashed" || top["page_title"] != "Roadmap" || top["space_key"] != "PRIV" {
		t.Fatalf("top: %v", top)
	}
	if top["actor"].(map[string]any)["id"] != w.spaceAdm.ID.String() {
		t.Fatalf("actor: %v", top["actor"])
	}
	for i := 1; i < len(r.list); i++ {
		if r.list[i-1]["created_at"].(string) < r.list[i]["created_at"].(string) {
			t.Fatal("expected newest first")
		}
	}

	r = c.do("GET", "/api/v1/spaces/PRIV/activity?limit=2", nil)
	if len(r.list) != 2 {
		t.Fatalf("limit: %d", len(r.list))
	}
	before := url.QueryEscape(r.list[1]["created_at"].(string))
	r = c.do("GET", "/api/v1/spaces/PRIV/activity?limit=2&before="+before, nil)
	if len(r.list) != 2 || r.list[0]["type"] != "page.published" {
		t.Fatalf("page 2: %s", r.raw)
	}
	if r := c.do("GET", "/api/v1/spaces/PRIV/activity?before=zzz", nil); r.status != 400 {
		t.Fatalf("bad before: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/spaces/PRIV/activity?limit=0", nil); len(r.list) != 1 {
		t.Fatalf("limit floor: %d", len(r.list))
	}
	if r := c.do("GET", "/api/v1/spaces/PRIV/activity?limit=500", nil); len(r.list) != 4 {
		t.Fatalf("limit ceiling: %d", len(r.list))
	}
}

func TestSpaceActivityAccessRules(t *testing.T) {
	w := newNWWorld(t)
	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID, Version: 1})

	if r := w.as(w.outsider).do("GET", "/api/v1/spaces/PRIV/activity", nil); r.status != 404 {
		t.Fatalf("private non-member: %d", r.status)
	}
	if r := w.as(w.viewer).do("GET", "/api/v1/spaces/NOPE/activity", nil); r.status != 404 {
		t.Fatalf("unknown space: %d", r.status)
	}
	if r := w.as(w.sysAdmin).do("GET", "/api/v1/spaces/PRIV/activity", nil); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("system admin: %d", r.status)
	}
	empty := w.as(w.outsider).do("GET", "/api/v1/spaces/OPEN/activity", nil)
	if empty.status != 200 || string(empty.raw) != "[]\n" {
		t.Fatalf("internal space empty feed: %d %q", empty.status, empty.raw)
	}
}

func TestSpaceActivityHidesDraftPages(t *testing.T) {
	w := newNWWorld(t)
	draft := testutil.CreatePage(t, w.store, w.space, w.editor.ID, "Hidden draft")
	if _, err := w.store.Pool.Exec(t.Context(), `UPDATE pages SET status = 'draft' WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	w.publish(domain.PageMoved{PageID: draft, SpaceID: w.space, ActorID: w.editor.ID})
	w.publish(domain.PageMoved{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID})

	count := func(u domain.User) int {
		return len(w.as(u).do("GET", "/api/v1/spaces/PRIV/activity", nil).list)
	}
	if n := count(w.viewer); n != 1 {
		t.Fatalf("viewer sees %d", n)
	}
	if n := count(w.editor); n != 2 {
		t.Fatalf("creator sees %d", n)
	}
	if n := count(w.spaceAdm); n != 2 {
		t.Fatalf("space admin sees %d", n)
	}
}

func TestSpaceActivityHidesTrashedAndPurgedPages(t *testing.T) {
	w := newNWWorld(t)
	trashed := testutil.CreatePage(t, w.store, w.space, w.editor.ID, "Trashed published")
	purged := testutil.CreatePage(t, w.store, w.space, w.editor.ID, "Purged")
	if _, err := w.store.Pool.Exec(t.Context(),
		`UPDATE pages SET status = 'trashed', version = 1, published_at = now() WHERE id = $1`, trashed); err != nil {
		t.Fatal(err)
	}
	w.publish(domain.PagePublished{PageID: w.page, SpaceID: w.space, ActorID: w.editor.ID, Version: 1})
	w.publish(domain.PagePublished{PageID: trashed, SpaceID: w.space, ActorID: w.editor.ID, Version: 1})
	w.publish(domain.PagePublished{PageID: purged, SpaceID: w.space, ActorID: w.editor.ID, Version: 1})
	if _, err := w.store.Pool.Exec(t.Context(), `DELETE FROM pages WHERE id = $1`, purged); err != nil {
		t.Fatal(err)
	}

	titles := func(u domain.User) []string {
		out := []string{}
		for _, a := range w.as(u).do("GET", "/api/v1/spaces/PRIV/activity", nil).list {
			out = append(out, a["page_title"].(string))
		}
		return out
	}
	if got := titles(w.viewer); len(got) != 1 || got[0] != "Roadmap" {
		t.Fatalf("viewer: %v", got)
	}
	if got := titles(w.editor); len(got) != 2 {
		t.Fatalf("editor: %v", got)
	}
	if got := titles(w.spaceAdm); len(got) != 2 {
		t.Fatalf("space admin: %v", got)
	}
}
