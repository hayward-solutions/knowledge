package handlers_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestLoginLogoutMeFlow(t *testing.T) {
	e := newEnv(t, nil)
	testutil.CreateUser(t, e.store, "ada@example.test", "", false)
	c := e.client()

	if r := c.do("GET", "/api/v1/auth/me", nil); r.status != 401 || r.body["error"] != "unauthenticated" {
		t.Fatalf("me before login: %d %s", r.status, r.raw)
	}
	if r := c.login("ada@example.test", "Wrong-Password-1!"); r.status != 401 || r.body["error"] != "invalid credentials" {
		t.Fatalf("bad password: %d %s", r.status, r.raw)
	}
	if r := c.login("nobody@example.test", testutil.DefaultPassword); r.status != 401 || r.body["error"] != "invalid credentials" {
		t.Fatalf("unknown user: %d %s", r.status, r.raw)
	}
	if r := c.do("POST", "/api/v1/auth/login", "not an object"); r.status != 400 {
		t.Fatalf("bad body: %d", r.status)
	}

	r := c.login("ADA@Example.test", testutil.DefaultPassword)
	if r.status != 200 || r.body["email"] != "ada@example.test" {
		t.Fatalf("login: %d %s", r.status, r.raw)
	}
	if strings.Contains(string(r.raw), "password") || strings.Contains(string(r.raw), "argon2") {
		t.Fatalf("login response leaks password data: %s", r.raw)
	}
	for _, k := range []string{"id", "email", "display_name", "is_system_admin", "disabled_at", "created_at", "updated_at"} {
		if _, ok := r.body[k]; !ok {
			t.Errorf("user json missing %q: %s", k, r.raw)
		}
	}

	cookies := r.header.Values("Set-Cookie")
	if len(cookies) != 1 || !strings.HasPrefix(cookies[0], "sid=") {
		t.Fatalf("cookies: %v", cookies)
	}
	if !strings.Contains(cookies[0], "HttpOnly") || !strings.Contains(cookies[0], "SameSite=Lax") || !strings.Contains(cookies[0], "Path=/") {
		t.Errorf("cookie attributes: %s", cookies[0])
	}
	if strings.Contains(cookies[0], "Secure") {
		t.Errorf("cookie should not be Secure by default: %s", cookies[0])
	}
	token := strings.TrimPrefix(strings.SplitN(cookies[0], ";", 2)[0], "sid=")

	var storedID string
	var lifetime float64
	err := e.store.Pool.QueryRow(context.Background(),
		`SELECT id, extract(epoch FROM expires_at - created_at) FROM sessions`).Scan(&storedID, &lifetime)
	if err != nil {
		t.Fatal(err)
	}
	if storedID == token || storedID != store.HashSessionToken(token) {
		t.Fatalf("session id must be sha256(token); stored %q token %q", storedID, token)
	}
	if lifetime < 29.9*86400 || lifetime > 30.1*86400 {
		t.Errorf("session lifetime %.0fs, want about 30 days", lifetime)
	}

	if r := c.do("GET", "/api/v1/auth/me", nil); r.status != 200 || r.body["email"] != "ada@example.test" {
		t.Fatalf("me: %d %s", r.status, r.raw)
	}

	r = c.do("POST", "/api/v1/auth/logout", nil)
	if r.status != 204 {
		t.Fatalf("logout: %d", r.status)
	}
	if cleared := r.header.Get("Set-Cookie"); !strings.Contains(cleared, "sid=;") || !strings.Contains(cleared, "Max-Age=0") {
		t.Errorf("logout cookie not cleared: %s", cleared)
	}
	if r := c.do("GET", "/api/v1/auth/me", nil); r.status != 401 {
		t.Fatalf("me after logout: %d", r.status)
	}
	var n int
	_ = e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&n)
	if n != 0 {
		t.Errorf("session row not deleted: %d", n)
	}
	if r := c.do("POST", "/api/v1/auth/logout", nil); r.status != 204 {
		t.Fatalf("logout without session: %d", r.status)
	}
}

func TestLoginRejectsDisabledAndPasswordlessUsers(t *testing.T) {
	e := newEnv(t, nil)
	u := testutil.CreateUser(t, e.store, "off@example.test", "", false)
	disabled := true
	if _, err := e.store.AdminUpdateUser(context.Background(), u.ID, store.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	if r := c.login("off@example.test", testutil.DefaultPassword); r.status != 401 || r.body["error"] != "account disabled" {
		t.Fatalf("disabled: %d %s", r.status, r.raw)
	}
	if r := c.login("off@example.test", "Wrong-Password-1!"); r.body["error"] != "invalid credentials" {
		t.Fatalf("disabled + wrong password must not reveal state: %s", r.raw)
	}

	if _, err := e.store.UpsertOIDCUser(context.Background(), "https://issuer.test", "sub-1", "sso@example.test", "SSO", nil); err != nil {
		t.Fatal(err)
	}
	if r := c.login("sso@example.test", testutil.DefaultPassword); r.status != 401 {
		t.Fatalf("passwordless: %d %s", r.status, r.raw)
	}
}

func TestSecureCookieFlag(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.CookieSecure = true })
	testutil.CreateUser(t, e.store, "sec@example.test", "", false)
	r := e.client().login("sec@example.test", testutil.DefaultPassword)
	if r.status != 200 || !strings.Contains(r.header.Get("Set-Cookie"), "Secure") {
		t.Fatalf("expected Secure cookie: %d %v", r.status, r.header.Values("Set-Cookie"))
	}
}

func TestUpdateMeAndChangePassword(t *testing.T) {
	e := newEnv(t, nil)
	testutil.CreateUser(t, e.store, "me@example.test", "", false)
	testutil.CreateUser(t, e.store, "taken@example.test", "", false)
	c := e.client()

	if r := c.do("PATCH", "/api/v1/auth/me", map[string]string{"display_name": "x"}); r.status != 401 {
		t.Fatalf("unauthenticated patch: %d", r.status)
	}
	c.mustLogin("me@example.test", testutil.DefaultPassword)

	r := c.do("PATCH", "/api/v1/auth/me", map[string]string{"display_name": "  Ada L  "})
	if r.status != 200 || r.body["display_name"] != "Ada L" {
		t.Fatalf("patch name: %d %s", r.status, r.raw)
	}
	if r := c.do("PATCH", "/api/v1/auth/me", map[string]string{"display_name": "  "}); r.status != 400 {
		t.Fatalf("blank name: %d", r.status)
	}
	if r := c.do("PATCH", "/api/v1/auth/me", map[string]string{"email": "not-an-email"}); r.status != 400 {
		t.Fatalf("bad email: %d", r.status)
	}
	if r := c.do("PATCH", "/api/v1/auth/me", map[string]string{"email": "Taken@Example.test"}); r.status != 409 {
		t.Fatalf("duplicate email: %d %s", r.status, r.raw)
	}
	r = c.do("PATCH", "/api/v1/auth/me", map[string]string{"email": "New@Example.test"})
	if r.status != 200 || r.body["email"] != "new@example.test" {
		t.Fatalf("patch email: %d %s", r.status, r.raw)
	}

	pw := map[string]string{"current_password": "Wrong-Password-1!", "new_password": "Brand-New-Passw0rd!"}
	if r := c.do("POST", "/api/v1/auth/change-password", pw); r.status != 401 {
		t.Fatalf("wrong current: %d %s", r.status, r.raw)
	}
	pw = map[string]string{"current_password": testutil.DefaultPassword, "new_password": "short"}
	if r := c.do("POST", "/api/v1/auth/change-password", pw); r.status != 400 {
		t.Fatalf("weak new: %d %s", r.status, r.raw)
	}
	pw = map[string]string{"current_password": testutil.DefaultPassword, "new_password": "Brand-New-Passw0rd!"}
	if r := c.do("POST", "/api/v1/auth/change-password", pw); r.status != 204 {
		t.Fatalf("change: %d %s", r.status, r.raw)
	}
	other := e.client()
	if r := other.login("new@example.test", testutil.DefaultPassword); r.status != 401 {
		t.Fatalf("old password still works: %d", r.status)
	}
	if r := other.login("new@example.test", "Brand-New-Passw0rd!"); r.status != 200 {
		t.Fatalf("new password rejected: %d %s", r.status, r.raw)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client()
	last := 0
	for i := 0; i < 11; i++ {
		last = c.login("nobody@example.test", "Wrong-Password-1!").status
		if i < 10 && last != 401 {
			t.Fatalf("attempt %d: %d", i, last)
		}
	}
	if last != 429 {
		t.Fatalf("11th attempt: %d, want 429", last)
	}
}
