package handlers_test

import (
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

func TestPagesVersionsListGetRestore(t *testing.T) {
	w := pagesSetup(t)
	p := w.create("ed1", "Spec", nil)
	id := pagesID(p)

	doc := func(text string) map[string]any {
		return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}}
	}
	publish := func(name, title, text string, base int, msg string) {
		body := map[string]any{"title": title, "content_json": doc(text), "content_html": "<p>" + text + "</p>", "base_version": base}
		if msg != "" {
			body["message"] = msg
		}
		if r := w.as(name).do("POST", pagesPath(id, "/publish"), body); r.status != 200 {
			t.Fatalf("publish %s: %d %s", title, r.status, r.raw)
		}
	}
	publish("ed1", "Spec v1", "alpha", 0, "initial")
	publish("ed2", "Spec v2", "beta", 1, "")
	publish("ed1", "Spec v3", "gamma", 2, "tweaks")

	r := w.as("viewer").do("GET", pagesPath(id, "/versions"), nil)
	if r.status != 200 || len(r.list) != 3 {
		t.Fatalf("list: %d %s", r.status, r.raw)
	}
	if r.list[0]["version"].(float64) != 3 || r.list[2]["version"].(float64) != 1 {
		t.Fatalf("newest first: %s", r.raw)
	}
	for _, v := range r.list {
		if _, ok := v["content_json"]; ok {
			t.Fatalf("summary must omit content_json: %v", v)
		}
		if _, ok := v["content_html"]; ok {
			t.Fatalf("summary must omit content_html: %v", v)
		}
		for _, key := range []string{"id", "page_id", "version", "title", "author_id", "author", "message", "created_at"} {
			if _, ok := v[key]; !ok {
				t.Errorf("summary missing %q", key)
			}
		}
	}
	if r.list[1]["author"].(map[string]any)["id"] != w.users["ed2"].ID.String() || r.list[1]["message"] != nil || r.list[2]["message"] != "initial" {
		t.Fatalf("authors and messages: %s", r.raw)
	}

	v := w.as("viewer").do("GET", pagesPath(id, "/versions/1"), nil)
	if v.status != 200 || v.body["title"] != "Spec v1" || v.body["content_html"] != "<p>alpha</p>" || v.body["content_json"] == nil ||
		v.body["author"].(map[string]any)["id"] != w.users["ed1"].ID.String() || v.body["version"].(float64) != 1 {
		t.Fatalf("get: %d %s", v.status, v.raw)
	}
	if r := w.as("viewer").do("GET", pagesPath(id, "/versions/9"), nil); r.status != 404 {
		t.Fatalf("missing version: %d", r.status)
	}
	for _, bad := range []string{"abc", "0", "-1"} {
		if r := w.as("viewer").do("GET", pagesPath(id, "/versions/"+bad), nil); r.status != 400 {
			t.Fatalf("bad version %q: %d", bad, r.status)
		}
	}
	if r := w.as("outsider").do("GET", pagesPath(id, "/versions"), nil); r.status != 404 {
		t.Fatalf("outsider: %d", r.status)
	}
	if r := w.as("viewer").do("POST", pagesPath(id, "/versions/1/restore"), nil); r.status != 403 {
		t.Fatalf("viewer restore: %d", r.status)
	}

	w.as("ed2").do("PUT", pagesPath(id, "/draft"), map[string]any{"title": "mine", "content_json": nil, "content_html": "", "base_version": 3})
	r2 := w.as("ed2").do("POST", pagesPath(id, "/versions/1/restore"), nil)
	if r2.status != 200 || r2.body["version"].(float64) != 4 || r2.body["title"] != "Spec v1" || r2.body["content_html"] != "<p>alpha</p>" ||
		r2.body["status"] != "published" || r2.body["updated_by"] != w.users["ed2"].ID.String() {
		t.Fatalf("restore: %d %s", r2.status, r2.raw)
	}
	if n := w.dbInt(`SELECT count(*) FROM page_drafts WHERE page_id = $1 AND user_id = $2`, id, w.users["ed2"].ID); n != 0 {
		t.Fatalf("restore clears the caller's draft: %d", n)
	}
	latest := w.as("ed1").do("GET", pagesPath(id, "/versions/4"), nil)
	if latest.status != 200 || latest.body["message"] != "Restored version 1" || latest.body["title"] != "Spec v1" {
		t.Fatalf("restored version row: %d %s", latest.status, latest.raw)
	}
	var text string
	if err := w.store.Pool.QueryRow(t.Context(), `SELECT content_text FROM pages WHERE id = $1`, id).Scan(&text); err != nil || text != "alpha" {
		t.Fatalf("content_text re-extracted: %q %v", text, err)
	}

	if r := w.as("ed1").do("POST", pagesPath(id, "/versions/99/restore"), nil); r.status != 404 {
		t.Fatalf("restore missing: %d", r.status)
	}

	w.bus.Wait()
	var last *domain.PagePublished
	for _, e := range w.events.all() {
		if ev, ok := e.(domain.PagePublished); ok && ev.Version == 4 {
			last = &ev
		}
	}
	if last == nil || last.ActorID != w.users["ed2"].ID || last.PageID.String() != id {
		t.Fatalf("restore must publish PagePublished: %+v", w.events.all())
	}
}

func TestPagesVersionsHiddenForOtherUsersDrafts(t *testing.T) {
	w := pagesSetup(t)
	id := pagesID(w.create("ed1", "Hidden", nil))
	if r := w.as("ed2").do("GET", pagesPath(id, "/versions"), nil); r.status != 404 {
		t.Fatalf("ed2: %d", r.status)
	}
	if r := w.as("ed2").do("POST", pagesPath(id, "/versions/1/restore"), nil); r.status != 404 {
		t.Fatalf("ed2 restore: %d", r.status)
	}
	if r := w.as("ed1").do("GET", pagesPath(id, "/versions"), nil); r.status != 200 || string(r.raw) != "[]\n" {
		t.Fatalf("empty list must be []: %d %q", r.status, r.raw)
	}
}
