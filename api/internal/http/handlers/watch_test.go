package handlers_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestWatchPageLifecycle(t *testing.T) {
	w := newNWWorld(t)
	c := w.as(w.viewer)
	path := fmt.Sprintf("/api/v1/watch/page/%s", w.page)

	if r := c.do("GET", path, nil); r.status != 200 || r.body["watching"] != false {
		t.Fatalf("initial: %d %s", r.status, r.raw)
	}
	for i := 0; i < 2; i++ {
		if r := c.do("PUT", path, nil); r.status != 200 || r.body["watching"] != true {
			t.Fatalf("put: %d %s", r.status, r.raw)
		}
	}
	if r := c.do("GET", path, nil); r.body["watching"] != true {
		t.Fatalf("after put: %s", r.raw)
	}
	if r := w.as(w.editor).do("GET", path, nil); r.body["watching"] != false {
		t.Fatalf("other user's state: %s", r.raw)
	}
	if r := c.do("GET", fmt.Sprintf("/api/v1/pages/%s", w.page), nil); r.body["watching"] != true {
		t.Fatalf("page detail watching: %v", r.body["watching"])
	}
	for i := 0; i < 2; i++ {
		if r := c.do("DELETE", path, nil); r.status != 200 || r.body["watching"] != false {
			t.Fatalf("delete: %d %s", r.status, r.raw)
		}
	}
	if r := c.do("GET", fmt.Sprintf("/api/v1/pages/%s", w.page), nil); r.body["watching"] != false {
		t.Fatalf("page detail after delete: %v", r.body["watching"])
	}
}

func TestWatchSpace(t *testing.T) {
	w := newNWWorld(t)
	c := w.as(w.viewer)
	path := fmt.Sprintf("/api/v1/watch/space/%s", w.space)
	if r := c.do("PUT", path, nil); r.status != 200 || r.body["watching"] != true {
		t.Fatalf("put: %d %s", r.status, r.raw)
	}
	if r := c.do("GET", path, nil); r.body["watching"] != true {
		t.Fatalf("get: %s", r.raw)
	}
	page := fmt.Sprintf("/api/v1/watch/page/%s", w.page)
	if r := c.do("GET", page, nil); r.body["watching"] != false {
		t.Fatal("space watch must not mark the page as watched")
	}
	if r := c.do("DELETE", path, nil); r.body["watching"] != false {
		t.Fatalf("delete: %s", r.raw)
	}

	openPath := fmt.Sprintf("/api/v1/watch/space/%s", w.internal)
	if r := w.as(w.outsider).do("PUT", openPath, nil); r.status != 200 {
		t.Fatalf("internal space is watchable by any signed-in user: %d", r.status)
	}
}

func TestWatchAccessRules(t *testing.T) {
	w := newNWWorld(t)
	outsider := w.as(w.outsider)
	pagePath := fmt.Sprintf("/api/v1/watch/page/%s", w.page)
	spacePath := fmt.Sprintf("/api/v1/watch/space/%s", w.space)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if r := outsider.do(method, pagePath, nil); r.status != 404 {
			t.Fatalf("%s private page: %d", method, r.status)
		}
		if r := outsider.do(method, spacePath, nil); r.status != 404 {
			t.Fatalf("%s private space: %d", method, r.status)
		}
	}
	if ok, _ := w.store.IsWatching(t.Context(), w.outsider.ID, domain.WatchTargetPage, w.page); ok {
		t.Fatal("rejected PUT must not create a watch")
	}

	viewer := w.as(w.viewer)
	if r := viewer.do("PUT", "/api/v1/watch/comment/"+w.page.String(), nil); r.status != 400 {
		t.Fatalf("bad type: %d", r.status)
	}
	if r := viewer.do("PUT", "/api/v1/watch/page/nope", nil); r.status != 400 {
		t.Fatalf("bad id: %d", r.status)
	}
	if r := viewer.do("PUT", "/api/v1/watch/page/"+uuid.Must(uuid.NewV7()).String(), nil); r.status != 404 {
		t.Fatalf("missing page: %d", r.status)
	}
	if r := viewer.do("PUT", "/api/v1/watch/space/"+uuid.Must(uuid.NewV7()).String(), nil); r.status != 404 {
		t.Fatalf("missing space: %d", r.status)
	}

	root := w.as(w.sysAdmin)
	if r := root.do("PUT", pagePath, nil); r.status != 200 {
		t.Fatalf("system admin: %d", r.status)
	}
}

func TestWatchDraftPagesAndDeletedSpaces(t *testing.T) {
	w := newNWWorld(t)
	draft := testutil.CreatePage(t, w.store, w.space, w.editor.ID, "Draft")
	if _, err := w.store.Pool.Exec(t.Context(), `UPDATE pages SET status = 'draft' WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	other := testutil.CreateUser(t, w.store, "otherEditor@example.test", "", false)
	testutil.AddMember(t, w.store, w.space, other.ID, domain.RoleEditor)
	path := fmt.Sprintf("/api/v1/watch/page/%s", draft)

	if r := w.as(other).do("PUT", path, nil); r.status != 404 {
		t.Fatalf("non-creator editor on draft: %d", r.status)
	}
	if r := w.as(w.viewer).do("PUT", path, nil); r.status != 404 {
		t.Fatalf("viewer on draft: %d", r.status)
	}
	if r := w.as(w.editor).do("PUT", path, nil); r.status != 200 {
		t.Fatalf("creator on draft: %d", r.status)
	}
	if r := w.as(w.spaceAdm).do("PUT", path, nil); r.status != 200 {
		t.Fatalf("space admin on draft: %d", r.status)
	}

	gone := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "GONE", Deleted: true, CreatedBy: w.spaceAdm.ID})
	goneP := fmt.Sprintf("/api/v1/watch/space/%s", gone)
	if r := w.as(w.viewer).do("PUT", goneP, nil); r.status != 404 {
		t.Fatalf("deleted space: %d", r.status)
	}
}
