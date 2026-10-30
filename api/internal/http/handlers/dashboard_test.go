package handlers_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func dashTitlesOf(v any) []string {
	out := []string{}
	for _, item := range v.([]any) {
		out = append(out, item.(map[string]any)["title"].(string))
	}
	return out
}

func dashContains(list []string, title string) bool {
	for _, t := range list {
		if t == title {
			return true
		}
	}
	return false
}

func TestDashboardShapeAndSections(t *testing.T) {
	w := newNWWorld(t)
	openPage := testutil.CreatePage(t, w.store, w.internal, w.editor.ID, "Open page")
	c := w.as(w.viewer)

	if r := c.do("POST", fmt.Sprintf("/api/v1/pages/%s/star", w.page), nil); r.status != 204 {
		t.Fatalf("star: %d %s", r.status, r.raw)
	}
	if r := c.do("GET", fmt.Sprintf("/api/v1/pages/%s", openPage), nil); r.status != 200 {
		t.Fatalf("view: %d", r.status)
	}
	own := testutil.CreatePage(t, w.store, w.internal, w.viewer.ID, "My draft")
	if _, err := w.store.Pool.Exec(t.Context(), `UPDATE pages SET status = 'draft' WHERE id = $1`, own); err != nil {
		t.Fatal(err)
	}

	r := c.do("GET", "/api/v1/me/dashboard", nil)
	if r.status != 200 {
		t.Fatalf("dashboard: %d %s", r.status, r.raw)
	}
	for _, k := range []string{"recent_updates", "recently_viewed", "my_drafts", "starred"} {
		if _, ok := r.body[k].([]any); !ok {
			t.Fatalf("section %s missing or not an array: %s", k, r.raw)
		}
	}
	recent := dashTitlesOf(r.body["recent_updates"])
	if !dashContains(recent, "Roadmap") || !dashContains(recent, "Open page") || dashContains(recent, "My draft") {
		t.Fatalf("recent: %v", recent)
	}
	if got := dashTitlesOf(r.body["recently_viewed"]); len(got) != 1 || got[0] != "Open page" {
		t.Fatalf("viewed: %v", got)
	}
	if got := dashTitlesOf(r.body["starred"]); len(got) != 1 || got[0] != "Roadmap" {
		t.Fatalf("starred: %v", got)
	}
	drafts := r.body["my_drafts"].([]any)
	if len(drafts) != 1 {
		t.Fatalf("drafts: %v", drafts)
	}
	d := drafts[0].(map[string]any)
	for _, k := range []string{"id", "title", "slug", "parent_id", "position", "status", "has_children", "space_key", "space_name", "updated_at", "author"} {
		if _, ok := d[k]; !ok {
			t.Fatalf("missing %s in %v", k, d)
		}
	}
	if d["title"] != "My draft" || d["space_key"] != "OPEN" || d["status"] != "draft" || d["author"].(map[string]any)["id"] != w.viewer.ID.String() {
		t.Fatalf("draft: %v", d)
	}
}

func TestDashboardAccessRules(t *testing.T) {
	w := newNWWorld(t)
	testutil.CreatePage(t, w.store, w.internal, w.editor.ID, "Open page")

	out := w.as(w.outsider).do("GET", "/api/v1/me/dashboard", nil)
	recent := dashTitlesOf(out.body["recent_updates"])
	if dashContains(recent, "Roadmap") || !dashContains(recent, "Open page") {
		t.Fatalf("outsider recent: %v", recent)
	}

	member := w.as(w.viewer).do("GET", "/api/v1/me/dashboard", nil)
	if recent := dashTitlesOf(member.body["recent_updates"]); !dashContains(recent, "Roadmap") || !dashContains(recent, "Open page") {
		t.Fatalf("member recent: %v", recent)
	}

	if _, err := w.store.Pool.Exec(t.Context(), `INSERT INTO page_stars (user_id, page_id) VALUES ($1, $2)`, w.outsider.ID, w.page); err != nil {
		t.Fatal(err)
	}
	if _, err := w.store.Pool.Exec(t.Context(), `INSERT INTO page_views (user_id, page_id) VALUES ($1, $2)`, w.outsider.ID, w.page); err != nil {
		t.Fatal(err)
	}
	out = w.as(w.outsider).do("GET", "/api/v1/me/dashboard", nil)
	if len(out.body["starred"].([]any)) != 0 || len(out.body["recently_viewed"].([]any)) != 0 {
		t.Fatalf("stale star/view on an inaccessible space leaked: %s", out.raw)
	}

	gone := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "GONE", Deleted: true, CreatedBy: w.spaceAdm.ID})
	testutil.CreatePage(t, w.store, gone, w.editor.ID, "Deleted space page")
	root := w.as(w.sysAdmin).do("GET", "/api/v1/me/dashboard", nil)
	titles := dashTitlesOf(root.body["recent_updates"])
	if !dashContains(titles, "Roadmap") || dashContains(titles, "Deleted space page") {
		t.Fatalf("system admin recent: %v", titles)
	}
}

func TestDashboardTrashedAndHiddenDraftsExcluded(t *testing.T) {
	w := newNWWorld(t)
	trashed := testutil.CreatePage(t, w.store, w.internal, w.editor.ID, "Trashed")
	theirs := testutil.CreatePage(t, w.store, w.internal, w.editor.ID, "Their draft")
	for _, q := range []struct {
		status string
		id     uuid.UUID
	}{{"trashed", trashed}, {"draft", theirs}} {
		if _, err := w.store.Pool.Exec(t.Context(), `UPDATE pages SET status = $1 WHERE id = $2`, q.status, q.id); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"page_stars", "page_views"} {
			if _, err := w.store.Pool.Exec(t.Context(), fmt.Sprintf(`INSERT INTO %s (user_id, page_id) VALUES ($1, $2)`, table), w.viewer.ID, q.id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := w.store.Pool.Exec(t.Context(),
		`INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, 'viewer')`, w.internal, w.viewer.ID); err != nil {
		t.Fatal(err)
	}
	r := w.as(w.viewer).do("GET", "/api/v1/me/dashboard", nil)
	for _, k := range []string{"recent_updates", "recently_viewed", "starred", "my_drafts"} {
		if got := dashTitlesOf(r.body[k]); dashContains(got, "Trashed") || dashContains(got, "Their draft") {
			t.Fatalf("%s leaked: %v", k, got)
		}
	}
	admin := w.as(w.sysAdmin)
	if got := dashTitlesOf(admin.do("GET", "/api/v1/me/dashboard", nil).body["recent_updates"]); dashContains(got, "Their draft") {
		t.Fatalf("drafts are not recent updates: %v", got)
	}
}
