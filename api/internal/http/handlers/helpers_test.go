package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

var ipCounter atomic.Int32

type env struct {
	t      *testing.T
	store  *store.Store
	server *httptest.Server
}

type ipTransport struct{ ip string }

func (t ipTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Forwarded-For", t.ip)
	return http.DefaultTransport.RoundTrip(r)
}

func newEnv(t *testing.T, mutate func(*config.Config)) *env {
	t.Helper()
	s := testutil.NewDB(t)
	cfg := &config.Config{AppOrigins: []string{"http://app.test"}}
	if mutate != nil {
		mutate(cfg)
	}
	hub := events.NewHub()
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store:    s,
		Bus:      events.NewBus(),
		Hub:      hub,
		Presence: presence,
		Config:   cfg,
		Version:  "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &env{t: t, store: s, server: srv}
}

type client struct {
	e    *env
	http *http.Client
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	n := ipCounter.Add(1)
	ip := fmt.Sprintf("10.%d.%d.%d", (n>>16)&255, (n>>8)&255, n&255)
	return &client{e: e, http: &http.Client{Jar: jar, Transport: ipTransport{ip: ip}}}
}

type response struct {
	status int
	header http.Header
	raw    []byte
	body   map[string]any
	list   []map[string]any
}

func (c *client) do(method, path string, body any) response {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.e.server.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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

func (c *client) login(email, password string) response {
	c.e.t.Helper()
	return c.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": password})
}

func (c *client) mustLogin(email, password string) {
	c.e.t.Helper()
	if r := c.login(email, password); r.status != 200 {
		c.e.t.Fatalf("login %s: %d %s", email, r.status, r.raw)
	}
}
