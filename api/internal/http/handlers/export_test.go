package handlers_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type exportWorld struct {
	*env
	space     uuid.UUID
	home      uuid.UUID
	users     map[string]domain.User
	clients   map[string]*client
	mu        sync.Mutex
	published []domain.PagePublished
	bus       *events.Bus
}

func exportSetup(t *testing.T, mutate func(*config.Config)) *exportWorld {
	t.Helper()
	s := testutil.NewDB(t)
	cfg := &config.Config{AppOrigins: []string{"http://app.test"}}
	if mutate != nil {
		mutate(cfg)
	}
	bus := events.NewBus()
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store: s, Bus: bus, Hub: events.NewHub(), Presence: presence, Config: cfg, Version: "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	w := &exportWorld{
		env:     &env{t: t, store: s, server: srv},
		users:   map[string]domain.User{},
		clients: map[string]*client{},
		bus:     bus,
	}
	bus.Subscribe(func(e any) {
		if p, ok := e.(domain.PagePublished); ok {
			w.mu.Lock()
			w.published = append(w.published, p)
			w.mu.Unlock()
		}
	})
	for _, name := range []string{"sys", "spaceadmin", "ed1", "ed2", "viewer", "outsider"} {
		w.users[name] = testutil.CreateUser(t, s, name+"@example.test", "", name == "sys")
	}
	w.space = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", Visibility: domain.VisibilityPrivate, CreatedBy: w.users["sys"].ID})
	testutil.AddMember(t, s, w.space, w.users["spaceadmin"].ID, domain.RoleAdmin)
	testutil.AddMember(t, s, w.space, w.users["ed1"].ID, domain.RoleEditor)
	testutil.AddMember(t, s, w.space, w.users["ed2"].ID, domain.RoleEditor)
	testutil.AddMember(t, s, w.space, w.users["viewer"].ID, domain.RoleViewer)
	w.home = w.page("sys", "Home", nil, "", nil)
	if _, err := s.Pool.Exec(context.Background(), `UPDATE spaces SET home_page_id = $2, name = 'Engineering' WHERE id = $1`, w.space, w.home); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *exportWorld) as(name string) *client {
	w.t.Helper()
	if c, ok := w.clients[name]; ok {
		return c
	}
	c := w.client()
	c.mustLogin(name+"@example.test", testutil.DefaultPassword)
	w.clients[name] = c
	return c
}

func (w *exportWorld) page(author, title string, parent *uuid.UUID, html string, doc any) uuid.UUID {
	w.t.Helper()
	ctx := context.Background()
	p, err := w.store.CreatePage(ctx, store.NewPage{SpaceID: w.space, ParentID: parent, Title: title, CreatedBy: w.users[author].ID})
	if err != nil {
		w.t.Fatal(err)
	}
	var raw json.RawMessage
	if doc != nil {
		raw, _ = json.Marshal(doc)
	}
	if _, err := w.store.PublishPage(ctx, store.PublishParams{
		PageID: p.ID, ActorID: w.users[author].ID, Title: title, ContentJSON: raw, ContentHTML: html,
	}); err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

func (w *exportWorld) draft(author, title string, parent *uuid.UUID) uuid.UUID {
	w.t.Helper()
	p, err := w.store.CreatePage(context.Background(), store.NewPage{
		SpaceID: w.space, ParentID: parent, Title: title, CreatedBy: w.users[author].ID,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

func exportDoc(text string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "heading", "attrs": map[string]any{"level": 2}, "content": []any{map[string]any{"type": "text", "text": "Section"}}},
		map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": text, "marks": []any{map[string]any{"type": "bold"}}},
		}},
		map[string]any{"type": "taskList", "content": []any{
			map[string]any{"type": "taskItem", "attrs": map[string]any{"checked": true}, "content": []any{
				map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "done"}}},
			}},
		}},
	}}
}

func exportPath(id uuid.UUID, format string) string {
	return "/api/v1/pages/" + id.String() + "/export?format=" + format
}

func TestExportPageMarkdown(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.page("ed1", "Release Notes", &w.home, "<p>ignored</p>", exportDoc("shipped"))
	r := w.as("viewer").do("GET", exportPath(id, "md"), nil)
	if r.status != 200 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	if ct := r.header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Fatalf("content-type %q", ct)
	}
	if cd := r.header.Get("Content-Disposition"); cd != `attachment; filename="release-notes.md"` {
		t.Fatalf("disposition %q", cd)
	}
	want := "# Release Notes\n\n## Section\n\n**shipped**\n\n- [x] done\n"
	if string(r.raw) != want {
		t.Fatalf("body %q want %q", r.raw, want)
	}
	if r2 := w.as("viewer").do("GET", "/api/v1/pages/"+id.String()+"/export", nil); r2.status != 200 || string(r2.raw) != want {
		t.Fatalf("default format: %d %q", r2.status, r2.raw)
	}
}

func TestExportPageMarkdownFromHTMLWhenJSONNull(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.page("ed1", "Imported", &w.home, `<h2>Head</h2><p>Hello <strong>there</strong></p><ul data-type="taskList"><li data-type="taskItem" data-checked="false"><p>todo</p></li></ul>`, nil)
	r := w.as("ed1").do("GET", exportPath(id, "md"), nil)
	want := "# Imported\n\n## Head\n\nHello **there**\n\n- [ ] todo\n"
	if r.status != 200 || string(r.raw) != want {
		t.Fatalf("%d %q", r.status, r.raw)
	}
}

func TestExportPageHTML(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.page("ed1", "Panels & Co", &w.home, `<div data-type="panel" data-color="warning"><p>Heads up</p></div><script>alert(1)</script>`, nil)
	r := w.as("viewer").do("GET", exportPath(id, "html"), nil)
	if r.status != 200 {
		t.Fatalf("status %d", r.status)
	}
	if ct := r.header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type %q", ct)
	}
	if cd := r.header.Get("Content-Disposition"); cd != `attachment; filename="panels-co.html"` {
		t.Fatalf("disposition %q", cd)
	}
	body := string(r.raw)
	for _, want := range []string{"<!doctype html>", "<title>Panels &amp; Co</title>", `<div data-type="panel" data-color="warning"><p>Heads up</p></div>`, "<style>"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "alert(1)") {
		t.Fatal("script leaked")
	}
}

func TestExportPageValidationAndAccess(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.page("ed1", "Doc", &w.home, "<p>x</p>", nil)
	if r := w.as("viewer").do("GET", exportPath(id, "pdf"), nil); r.status != 400 {
		t.Fatalf("bad format %d", r.status)
	}
	if r := w.as("outsider").do("GET", exportPath(id, "md"), nil); r.status != 404 {
		t.Fatalf("outsider %d", r.status)
	}
	if r := w.client().do("GET", exportPath(id, "md"), nil); r.status != 401 {
		t.Fatalf("anon %d", r.status)
	}
	if r := w.as("viewer").do("GET", exportPath(uuid.New(), "md"), nil); r.status != 404 {
		t.Fatalf("missing %d", r.status)
	}
}

func TestExportHiddenDraft404(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.draft("ed1", "Secret plan", &w.home)
	for _, name := range []string{"ed2", "viewer", "outsider"} {
		if r := w.as(name).do("GET", exportPath(id, "md"), nil); r.status != 404 {
			t.Fatalf("%s: %d", name, r.status)
		}
		if r := w.as(name).do("GET", exportPath(id, "html"), nil); r.status != 404 {
			t.Fatalf("%s html: %d", name, r.status)
		}
	}
	for _, name := range []string{"ed1", "spaceadmin", "sys"} {
		if r := w.as(name).do("GET", exportPath(id, "md"), nil); r.status != 200 {
			t.Fatalf("%s: %d", name, r.status)
		}
	}
}

func TestExportTrashedRequiresEditor(t *testing.T) {
	w := exportSetup(t, nil)
	id := w.page("ed1", "Old", &w.home, "<p>old</p>", nil)
	if err := w.store.TrashPage(context.Background(), id, w.users["ed1"].ID); err != nil {
		t.Fatal(err)
	}
	if r := w.as("viewer").do("GET", exportPath(id, "md"), nil); r.status != 404 {
		t.Fatalf("viewer %d", r.status)
	}
	if r := w.as("ed2").do("GET", exportPath(id, "md"), nil); r.status != 200 {
		t.Fatalf("editor %d", r.status)
	}
}

func exportZipNames(t *testing.T, raw []byte) (map[string]string, []string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	files := map[string]string{}
	var names []string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return files, names
}

func TestExportSpaceZip(t *testing.T) {
	w := exportSetup(t, nil)
	guides := w.page("ed1", "Guides", &w.home, "<p>guides index</p>", nil)
	w.page("ed1", "Setup", &guides, "<p>first</p>", exportDoc("one"))
	w.page("ed1", "Setup", &guides, "<p>second</p>", nil)
	w.page("ed1", "Notes", &w.home, "<p>notes</p>", nil)
	w.page("ed1", "README", &w.home, "<p>named readme</p>", nil)
	w.page("ed1", "Index", &guides, "<p>named index</p>", nil)
	w.draft("ed1", "Hidden draft", &w.home)
	gone := w.page("ed1", "Gone", &w.home, "<p>trashed</p>", nil)
	if err := w.store.TrashPage(context.Background(), gone, w.users["ed1"].ID); err != nil {
		t.Fatal(err)
	}

	r := w.as("viewer").do("GET", "/api/v1/spaces/eng/export", nil)
	if r.status != 200 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type %q", ct)
	}
	if cd := r.header.Get("Content-Disposition"); cd != `attachment; filename="ENG-export.zip"` {
		t.Fatalf("disposition %q", cd)
	}
	files, names := exportZipNames(t, r.raw)
	wantNames := []string{
		"README.md",
		"home/guides/index-2.md", "home/guides/index.md", "home/guides/setup-2.md", "home/guides/setup.md",
		"home/index.md", "home/notes.md", "home/readme.md",
	}
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("names:\n%v\nwant:\n%v", names, wantNames)
	}
	if files["home/guides/setup.md"] != "# Setup\n\n## Section\n\n**one**\n\n- [x] done\n" {
		t.Fatalf("setup.md = %q", files["home/guides/setup.md"])
	}
	if files["home/guides/setup-2.md"] != "# Setup\n\nsecond\n" {
		t.Fatalf("setup-2.md = %q", files["home/guides/setup-2.md"])
	}
	if files["home/guides/index.md"] != "# Guides\n\nguides index\n" {
		t.Fatalf("guides index = %q", files["home/guides/index.md"])
	}
	readme := files["README.md"]
	for _, want := range []string{"# Engineering export", "- [Home](home/index.md)", "  - [Guides](home/guides/index.md)", "    - [Setup](home/guides/setup.md)", "  - [Notes](home/notes.md)"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing %q:\n%s", want, readme)
		}
	}
	for _, bad := range []string{"Hidden draft", "Gone"} {
		if strings.Contains(readme, bad) {
			t.Fatalf("README leaked %q", bad)
		}
	}
}

func TestExportSpaceZipAccess(t *testing.T) {
	w := exportSetup(t, nil)
	if r := w.as("outsider").do("GET", "/api/v1/spaces/ENG/export", nil); r.status != 404 {
		t.Fatalf("outsider %d", r.status)
	}
	if r := w.client().do("GET", "/api/v1/spaces/ENG/export", nil); r.status != 401 {
		t.Fatalf("anon %d", r.status)
	}
	if r := w.as("sys").do("GET", "/api/v1/spaces/ENG/export", nil); r.status != 200 {
		t.Fatalf("sys %d", r.status)
	}
}

type exportFile struct{ name, body string }

func exportUpload(t *testing.T, c *client, path string, files []exportFile, fields map[string]string) response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="files"; filename="`+f.name+`"`)
		h.Set("Content-Type", "text/markdown")
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(f.body))
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req, err := http.NewRequest("POST", c.e.server.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := response{status: resp.StatusCode, header: resp.Header, raw: raw}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func exportPages(t *testing.T, r response) []map[string]any {
	t.Helper()
	var out struct {
		Pages []map[string]any `json:"pages"`
	}
	if err := json.Unmarshal(r.raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Pages
}

func exportByTitle(pages []map[string]any, title string) map[string]any {
	for _, p := range pages {
		if p["title"] == title {
			return p
		}
	}
	return nil
}

func TestImportNestedFolders(t *testing.T) {
	w := exportSetup(t, nil)
	files := []exportFile{
		{"overview.md", "# Project Overview\n\nHello **world**\n"},
		{"guides/setup.md", "Install it.\n\n- [x] step\n"},
		{"guides/advanced/tuning.md", "# Tuning\n\n| a | b |\n| - | - |\n| 1 | 2 |\n"},
		{"guides/advanced/index.md", "# Advanced Topics\n\nAdvanced intro\n"},
		{"guides/index.md", "Guides landing page\n"},
		{"other/readme-first.md", "<script>alert(1)</script>\n\nsafe text\n"},
	}
	r := exportUpload(t, w.as("ed1"), "/api/v1/spaces/eng/import", files, nil)
	if r.status != 201 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	pages := exportPages(t, r)
	titles := map[string]bool{}
	for _, p := range pages {
		titles[p["title"].(string)] = true
		if p["status"] != "published" {
			t.Fatalf("status %v", p["status"])
		}
	}
	for _, want := range []string{"Project Overview", "Guides", "Setup", "Advanced Topics", "Tuning", "Other", "Readme first"} {
		if !titles[want] {
			t.Fatalf("missing page %q in %v", want, titles)
		}
	}
	if len(pages) != 7 {
		t.Fatalf("expected 7 pages, got %d: %v", len(pages), titles)
	}
	guidesPage := exportByTitle(pages, "Guides")
	setup := exportByTitle(pages, "Setup")
	advanced := exportByTitle(pages, "Advanced Topics")
	tuning := exportByTitle(pages, "Tuning")
	overview := exportByTitle(pages, "Project Overview")
	if guidesPage["parent_id"] != w.home.String() || overview["parent_id"] != w.home.String() {
		t.Fatalf("top level parents: %v %v", guidesPage["parent_id"], overview["parent_id"])
	}
	if setup["parent_id"] != guidesPage["id"] || advanced["parent_id"] != guidesPage["id"] || tuning["parent_id"] != advanced["id"] {
		t.Fatalf("nesting wrong: setup=%v adv=%v tuning=%v", setup["parent_id"], advanced["parent_id"], tuning["parent_id"])
	}
	if guidesPage["has_children"] != true || tuning["has_children"] != false {
		t.Fatalf("has_children: %v %v", guidesPage["has_children"], tuning["has_children"])
	}

	ctx := context.Background()
	gid := uuid.MustParse(guidesPage["id"].(string))
	page, err := w.store.GetPage(ctx, gid)
	if err != nil {
		t.Fatal(err)
	}
	if page.Version != 1 || page.Status != domain.PageStatusPublished || !strings.Contains(page.ContentHTML, "Guides landing page") || page.ContentJSON != nil {
		t.Fatalf("guides page: v%d %s json=%s html=%q", page.Version, page.Status, page.ContentJSON, page.ContentHTML)
	}
	if page.CreatedBy != w.users["ed1"].ID {
		t.Fatal("author")
	}
	versions, err := w.store.ListPageVersions(ctx, gid)
	if err != nil || len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	var msg *string
	if err := w.store.Pool.QueryRow(ctx, `SELECT message FROM page_versions WHERE page_id = $1`, uuid.MustParse(setup["id"].(string))).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if msg == nil || *msg != "Imported from guides/setup.md" {
		t.Fatalf("message %v", msg)
	}
	tp, _ := w.store.GetPage(ctx, uuid.MustParse(tuning["id"].(string)))
	if !strings.Contains(tp.ContentHTML, "<table>") || strings.Contains(tp.ContentHTML, "Tuning") || tp.ContentText == "" {
		t.Fatalf("tuning: %q text=%q", tp.ContentHTML, tp.ContentText)
	}
	other, _ := w.store.GetPage(ctx, uuid.MustParse(exportByTitle(pages, "Readme first")["id"].(string)))
	if strings.Contains(other.ContentHTML, "script") || strings.Contains(other.ContentHTML, "alert") || !strings.Contains(other.ContentHTML, "safe text") {
		t.Fatalf("unsafe html: %q", other.ContentHTML)
	}
	empty, _ := w.store.GetPage(ctx, uuid.MustParse(exportByTitle(pages, "Other")["id"].(string)))
	if empty.ContentHTML != "" || empty.Version != 1 {
		t.Fatalf("folder page: %q v%d", empty.ContentHTML, empty.Version)
	}

	w.bus.Wait()
	w.mu.Lock()
	n := len(w.published)
	w.mu.Unlock()
	if n < 7 {
		t.Fatalf("expected 7 PagePublished events (home page publish adds one), got %d", n)
	}

	exp := w.as("viewer").do("GET", exportPath(uuid.MustParse(setup["id"].(string)), "md"), nil)
	if exp.status != 200 || string(exp.raw) != "# Setup\n\nInstall it.\n\n- [x] step\n" {
		t.Fatalf("re-export: %d %q", exp.status, exp.raw)
	}
}

func TestImportSingleFolderTitlesAndParent(t *testing.T) {
	w := exportSetup(t, nil)
	parent := w.page("ed1", "Parent", &w.home, "<p>p</p>", nil)
	r := exportUpload(t, w.as("ed1"), "/api/v1/spaces/ENG/import",
		[]exportFile{{"guides/setup.md", "text"}}, map[string]string{"parent_id": parent.String()})
	if r.status != 201 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	pages := exportPages(t, r)
	if len(pages) != 2 || pages[0]["title"] != "Guides" || pages[1]["title"] != "Setup" {
		t.Fatalf("pages %v", pages)
	}
	if pages[0]["parent_id"] != parent.String() || pages[1]["parent_id"] != pages[0]["id"] {
		t.Fatalf("parents %v", pages)
	}
}

func TestImportFilenameFallbackTitle(t *testing.T) {
	w := exportSetup(t, nil)
	r := exportUpload(t, w.as("ed1"), "/api/v1/spaces/ENG/import",
		[]exportFile{{"meeting-notes.md", "no heading here"}, {"index.md", "root index is a page"}}, nil)
	if r.status != 201 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	pages := exportPages(t, r)
	if len(pages) != 2 || pages[0]["title"] != "Meeting notes" || pages[1]["title"] != "Index" {
		t.Fatalf("pages %v", pages)
	}
}

func TestImportPermissionsAndValidation(t *testing.T) {
	w := exportSetup(t, func(c *config.Config) { c.AttachmentMaxBytes = 64 })
	ok := []exportFile{{"a.md", "# A"}}
	if r := exportUpload(t, w.as("viewer"), "/api/v1/spaces/ENG/import", ok, nil); r.status != 403 {
		t.Fatalf("viewer %d", r.status)
	}
	if r := exportUpload(t, w.as("outsider"), "/api/v1/spaces/ENG/import", ok, nil); r.status != 404 {
		t.Fatalf("outsider %d", r.status)
	}
	if r := exportUpload(t, w.client(), "/api/v1/spaces/ENG/import", ok, nil); r.status != 401 {
		t.Fatalf("anon %d", r.status)
	}
	ed := w.as("ed1")
	cases := []struct {
		name   string
		files  []exportFile
		fields map[string]string
		status int
	}{
		{"no files", nil, nil, 400},
		{"not markdown", []exportFile{{"pic.png", "x"}}, nil, 400},
		{"traversal", []exportFile{{"../evil.md", "x"}}, nil, 400},
		{"nested traversal", []exportFile{{"a/../../evil.md", "x"}}, nil, 400},
		{"absolute", []exportFile{{"/etc/evil.md", "x"}}, nil, 400},
		{"too large", []exportFile{{"big.md", strings.Repeat("x", 65)}}, nil, 413},
		{"bad parent", ok, map[string]string{"parent_id": "nope"}, 400},
		{"unknown parent", ok, map[string]string{"parent_id": uuid.NewString()}, 404},
	}
	for _, c := range cases {
		if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", c.files, c.fields); r.status != c.status {
			t.Fatalf("%s: %d %s", c.name, r.status, r.raw)
		}
	}
	var many []exportFile
	for i := 0; i < 201; i++ {
		many = append(many, exportFile{"f" + strings.Repeat("x", i%5) + ".md", "x"})
	}
	if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", many, nil); r.status != 400 {
		t.Fatalf("too many: %d %s", r.status, r.raw)
	}
	var count int
	if err := w.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pages WHERE space_id = $1`, w.space).Scan(&count); err != nil || count != 1 {
		t.Fatalf("pages after failed imports: %d %v", count, err)
	}
}

func TestImportParentFromOtherSpaceAndDraft(t *testing.T) {
	w := exportSetup(t, nil)
	other := testutil.CreateSpace(t, w.store, testutil.SpaceOpts{Key: "OPS", Visibility: domain.VisibilityInternal, CreatedBy: w.users["sys"].ID})
	foreign := testutil.CreatePage(t, w.store, other, w.users["sys"].ID, "Foreign")
	ed := w.as("ed1")
	if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", []exportFile{{"a.md", "x"}}, map[string]string{"parent_id": foreign.String()}); r.status != 404 {
		t.Fatalf("foreign parent %d", r.status)
	}
	hidden := w.draft("ed2", "Hidden", &w.home)
	if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", []exportFile{{"a.md", "x"}}, map[string]string{"parent_id": hidden.String()}); r.status != 404 {
		t.Fatalf("hidden draft parent %d", r.status)
	}
}

func TestImportRawFilenameNotBasenamed(t *testing.T) {
	w := exportSetup(t, nil)
	r := exportUpload(t, w.as("ed1"), "/api/v1/spaces/ENG/import",
		[]exportFile{{`guides\win\path.md`, "# Windows Path"}}, nil)
	if r.status != 201 {
		t.Fatalf("status %d %s", r.status, r.raw)
	}
	pages := exportPages(t, r)
	if len(pages) != 3 || pages[0]["title"] != "Guides" || pages[1]["title"] != "Win" || pages[2]["title"] != "Windows Path" {
		t.Fatalf("pages %v", pages)
	}
}

func TestImportHardLimits(t *testing.T) {
	w := exportSetup(t, nil)
	ed := w.as("ed1")
	const path = "/api/v1/spaces/ENG/import"
	deep := strings.Repeat("d/", 11) + "x.md"
	if r := exportUpload(t, ed, path, []exportFile{{deep, "x"}}, nil); r.status != 400 {
		t.Fatalf("11 dirs: %d %s", r.status, r.raw)
	}
	ok10 := strings.Repeat("d/", 10) + "x.md"
	if r := exportUpload(t, ed, path, []exportFile{{ok10, "x"}}, nil); r.status != 201 {
		t.Fatalf("10 dirs: %d %s", r.status, r.raw)
	}
	cases := []struct {
		name  string
		files []exportFile
		want  int
	}{
		{"very deep path", []exportFile{{strings.Repeat("d/", 20000) + "x.md", "x"}}, 400},
		{"long segment", []exportFile{{strings.Repeat("s", 121) + "/x.md", "x"}}, 400},
		{"long filename", []exportFile{{strings.Repeat("f", 253) + ".md", "x"}}, 400},
		{"oversized file", []exportFile{{"big.md", strings.Repeat("x", 2<<20+1)}}, 413},
		{"deep blockquote", []exportFile{{"q.md", strings.Repeat(">", 200) + " x"}}, 400},
		{"deep list", []exportFile{{"l.md", strings.Repeat("- ", 40) + "x"}}, 400},
		{"too many lines", []exportFile{{"lines.md", strings.Repeat("a\n", 20001)}}, 400},
	}
	for _, c := range cases {
		start := time.Now()
		r := exportUpload(t, ed, path, c.files, nil)
		if r.status != c.want {
			t.Fatalf("%s: %d %.200s", c.name, r.status, r.raw)
		}
		if time.Since(start) > 3*time.Second {
			t.Fatalf("%s took %v", c.name, time.Since(start))
		}
	}
	var many []exportFile
	for i := 0; i < 101; i++ {
		many = append(many, exportFile{"f" + strconv.Itoa(i) + ".md", "x"})
	}
	if r := exportUpload(t, ed, path, many, nil); r.status != 400 {
		t.Fatalf("101 files: %d %s", r.status, r.raw)
	}
	many = many[:100]
	if r := exportUpload(t, ed, path, many, nil); r.status != 201 {
		t.Fatalf("100 files: %d %s", r.status, r.raw)
	}
	var pageHeavy []exportFile
	for i := 0; i < 100; i++ {
		pageHeavy = append(pageHeavy, exportFile{"d" + strconv.Itoa(i) + "/a/b/c/d/x.md", "x"})
	}
	if r := exportUpload(t, ed, path, pageHeavy, nil); r.status != 400 {
		t.Fatalf("page cap: %d %s", r.status, r.raw)
	}
}

func TestImportTreeDepthCap(t *testing.T) {
	w := exportSetup(t, nil)
	parent := w.home
	for i := 0; i < 60; i++ {
		parent = w.page("ed1", "L"+strconv.Itoa(i), &parent, "<p>x</p>", nil)
	}
	ed := w.as("ed1")
	body := map[string]string{"parent_id": parent.String()}
	if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", []exportFile{{"a/b/c/d/e/f/x.md", "x"}}, body); r.status != 400 {
		t.Fatalf("too deep: %d %s", r.status, r.raw)
	}
	if r := exportUpload(t, ed, "/api/v1/spaces/ENG/import", []exportFile{{"a/b/x.md", "x"}}, body); r.status != 201 {
		t.Fatalf("within depth: %d %s", r.status, r.raw)
	}
	var count int
	if err := w.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pages WHERE title IN ('A','B','C','D','E','F')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rejected import must not leave pages: %d %v", count, err)
	}
}
