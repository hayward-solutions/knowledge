package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/config"
)

type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objects[r.URL.Path] = b
		f.types[r.URL.Path] = r.Header.Get("Content-Type")
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		b, ok := f.objects[r.URL.Path]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`))
			return
		}
		w.Header().Set("Content-Type", f.types[r.URL.Path])
		w.Header().Set("Content-Length", "11")
		_, _ = w.Write(b)
	case http.MethodDelete:
		delete(f.objects, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestFromEnvUnset(t *testing.T) {
	t.Setenv("STORAGE_S3_BUCKET", "")
	st, err := FromEnv(context.Background())
	if err != nil || st != nil {
		t.Fatalf("want nil,nil got %v,%v", st, err)
	}
}

func TestS3RoundTrip(t *testing.T) {
	fake := &fakeS3{objects: map[string][]byte{}, types: map[string]string{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	st, err := New(context.Background(), config.StorageSettings{
		Bucket:         "bkt",
		Region:         "us-east-1",
		Endpoint:       srv.URL,
		ForcePathStyle: true,
		AccessKeyID:    "ak",
		SecretKey:      "sk",
	})
	if err != nil || st == nil {
		t.Fatalf("new: %v %v", st, err)
	}
	ctx := context.Background()
	body := "hello world"
	if err := st.Put(ctx, "spaces/a/b.txt", "text/plain", io.NopCloser(strings.NewReader(body)), int64(len(body))); err != nil {
		t.Fatalf("put: %v", err)
	}
	if got := string(fake.objects["/bkt/spaces/a/b.txt"]); got != body {
		t.Fatalf("stored %q", got)
	}
	rc, ct, size, err := st.Get(ctx, "spaces/a/b.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != body || ct != "text/plain" || size != 11 {
		t.Fatalf("get returned %q %q %d", data, ct, size)
	}
	if _, _, _, err := st.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := st.Delete(ctx, "spaces/a/b.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := fake.objects["/bkt/spaces/a/b.txt"]; ok {
		t.Fatal("object still present")
	}
}

func TestSeekableLarge(t *testing.T) {
	data := strings.Repeat("x", memoryBufferLimit+10)
	rs, n, cleanup, err := seekable(io.NopCloser(strings.NewReader(data)), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if n != int64(len(data)) {
		t.Fatalf("n=%d", n)
	}
	got, _ := io.ReadAll(rs)
	if string(got) != data {
		t.Fatal("content mismatch")
	}
}

func TestSeekableShortBody(t *testing.T) {
	if _, _, _, err := seekable(io.NopCloser(strings.NewReader("abc")), 10); err == nil {
		t.Fatal("expected short body error")
	}
}
