package handlers_test

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	e := newEnv(t, nil)
	r := e.client().do("GET", "/health", nil)
	if r.status != 200 || r.body["ok"] != true || r.body["version"] != "test" {
		t.Fatalf("health: %d %s", r.status, r.raw)
	}
}

func TestOIDCDisabled(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client()
	r := c.do("GET", "/api/v1/auth/oidc/config", nil)
	if r.status != 200 || r.body["enabled"] != false {
		t.Fatalf("oidc config: %d %s", r.status, r.raw)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, p := range []string{"/api/v1/auth/oidc/login", "/api/v1/auth/oidc/callback"} {
		resp, err := noFollow.Get(e.server.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
}

func TestNotFoundAndMethodNotAllowedUseJSON(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client()
	if r := c.do("GET", "/api/v1/nope", nil); r.status != 404 || r.body["error"] != "not found" {
		t.Fatalf("404: %d %s", r.status, r.raw)
	}
	if r := c.do("DELETE", "/api/v1/auth/login", nil); r.status != 405 || r.body["error"] == nil {
		t.Fatalf("405: %d %s", r.status, r.raw)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t, nil)
	do := func(origin, method string) *http.Response {
		req, _ := http.NewRequest(method, e.server.URL+"/api/v1/auth/me", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	resp := do("http://app.test", "GET")
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://app.test" || resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin headers: %v", resp.Header)
	}
	if resp := do("http://evil.test", "GET"); resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("disallowed origin got CORS headers: %v", resp.Header)
	}
	resp = do("http://app.test", "OPTIONS")
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
}
