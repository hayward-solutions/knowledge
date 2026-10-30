package handlers_test

import (
	"net/url"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestUserSearchAndGet(t *testing.T) {
	e := newEnv(t, nil)
	alice := testutil.CreateUser(t, e.store, "alice@example.test", "", false)
	testutil.CreateUser(t, e.store, "alina@example.test", "", false)
	testutil.CreateUser(t, e.store, "bob@example.test", "", false)

	if r := e.client().do("GET", "/api/v1/users?q=ali", nil); r.status != 401 {
		t.Fatalf("anon search: %d", r.status)
	}

	c := e.client()
	c.mustLogin("bob@example.test", testutil.DefaultPassword)

	r := c.do("GET", "/api/v1/users?q=ali", nil)
	if r.status != 200 || len(r.list) != 2 {
		t.Fatalf("search: %d %s", r.status, r.raw)
	}
	for _, u := range r.list {
		if len(u) != 3 || u["id"] == nil || u["display_name"] == nil || u["email"] == nil {
			t.Fatalf("user ref shape: %v", u)
		}
	}
	if r := c.do("GET", "/api/v1/users?q=ali&limit=1", nil); len(r.list) != 1 {
		t.Fatalf("limit: %s", r.raw)
	}
	if r := c.do("GET", "/api/v1/users?q="+url.QueryEscape("%"), nil); r.status != 200 || len(r.list) != 0 {
		t.Fatalf("wildcard must be escaped: %s", r.raw)
	}
	if r := c.do("GET", "/api/v1/users", nil); r.status != 200 || len(r.list) != 3 {
		t.Fatalf("list all: %s", r.raw)
	}
	if r := c.do("GET", "/api/v1/users?q=zzz", nil); r.status != 200 || string(r.raw) != "[]\n" {
		t.Fatalf("empty result must be []: %q", r.raw)
	}

	r = c.do("GET", "/api/v1/users/"+alice.ID.String(), nil)
	if r.status != 200 || r.body["email"] != "alice@example.test" || r.body["id"] != alice.ID.String() {
		t.Fatalf("get: %d %s", r.status, r.raw)
	}
	if r := c.do("GET", "/api/v1/users/0194f0a0-0000-7000-8000-0000000000ff", nil); r.status != 404 {
		t.Fatalf("unknown: %d", r.status)
	}
	if r := c.do("GET", "/api/v1/users/nope", nil); r.status != 400 {
		t.Fatalf("bad id: %d", r.status)
	}
}
