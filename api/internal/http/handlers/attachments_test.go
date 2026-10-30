package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/storage"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type fakeObject struct {
	data        []byte
	contentType string
}

type fakeObjectStore struct {
	mu         sync.Mutex
	objects    map[string]fakeObject
	failDelete bool
	onPut      func()
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: map[string]fakeObject{}}
}

func (f *fakeObjectStore) Put(_ context.Context, key, contentType string, r io.Reader, size int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return io.ErrUnexpectedEOF
	}
	f.mu.Lock()
	f.objects[key] = fakeObject{data: data, contentType: contentType}
	hook := f.onPut
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (f *fakeObjectStore) Get(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key]
	if !ok {
		return nil, "", 0, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(o.data)), o.contentType, int64(len(o.data)), nil
}

func (f *fakeObjectStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDelete {
		return errors.New("delete failed")
	}
	delete(f.objects, key)
	return nil
}

func (f *fakeObjectStore) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.objects))
	for k := range f.objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func newAttachmentEnv(t *testing.T, mutate func(*config.Config), st storage.ObjectStore) *env {
	t.Helper()
	s := testutil.NewDB(t)
	cfg := &config.Config{AppOrigins: []string{"http://app.test"}}
	if mutate != nil {
		mutate(cfg)
	}
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store:    s,
		Bus:      events.NewBus(),
		Hub:      events.NewHub(),
		Presence: presence,
		Storage:  st,
		Config:   cfg,
		Version:  "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &env{t: t, store: s, server: srv}
}

type attachmentFixture struct {
	env      *env
	fake     *fakeObjectStore
	admin    domain.User
	editor   domain.User
	editor2  domain.User
	viewer   domain.User
	outsider domain.User
	spaceID  uuid.UUID
	pageID   uuid.UUID
	private  uuid.UUID
	privPage uuid.UUID
}

func newAttachmentFixture(t *testing.T, mutate func(*config.Config)) *attachmentFixture {
	t.Helper()
	fake := newFakeObjectStore()
	e := newAttachmentEnv(t, mutate, fake)
	s := e.store
	f := &attachmentFixture{env: e, fake: fake}
	f.admin = testutil.CreateUser(t, s, "admin@att.test", "", true)
	f.editor = testutil.CreateUser(t, s, "editor@att.test", "", false)
	f.editor2 = testutil.CreateUser(t, s, "editor2@att.test", "", false)
	f.viewer = testutil.CreateUser(t, s, "viewer@att.test", "", false)
	f.outsider = testutil.CreateUser(t, s, "outsider@att.test", "", false)

	f.spaceID = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "DOCS", Visibility: domain.VisibilityInternal, CreatedBy: f.admin.ID})
	testutil.AddMember(t, s, f.spaceID, f.editor.ID, domain.RoleEditor)
	testutil.AddMember(t, s, f.spaceID, f.editor2.ID, domain.RoleEditor)
	testutil.AddMember(t, s, f.spaceID, f.viewer.ID, domain.RoleViewer)
	f.pageID = testutil.CreatePage(t, s, f.spaceID, f.editor.ID, "Page")

	f.private = testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "SECRET", Visibility: domain.VisibilityPrivate, CreatedBy: f.admin.ID})
	testutil.AddMember(t, s, f.private, f.editor.ID, domain.RoleEditor)
	f.privPage = testutil.CreatePage(t, s, f.private, f.editor.ID, "Secret page")
	return f
}

func (f *attachmentFixture) login(u domain.User) *client {
	f.env.t.Helper()
	c := f.env.client()
	c.mustLogin(u.Email, testutil.DefaultPassword)
	return c
}

func (f *attachmentFixture) makeDraft(pageID uuid.UUID) {
	f.env.t.Helper()
	if _, err := f.env.store.Pool.Exec(context.Background(), `UPDATE pages SET status = 'draft' WHERE id = $1`, pageID); err != nil {
		f.env.t.Fatal(err)
	}
}

type attachmentPart struct {
	field       string
	filename    string
	contentType string
	data        []byte
}

func attachmentMultipart(t *testing.T, parts ...attachmentPart) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+p.field+`"; filename="`+esc.Replace(p.filename)+`"`)
		if p.contentType != "" {
			h.Set("Content-Type", p.contentType)
		}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func (c *client) rawRequest(method, path string, body io.Reader, contentType string) response {
	c.e.t.Helper()
	req, err := http.NewRequest(method, c.e.server.URL+path, body)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := response{status: resp.StatusCode, header: resp.Header, raw: raw}
	_ = json.Unmarshal(raw, &out.body)
	_ = json.Unmarshal(raw, &out.list)
	return out
}

func (c *client) upload(pageID uuid.UUID, filename, contentType string, data []byte) response {
	c.e.t.Helper()
	body, ct := attachmentMultipart(c.e.t, attachmentPart{field: "file", filename: filename, contentType: contentType, data: data})
	return c.rawRequest("POST", "/api/v1/pages/"+pageID.String()+"/attachments", body, ct)
}

func (f *attachmentFixture) mustUpload(c *client, pageID uuid.UUID, filename, contentType string, data []byte) map[string]any {
	f.env.t.Helper()
	r := c.upload(pageID, filename, contentType, data)
	if r.status != http.StatusCreated {
		f.env.t.Fatalf("upload %s: %d %s", filename, r.status, r.raw)
	}
	return r.body
}

var pngBytes = append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0x01, 0x02, 0x03}, 40)...)

func TestAttachmentUploadStoresObjectAndRow(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	data := []byte("hello attachment\n")

	got := f.mustUpload(c, f.pageID, "notes.txt", "text/plain", data)
	id, _ := got["id"].(string)
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("bad id %q", id)
	}
	if got["page_id"] != f.pageID.String() || got["space_id"] != f.spaceID.String() || got["uploader_id"] != f.editor.ID.String() {
		t.Fatalf("unexpected ids: %v", got)
	}
	if got["filename"] != "notes.txt" || got["size"].(float64) != float64(len(data)) {
		t.Fatalf("unexpected metadata: %v", got)
	}
	if !strings.HasPrefix(got["content_type"].(string), "text/plain") {
		t.Fatalf("content type: %v", got["content_type"])
	}
	if _, ok := got["created_at"].(string); !ok {
		t.Fatalf("created_at missing: %v", got)
	}
	if _, leaked := got["s3_key"]; leaked {
		t.Fatalf("s3_key must not be exposed: %v", got)
	}
	up, ok := got["uploader"].(map[string]any)
	if !ok || up["email"] != f.editor.Email || up["id"] != f.editor.ID.String() {
		t.Fatalf("uploader: %v", got["uploader"])
	}

	wantKey := "spaces/" + f.spaceID.String() + "/pages/" + f.pageID.String() + "/" + id + "/notes.txt"
	keys := f.fake.keys()
	if len(keys) != 1 || keys[0] != wantKey {
		t.Fatalf("stored keys %v, want %s", keys, wantKey)
	}
	if !bytes.Equal(f.fake.objects[wantKey].data, data) {
		t.Fatal("stored bytes differ")
	}
	if !strings.HasPrefix(f.fake.objects[wantKey].contentType, "text/plain") {
		t.Fatalf("stored content type %q", f.fake.objects[wantKey].contentType)
	}

	list := c.do("GET", "/api/v1/pages/"+f.pageID.String()+"/attachments", nil)
	if list.status != 200 || len(list.list) != 1 || list.list[0]["id"] != id {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
	if list.list[0]["uploader"].(map[string]any)["email"] != f.editor.Email {
		t.Fatalf("list uploader: %v", list.list[0]["uploader"])
	}
}

func TestAttachmentUploadWithoutStorage(t *testing.T) {
	e := newAttachmentEnv(t, nil, nil)
	admin := testutil.CreateUser(t, e.store, "admin@nostore.test", "", true)
	spaceID := testutil.CreateSpace(t, e.store, testutil.SpaceOpts{Key: "NOS", CreatedBy: admin.ID})
	pageID := testutil.CreatePage(t, e.store, spaceID, admin.ID, "Page")
	c := e.client()
	c.mustLogin(admin.Email, testutil.DefaultPassword)

	r := c.upload(pageID, "a.txt", "text/plain", []byte("x"))
	if r.status != http.StatusServiceUnavailable || r.body["error"] != "object storage not configured" {
		t.Fatalf("got %d %s", r.status, r.raw)
	}
}

func TestAttachmentUploadSizeLimit(t *testing.T) {
	f := newAttachmentFixture(t, func(c *config.Config) { c.AttachmentMaxBytes = 1024 })
	c := f.login(f.editor)

	r := c.upload(f.pageID, "big.bin", "application/octet-stream", bytes.Repeat([]byte{'a'}, 1025))
	if r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("1025 bytes: %d %s", r.status, r.raw)
	}
	r = c.upload(f.pageID, "huge.bin", "application/octet-stream", bytes.Repeat([]byte{'a'}, 64*1024))
	if r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("64 KiB: %d %s", r.status, r.raw)
	}
	if len(f.fake.keys()) != 0 {
		t.Fatalf("oversized upload must not be stored: %v", f.fake.keys())
	}
	r = c.upload(f.pageID, "exact.bin", "application/octet-stream", bytes.Repeat([]byte{'a'}, 1024))
	if r.status != http.StatusCreated {
		t.Fatalf("exactly at the cap: %d %s", r.status, r.raw)
	}
}

func TestAttachmentUploadValidation(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)

	if r := c.upload(f.pageID, "empty.txt", "text/plain", nil); r.status != http.StatusBadRequest {
		t.Fatalf("empty file: %d %s", r.status, r.raw)
	}

	body, ct := attachmentMultipart(t, attachmentPart{field: "other", filename: "a.txt", data: []byte("x")})
	if r := c.rawRequest("POST", "/api/v1/pages/"+f.pageID.String()+"/attachments", body, ct); r.status != http.StatusBadRequest {
		t.Fatalf("wrong field: %d %s", r.status, r.raw)
	}

	r := c.rawRequest("POST", "/api/v1/pages/"+f.pageID.String()+"/attachments", strings.NewReader(`{"file":"x"}`), "application/json")
	if r.status != http.StatusBadRequest {
		t.Fatalf("non-multipart: %d %s", r.status, r.raw)
	}

	r = c.rawRequest("POST", "/api/v1/pages/not-a-uuid/attachments", strings.NewReader(""), "")
	if r.status != http.StatusBadRequest {
		t.Fatalf("bad page id: %d %s", r.status, r.raw)
	}
	r = c.upload(uuid.Must(uuid.NewV7()), "a.txt", "text/plain", []byte("x"))
	if r.status != http.StatusNotFound {
		t.Fatalf("missing page: %d %s", r.status, r.raw)
	}
	if len(f.fake.keys()) != 0 {
		t.Fatalf("nothing should be stored: %v", f.fake.keys())
	}
}

func TestAttachmentTrashedPage(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	editor := f.login(f.editor)
	att := f.mustUpload(editor, f.pageID, "a.txt", "text/plain", []byte("x"))
	id := att["id"].(string)
	if _, err := f.env.store.Pool.Exec(context.Background(), `UPDATE pages SET status = 'trashed' WHERE id = $1`, f.pageID); err != nil {
		t.Fatal(err)
	}
	listPath := "/api/v1/pages/" + f.pageID.String() + "/attachments"

	r := editor.upload(f.pageID, "b.txt", "text/plain", []byte("x"))
	if r.status != http.StatusConflict || r.body["error"] != "page is trashed" || r.body["code"] != "page_trashed" {
		t.Fatalf("upload to trashed page: %d %s", r.status, r.raw)
	}
	r = editor.do("DELETE", "/api/v1/attachments/"+id, nil)
	if r.status != http.StatusConflict || r.body["code"] != "page_trashed" {
		t.Fatalf("delete on trashed page: %d %s", r.status, r.raw)
	}
	if len(f.fake.keys()) != 1 {
		t.Fatalf("object must survive: %v", f.fake.keys())
	}

	for _, u := range []domain.User{f.editor, f.admin} {
		c := f.login(u)
		if r := c.do("GET", listPath, nil); r.status != 200 || len(r.list) != 1 {
			t.Fatalf("%s list trashed: %d %s", u.Email, r.status, r.raw)
		}
		for _, p := range []string{"", "/content"} {
			if r := c.do("GET", "/api/v1/attachments/"+id+p, nil); r.status != 200 {
				t.Fatalf("%s GET trashed %q: %d", u.Email, p, r.status)
			}
		}
	}

	viewer := f.login(f.viewer)
	if r := viewer.do("GET", listPath, nil); r.status != http.StatusNotFound {
		t.Fatalf("viewer list trashed: %d %s", r.status, r.raw)
	}
	for _, p := range []string{"", "/content"} {
		if r := viewer.do("GET", "/api/v1/attachments/"+id+p, nil); r.status != http.StatusNotFound {
			t.Fatalf("viewer GET trashed %q: %d", p, r.status)
		}
	}
}

func attachmentDenied(status int) bool {
	return status == http.StatusForbidden || status == http.StatusNotFound
}

func TestAttachmentPermissions(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	editor := f.login(f.editor)
	att := f.mustUpload(editor, f.pageID, "a.txt", "text/plain", []byte("data"))
	id := att["id"].(string)
	privAtt := f.mustUpload(editor, f.privPage, "p.txt", "text/plain", []byte("secret"))
	privID := privAtt["id"].(string)

	viewer := f.login(f.viewer)
	if r := viewer.upload(f.pageID, "v.txt", "text/plain", []byte("x")); r.status != http.StatusForbidden {
		t.Fatalf("viewer upload: %d %s", r.status, r.raw)
	}
	if r := viewer.do("DELETE", "/api/v1/attachments/"+id, nil); r.status != http.StatusForbidden {
		t.Fatalf("viewer delete: %d %s", r.status, r.raw)
	}
	if r := viewer.do("GET", "/api/v1/pages/"+f.pageID.String()+"/attachments", nil); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("viewer list: %d %s", r.status, r.raw)
	}
	if r := viewer.do("GET", "/api/v1/attachments/"+id, nil); r.status != 200 {
		t.Fatalf("viewer download: %d", r.status)
	}

	outsider := f.login(f.outsider)
	if r := outsider.upload(f.privPage, "o.txt", "text/plain", []byte("x")); r.status != http.StatusNotFound {
		t.Fatalf("outsider upload: %d %s", r.status, r.raw)
	}
	for _, path := range []string{
		"/api/v1/pages/" + f.privPage.String() + "/attachments",
		"/api/v1/attachments/" + privID,
		"/api/v1/attachments/" + privID + "/content",
	} {
		if r := outsider.do("GET", path, nil); r.status != http.StatusNotFound {
			t.Fatalf("outsider GET %s: %d %s", path, r.status, r.raw)
		}
	}
	if r := outsider.do("DELETE", "/api/v1/attachments/"+privID, nil); r.status != http.StatusNotFound {
		t.Fatalf("outsider delete: %d %s", r.status, r.raw)
	}

	anon := f.env.client()
	if r := anon.do("GET", "/api/v1/attachments/"+id, nil); r.status != http.StatusUnauthorized {
		t.Fatalf("anonymous download: %d", r.status)
	}
	if r := anon.upload(f.pageID, "a.txt", "text/plain", []byte("x")); r.status != http.StatusUnauthorized {
		t.Fatalf("anonymous upload: %d", r.status)
	}

	if r := f.login(f.admin).do("GET", "/api/v1/attachments/"+privID, nil); r.status != 200 {
		t.Fatalf("system admin on private space: %d", r.status)
	}
}

func TestAttachmentDownloadHeadersAndBytes(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	data := []byte("quarterly report body\nline two\n")
	att := f.mustUpload(c, f.pageID, "résumé ü.txt", "text/plain", data)
	id := att["id"].(string)

	r := c.do("GET", "/api/v1/attachments/"+id, nil)
	if r.status != 200 {
		t.Fatalf("download: %d %s", r.status, r.raw)
	}
	if !bytes.Equal(r.raw, data) {
		t.Fatalf("bytes differ: %q", r.raw)
	}
	if !strings.HasPrefix(r.header.Get("Content-Type"), "text/plain") {
		t.Fatalf("content-type %q", r.header.Get("Content-Type"))
	}
	if r.header.Get("Content-Length") != "31" {
		t.Fatalf("content-length %q", r.header.Get("Content-Length"))
	}
	cd := r.header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, `filename="r_sum_ _.txt"`) ||
		!strings.Contains(cd, "filename*=UTF-8''r%C3%A9sum%C3%A9%20%C3%BC.txt") {
		t.Fatalf("content-disposition %q", cd)
	}
	if r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff missing: %v", r.header)
	}
	if r.header.Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("cache-control %q", r.header.Get("Cache-Control"))
	}

	if r := c.do("GET", "/api/v1/attachments/"+uuid.Must(uuid.NewV7()).String(), nil); r.status != http.StatusNotFound {
		t.Fatalf("unknown id: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/attachments/nope", nil); r.status != http.StatusBadRequest {
		t.Fatalf("bad id: %d", r.status)
	}
}

func TestAttachmentDownloadMissingObject(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	att := f.mustUpload(c, f.pageID, "gone.txt", "text/plain", []byte("x"))
	for _, k := range f.fake.keys() {
		_ = f.fake.Delete(context.Background(), k)
	}
	for _, p := range []string{"", "/content"} {
		if r := c.do("GET", "/api/v1/attachments/"+att["id"].(string)+p, nil); r.status != http.StatusNotFound {
			t.Fatalf("missing object %q: %d %s", p, r.status, r.raw)
		}
	}
}

func TestAttachmentContentDisposition(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)

	cases := []struct {
		name        string
		contentType string
		data        []byte
		wantType    string
		wantInline  bool
	}{
		{"pic.png", "image/png", pngBytes, "image/png", true},
		{"photo.jpg", "application/octet-stream", append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{1}, 20)...), "image/jpeg", true},
		{"anim.gif", "image/gif", append([]byte("GIF89a"), bytes.Repeat([]byte{1}, 20)...), "image/gif", true},
		{"doc.pdf", "application/pdf", append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{'a'}, 30)...), "application/pdf", false},
		{"page.html", "image/png", []byte("<html><body><script>alert(1)</script></body></html>"), "text/html", false},
		{"plain.txt", "text/plain", []byte("just text"), "text/plain", false},
		{"blob.bin", "application/octet-stream", []byte{0, 1, 2, 3, 4, 5}, "application/octet-stream", false},
	}
	for _, tc := range cases {
		att := f.mustUpload(c, f.pageID, tc.name, tc.contentType, tc.data)
		if got := att["content_type"].(string); got != tc.wantType {
			t.Errorf("%s: stored content type %q, want %q", tc.name, got, tc.wantType)
		}
		id := att["id"].(string)
		r := c.do("GET", "/api/v1/attachments/"+id+"/content", nil)
		if r.status != 200 || !bytes.Equal(r.raw, tc.data) {
			t.Fatalf("%s: content %d", tc.name, r.status)
		}
		cd := r.header.Get("Content-Disposition")
		if tc.wantInline && !strings.HasPrefix(cd, "inline;") {
			t.Errorf("%s: expected inline, got %q", tc.name, cd)
		}
		if !tc.wantInline && !strings.HasPrefix(cd, "attachment;") {
			t.Errorf("%s: expected attachment, got %q", tc.name, cd)
		}
		if r.header.Get("X-Content-Type-Options") != "nosniff" || r.header.Get("Cache-Control") != "private, max-age=3600" {
			t.Errorf("%s: missing security/cache headers: %v", tc.name, r.header)
		}
		dl := c.do("GET", "/api/v1/attachments/"+id, nil)
		if !strings.HasPrefix(dl.header.Get("Content-Disposition"), "attachment;") {
			t.Errorf("%s: plain download must be attachment, got %q", tc.name, dl.header.Get("Content-Disposition"))
		}
	}
}

func TestAttachmentSVGIsNeverInline(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	id := uuid.Must(uuid.NewV7())
	key := "spaces/x/pages/y/" + id.String() + "/logo.svg"
	f.fake.objects[key] = fakeObject{data: svg, contentType: "image/svg+xml"}
	_, err := f.env.store.Pool.Exec(context.Background(),
		`INSERT INTO attachments (id, page_id, space_id, uploader_id, filename, content_type, size, s3_key)
		 VALUES ($1, $2, $3, $4, 'logo.svg', 'image/svg+xml', $5, $6)`,
		id, f.pageID, f.spaceID, f.editor.ID, len(svg), key)
	if err != nil {
		t.Fatal(err)
	}
	r := c.do("GET", "/api/v1/attachments/"+id.String()+"/content", nil)
	if r.status != 200 || !bytes.Equal(r.raw, svg) {
		t.Fatalf("svg content: %d", r.status)
	}
	if !strings.HasPrefix(r.header.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("svg must be forced to attachment, got %q", r.header.Get("Content-Disposition"))
	}
	if r.header.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("svg content type %q", r.header.Get("Content-Type"))
	}
	if !strings.Contains(r.header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("expected sandbox CSP, got %q", r.header.Get("Content-Security-Policy"))
	}
}

func TestAttachmentDeclaredTypeFallback(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	bin := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	att := f.mustUpload(c, f.pageID, "model.glb", "model/gltf-binary", bin)
	if att["content_type"] != "model/gltf-binary" {
		t.Fatalf("declared type should be used for opaque bytes, got %v", att["content_type"])
	}
	att = f.mustUpload(c, f.pageID, "model2.bin", "", bin)
	if att["content_type"] != "application/octet-stream" {
		t.Fatalf("expected octet-stream, got %v", att["content_type"])
	}
	att = f.mustUpload(c, f.pageID, "liar.png", "image/png", []byte("<html>hi</html>"))
	if att["content_type"] != "text/html" {
		t.Fatalf("sniffed type must win over declared, got %v", att["content_type"])
	}
}

func TestAttachmentFilenameSanitising(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)

	cases := []struct {
		in, want string
	}{
		{"../../etc/passwd", "passwd"},
		{`C:\Users\matt\report.docx`, "report.docx"},
		{"a\tb\u202ec\u200bd.txt", "abcd.txt"},
		{"..", "file"},
		{"   ", "file"},
		{"dir/", "dir"},
	}
	for _, tc := range cases {
		att := f.mustUpload(c, f.pageID, tc.in, "text/plain", []byte("x"))
		if att["filename"] != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, att["filename"], tc.want)
		}
	}

	long := strings.Repeat("é", 200) + ".txt"
	att := f.mustUpload(c, f.pageID, long, "text/plain", []byte("x"))
	name := att["filename"].(string)
	if len(name) > 255 || !strings.HasSuffix(name, ".txt") || !strings.Contains(name, "é") {
		t.Fatalf("long name not shortened properly: %d bytes %q", len(name), name)
	}
	for _, k := range f.fake.keys() {
		if strings.Contains(k, "..") || strings.Contains(k, `\`) {
			t.Errorf("unsafe key %q", k)
		}
	}
}

func TestAttachmentDelete(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	keep := f.mustUpload(c, f.pageID, "keep.txt", "text/plain", []byte("keep"))
	gone := f.mustUpload(c, f.pageID, "gone.txt", "text/plain", []byte("gone"))
	goneID := gone["id"].(string)
	if len(f.fake.keys()) != 2 {
		t.Fatalf("setup: %v", f.fake.keys())
	}

	if r := c.do("DELETE", "/api/v1/attachments/"+goneID, nil); r.status != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}
	keys := f.fake.keys()
	if len(keys) != 1 || !strings.Contains(keys[0], keep["id"].(string)) {
		t.Fatalf("object should be removed: %v", keys)
	}
	list := c.do("GET", "/api/v1/pages/"+f.pageID.String()+"/attachments", nil)
	if list.status != 200 || len(list.list) != 1 || list.list[0]["id"] != keep["id"] {
		t.Fatalf("list after delete: %d %s", list.status, list.raw)
	}
	for _, p := range []string{"", "/content"} {
		if r := c.do("GET", "/api/v1/attachments/"+goneID+p, nil); r.status != http.StatusNotFound {
			t.Fatalf("GET deleted %q: %d", p, r.status)
		}
	}
	if r := c.do("DELETE", "/api/v1/attachments/"+goneID, nil); r.status != http.StatusNotFound {
		t.Fatalf("double delete: %d", r.status)
	}
}

func TestAttachmentDeleteSurvivesStorageFailure(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	att := f.mustUpload(c, f.pageID, "a.txt", "text/plain", []byte("x"))
	f.fake.failDelete = true
	if r := c.do("DELETE", "/api/v1/attachments/"+att["id"].(string), nil); r.status != http.StatusNoContent {
		t.Fatalf("delete with failing storage: %d %s", r.status, r.raw)
	}
	list := c.do("GET", "/api/v1/pages/"+f.pageID.String()+"/attachments", nil)
	if len(list.list) != 0 {
		t.Fatalf("row should still be hidden: %s", list.raw)
	}
}

func TestAttachmentInsertFailureRemovesObject(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	f.fake.onPut = func() {
		_, _ = f.env.store.Pool.Exec(context.Background(), `DELETE FROM pages WHERE id = $1`, f.pageID)
	}
	c := f.login(f.editor)
	r := c.upload(f.pageID, "a.txt", "text/plain", []byte("x"))
	if r.status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d %s", r.status, r.raw)
	}
	if len(f.fake.keys()) != 0 {
		t.Fatalf("object must be cleaned up after a failed insert: %v", f.fake.keys())
	}
}

func TestAttachmentDraftPageVisibility(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	creator := f.login(f.editor)
	att := f.mustUpload(creator, f.pageID, "d.txt", "text/plain", []byte("draft data"))
	id := att["id"].(string)
	f.makeDraft(f.pageID)
	listPath := "/api/v1/pages/" + f.pageID.String() + "/attachments"

	if r := creator.do("GET", listPath, nil); r.status != 200 || len(r.list) != 1 {
		t.Fatalf("creator list: %d %s", r.status, r.raw)
	}
	f.mustUpload(creator, f.pageID, "d2.txt", "text/plain", []byte("more"))

	if r := f.login(f.admin).do("GET", listPath, nil); r.status != 200 || len(r.list) != 2 {
		t.Fatalf("system admin list: %d %s", r.status, r.raw)
	}
	f.mustUpload(f.login(f.admin), f.pageID, "d3.txt", "text/plain", []byte("admin"))

	other := f.login(f.editor2)
	if r := other.do("GET", listPath, nil); r.status != http.StatusNotFound {
		t.Fatalf("other editor list: %d %s", r.status, r.raw)
	}
	if r := other.upload(f.pageID, "x.txt", "text/plain", []byte("x")); r.status != http.StatusNotFound {
		t.Fatalf("other editor upload: %d %s", r.status, r.raw)
	}
	for _, p := range []string{"", "/content"} {
		if r := other.do("GET", "/api/v1/attachments/"+id+p, nil); r.status != http.StatusNotFound {
			t.Fatalf("other editor GET %q: %d", p, r.status)
		}
	}
	if r := other.do("DELETE", "/api/v1/attachments/"+id, nil); r.status != http.StatusNotFound {
		t.Fatalf("other editor delete: %d", r.status)
	}

	viewer := f.login(f.viewer)
	if r := viewer.do("GET", listPath, nil); r.status != http.StatusNotFound {
		t.Fatalf("viewer list of draft: %d", r.status)
	}
}

func TestAttachmentListNewestFirstAndEmpty(t *testing.T) {
	f := newAttachmentFixture(t, nil)
	c := f.login(f.editor)
	listPath := "/api/v1/pages/" + f.pageID.String() + "/attachments"

	r := c.do("GET", listPath, nil)
	if r.status != 200 || strings.TrimSpace(string(r.raw)) != "[]" {
		t.Fatalf("empty list: %d %q", r.status, r.raw)
	}
	a := f.mustUpload(c, f.pageID, "a.txt", "text/plain", []byte("a"))
	b := f.mustUpload(c, f.pageID, "b.txt", "text/plain", []byte("b"))
	r = c.do("GET", listPath, nil)
	if len(r.list) != 2 || r.list[0]["id"] != b["id"] || r.list[1]["id"] != a["id"] {
		t.Fatalf("expected newest first: %s", r.raw)
	}
}
