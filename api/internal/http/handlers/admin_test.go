package handlers_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func adminClient(t *testing.T, e *env) *client {
	t.Helper()
	testutil.CreateUser(t, e.store, "root@example.test", "", true)
	c := e.client()
	c.mustLogin("root@example.test", testutil.DefaultPassword)
	return c
}

func TestAdminUsersRequireSystemAdmin(t *testing.T) {
	e := newEnv(t, nil)
	testutil.CreateUser(t, e.store, "plain@example.test", "", false)

	anon := e.client()
	for _, req := range [][2]string{{"GET", "/api/v1/admin/users"}, {"POST", "/api/v1/admin/users"}, {"PATCH", "/api/v1/admin/users/0194f0a0-0000-7000-8000-000000000001"}} {
		if r := anon.do(req[0], req[1], map[string]string{}); r.status != 401 {
			t.Errorf("anon %s %s: %d", req[0], req[1], r.status)
		}
	}

	plain := e.client()
	plain.mustLogin("plain@example.test", testutil.DefaultPassword)
	for _, req := range [][2]string{{"GET", "/api/v1/admin/users"}, {"POST", "/api/v1/admin/users"}, {"PATCH", "/api/v1/admin/users/0194f0a0-0000-7000-8000-000000000001"}} {
		r := plain.do(req[0], req[1], map[string]string{})
		if r.status != 403 || r.body["error"] != "forbidden" {
			t.Errorf("non-admin %s %s: %d %s", req[0], req[1], r.status, r.raw)
		}
	}
}

func TestAdminCreateUser(t *testing.T) {
	e := newEnv(t, nil)
	admin := adminClient(t, e)

	r := admin.do("POST", "/api/v1/admin/users", map[string]any{
		"email": "New.User@Example.test", "password": "Sup3r-Secret-Pass!", "display_name": "New User",
	})
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	if r.body["email"] != "new.user@example.test" || r.body["display_name"] != "New User" || r.body["is_system_admin"] != false {
		t.Fatalf("created user: %s", r.raw)
	}
	if strings.Contains(string(r.raw), "argon2") || strings.Contains(string(r.raw), "password") {
		t.Fatalf("response leaks password data: %s", r.raw)
	}

	fresh := e.client()
	if lr := fresh.login("new.user@example.test", "Sup3r-Secret-Pass!"); lr.status != 200 {
		t.Fatalf("new user cannot log in: %d %s", lr.status, lr.raw)
	}

	cases := []struct {
		name string
		body map[string]any
		code int
	}{
		{"duplicate email", map[string]any{"email": "new.user@example.test", "password": "Sup3r-Secret-Pass!"}, 409},
		{"weak password", map[string]any{"email": "weak@example.test", "password": "short"}, 400},
		{"bad email", map[string]any{"email": "nope", "password": "Sup3r-Secret-Pass!"}, 400},
		{"missing email", map[string]any{"password": "Sup3r-Secret-Pass!"}, 400},
	}
	for _, c := range cases {
		if r := admin.do("POST", "/api/v1/admin/users", c.body); r.status != c.code {
			t.Errorf("%s: %d %s", c.name, r.status, r.raw)
		}
	}

	r = admin.do("POST", "/api/v1/admin/users", map[string]any{
		"email": "second@example.test", "password": "Sup3r-Secret-Pass!", "is_system_admin": true,
	})
	if r.status != 201 || r.body["is_system_admin"] != true || r.body["display_name"] != "second@example.test" {
		t.Fatalf("create admin: %d %s", r.status, r.raw)
	}

	list := admin.do("GET", "/api/v1/admin/users", nil)
	if list.status != 200 || len(list.list) != 3 {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
}

func TestAdminPatchUser(t *testing.T) {
	e := newEnv(t, nil)
	admin := adminClient(t, e)
	target := testutil.CreateUser(t, e.store, "target@example.test", "", false)
	path := "/api/v1/admin/users/" + target.ID.String()

	r := admin.do("PATCH", path, map[string]any{"display_name": "Renamed", "email": "Renamed@Example.test"})
	if r.status != 200 || r.body["display_name"] != "Renamed" || r.body["email"] != "renamed@example.test" {
		t.Fatalf("rename: %d %s", r.status, r.raw)
	}

	r = admin.do("PATCH", path, map[string]any{"is_system_admin": true})
	if r.status != 200 || r.body["is_system_admin"] != true {
		t.Fatalf("grant admin: %d %s", r.status, r.raw)
	}
	r = admin.do("PATCH", path, map[string]any{"is_system_admin": false})
	if r.status != 200 || r.body["is_system_admin"] != false {
		t.Fatalf("revoke admin: %d %s", r.status, r.raw)
	}

	victim := e.client()
	victim.mustLogin("renamed@example.test", testutil.DefaultPassword)
	if r := victim.do("GET", "/api/v1/auth/me", nil); r.status != 200 {
		t.Fatalf("me: %d", r.status)
	}

	r = admin.do("PATCH", path, map[string]any{"disabled": true})
	if r.status != 200 || r.body["disabled_at"] == nil {
		t.Fatalf("disable: %d %s", r.status, r.raw)
	}
	if r := victim.do("GET", "/api/v1/auth/me", nil); r.status != 401 {
		t.Fatalf("disabled user session still valid: %d", r.status)
	}
	if r := e.client().login("renamed@example.test", testutil.DefaultPassword); r.status != 401 {
		t.Fatalf("disabled user can log in: %d", r.status)
	}
	r = admin.do("PATCH", path, map[string]any{"disabled": false})
	if r.status != 200 || r.body["disabled_at"] != nil {
		t.Fatalf("enable: %d %s", r.status, r.raw)
	}

	again := e.client()
	again.mustLogin("renamed@example.test", testutil.DefaultPassword)
	if r := admin.do("PATCH", path, map[string]any{"password": "weak"}); r.status != 400 {
		t.Fatalf("weak reset: %d", r.status)
	}
	if r := admin.do("PATCH", path, map[string]any{"password": "Reset-Passw0rd-Now!"}); r.status != 200 {
		t.Fatalf("reset: %d %s", r.status, r.raw)
	}
	if r := again.do("GET", "/api/v1/auth/me", nil); r.status != 401 {
		t.Fatalf("sessions should be revoked after reset: %d", r.status)
	}
	if r := e.client().login("renamed@example.test", testutil.DefaultPassword); r.status != 401 {
		t.Fatalf("old password works after reset: %d", r.status)
	}
	if r := e.client().login("renamed@example.test", "Reset-Passw0rd-Now!"); r.status != 200 {
		t.Fatalf("new password rejected: %d", r.status)
	}

	cases := []struct {
		name string
		path string
		body map[string]any
		code int
	}{
		{"unknown user", "/api/v1/admin/users/0194f0a0-0000-7000-8000-0000000000ff", map[string]any{"display_name": "x"}, 404},
		{"bad id", "/api/v1/admin/users/nope", map[string]any{"display_name": "x"}, 400},
		{"duplicate email", path, map[string]any{"email": "root@example.test"}, 409},
		{"blank display name", path, map[string]any{"display_name": " "}, 400},
		{"bad email", path, map[string]any{"email": "nope"}, 400},
	}
	for _, c := range cases {
		if r := admin.do("PATCH", c.path, c.body); r.status != c.code {
			t.Errorf("%s: %d %s", c.name, r.status, r.raw)
		}
	}
}

func TestAdminLastAdminProtection(t *testing.T) {
	e := newEnv(t, nil)
	admin := adminClient(t, e)
	var rootID string
	if err := e.store.Pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = 'root@example.test'`).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/users/" + rootID

	if r := admin.do("PATCH", path, map[string]any{"is_system_admin": false}); r.status != 409 {
		t.Fatalf("demote last admin: %d %s", r.status, r.raw)
	}
	if r := admin.do("PATCH", path, map[string]any{"disabled": true}); r.status != 409 {
		t.Fatalf("disable last admin: %d %s", r.status, r.raw)
	}

	second := testutil.CreateUser(t, e.store, "second@example.test", "", true)
	if r := admin.do("PATCH", path, map[string]any{"is_system_admin": false}); r.status != 200 {
		t.Fatalf("demote with spare admin: %d %s", r.status, r.raw)
	}
	secondClient := e.client()
	secondClient.mustLogin("second@example.test", testutil.DefaultPassword)
	if r := secondClient.do("PATCH", "/api/v1/admin/users/"+second.ID.String(), map[string]any{"disabled": true}); r.status != 409 {
		t.Fatalf("disable now-last admin: %d %s", r.status, r.raw)
	}
}
