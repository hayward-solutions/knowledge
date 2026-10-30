package handlers_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

type fakeIdP struct {
	t        *testing.T
	server   *httptest.Server
	key      *rsa.PrivateKey
	mu       sync.Mutex
	nonce    string
	claims   map[string]any
	userinfo map[string]any
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{t: t, key: key}
	mux := http.NewServeMux()
	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	base := idp.server.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/authorize",
			"token_endpoint":                        base + "/token",
			"jwks_uri":                              base + "/jwks",
			"userinfo_endpoint":                     base + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idp.mu.Lock()
		defer idp.mu.Unlock()
		claims := map[string]any{
			"iss": base, "aud": "cid", "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "nonce": idp.nonce,
		}
		for k, v := range idp.claims {
			claims[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "id_token": idp.sign(claims),
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		idp.mu.Lock()
		defer idp.mu.Unlock()
		_ = json.NewEncoder(w).Encode(idp.userinfo)
	})
	return idp
}

func (i *fakeIdP) sign(claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	signing := b64(header) + "." + b64(payload)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, i.key, crypto.SHA256, sum[:])
	if err != nil {
		i.t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func (i *fakeIdP) set(claims, userinfo map[string]any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.claims = claims
	i.userinfo = userinfo
}

type oidcEnv struct {
	*env
	idp *fakeIdP
}

func newOIDCEnv(t *testing.T, mutate func(*config.OIDCSettings)) *oidcEnv {
	t.Helper()
	idp := newFakeIdP(t)
	s := testutil.NewDB(t)
	settings := config.OIDCSettings{
		IssuerURL: idp.server.URL, ClientID: "cid", ClientSecret: "sec",
		RedirectURL: "http://app.test/cb", Scopes: []string{"openid", "email"},
		GroupsClaim: "groups", EmailClaim: "email", NameClaim: "name", ProviderName: "Fake",
		RequireEmailVerified: true,
	}
	if mutate != nil {
		mutate(&settings)
	}
	oidcCfg, err := auth.LoadOIDC(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	presence := events.NewPresence()
	t.Cleanup(presence.Close)
	router := apphttp.NewRouter(handlers.Deps{
		Store: s, Bus: events.NewBus(), Hub: events.NewHub(), Presence: presence,
		Config:  &config.Config{PublicBaseURL: "http://app.test", TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}},
		OIDC:    oidcCfg,
		Version: "test",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &oidcEnv{env: &env{t: t, store: s, server: srv}, idp: idp}
}

type callbackResult struct {
	status   int
	location string
	body     string
	cookies  []*http.Cookie
}

func (e *oidcEnv) signIn(claims, userinfo map[string]any) callbackResult {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	n := ipCounter.Add(1)
	hc := &http.Client{
		Jar:           jar,
		Transport:     ipTransport{ip: "10.9." + string(rune('0'+n%10)) + ".1"},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Get(e.server.URL + "/api/v1/auth/oidc/login")
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.StatusCode != 302 {
		e.t.Fatalf("login redirect: %d %v", resp.StatusCode, err)
	}
	e.idp.mu.Lock()
	e.idp.nonce = loc.Query().Get("nonce")
	e.idp.mu.Unlock()
	e.idp.set(claims, userinfo)

	resp, err = hc.Get(e.server.URL + "/api/v1/auth/oidc/callback?code=abc&state=" + url.QueryEscape(loc.Query().Get("state")))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	b := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(b)
		buf.Write(b[:n])
		if err != nil {
			break
		}
	}
	return callbackResult{status: resp.StatusCode, location: resp.Header.Get("Location"), body: buf.String(), cookies: resp.Cookies()}
}

func (r callbackResult) hasSession() bool {
	for _, c := range r.cookies {
		if c.Name == "sid" && c.Value != "" {
			return true
		}
	}
	return false
}

func (e *oidcEnv) identities() int {
	var n int
	_ = e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM oidc_identities`).Scan(&n)
	return n
}

func TestOIDCCallbackRefusesToLinkUnverifiedEmail(t *testing.T) {
	e := newOIDCEnv(t, nil)
	testutil.CreateUser(t, e.store, "victim@example.test", "", true)

	for name, claims := range map[string]map[string]any{
		"claim false":   {"sub": "s1", "email": "victim@example.test", "email_verified": false},
		"string false":  {"sub": "s1", "email": "victim@example.test", "email_verified": "false"},
		"claim missing": {"sub": "s1", "email": "victim@example.test"},
	} {
		res := e.signIn(claims, map[string]any{"sub": "s1"})
		if res.status != 403 || res.hasSession() || res.location != "" {
			t.Fatalf("%s: status %d session %v location %q", name, res.status, res.hasSession(), res.location)
		}
		if !strings.Contains(res.body, "Sign-in blocked") || !strings.Contains(res.body, "has not verified") {
			t.Fatalf("%s: unclear error page: %s", name, res.body)
		}
		if e.identities() != 0 {
			t.Fatalf("%s: identity must not be created", name)
		}
	}
}

func TestOIDCCallbackLinksVerifiedEmail(t *testing.T) {
	cases := map[string]struct {
		claims   map[string]any
		userinfo map[string]any
	}{
		"bool true":         {map[string]any{"sub": "s", "email": "user@example.test", "email_verified": true}, nil},
		"string true":       {map[string]any{"sub": "s", "email": "user@example.test", "email_verified": "true"}, nil},
		"userinfo verified": {map[string]any{"sub": "s", "email": "user@example.test"}, map[string]any{"sub": "s", "email_verified": true}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newOIDCEnv(t, nil)
			existing := testutil.CreateUser(t, e.store, "user@example.test", "", false)
			res := e.signIn(c.claims, c.userinfo)
			if res.status != 302 || res.location != "http://app.test/" || !res.hasSession() {
				t.Fatalf("status %d location %q session %v body %s", res.status, res.location, res.hasSession(), res.body)
			}
			var linked string
			if err := e.store.Pool.QueryRow(context.Background(), `SELECT user_id::text FROM oidc_identities`).Scan(&linked); err != nil || linked != existing.ID.String() {
				t.Fatalf("identity not linked to the existing account: %v %s", err, linked)
			}
		})
	}
}

func TestOIDCCallbackVerificationDisabledOrNotNeeded(t *testing.T) {
	t.Run("requirement off", func(t *testing.T) {
		e := newOIDCEnv(t, func(s *config.OIDCSettings) { s.RequireEmailVerified = false })
		testutil.CreateUser(t, e.store, "user@example.test", "", false)
		res := e.signIn(map[string]any{"sub": "s", "email": "user@example.test"}, nil)
		if res.status != 302 || !res.hasSession() || e.identities() != 1 {
			t.Fatalf("status %d identities %d", res.status, e.identities())
		}
	})
	t.Run("new account unverified", func(t *testing.T) {
		e := newOIDCEnv(t, nil)
		res := e.signIn(map[string]any{"sub": "s", "email": "brand-new@example.test", "email_verified": false}, nil)
		if res.status != 302 || !res.hasSession() || e.identities() != 1 {
			t.Fatalf("status %d identities %d", res.status, e.identities())
		}
	})
	t.Run("already linked identity unverified", func(t *testing.T) {
		e := newOIDCEnv(t, nil)
		verified := map[string]any{"sub": "s", "email": "user@example.test", "email_verified": true}
		testutil.CreateUser(t, e.store, "user@example.test", "", false)
		if res := e.signIn(verified, nil); res.status != 302 {
			t.Fatalf("first login: %d", res.status)
		}
		unverified := map[string]any{"sub": "s", "email": "user@example.test", "email_verified": false}
		if res := e.signIn(unverified, nil); res.status != 302 || !res.hasSession() {
			t.Fatalf("second login: %d", res.status)
		}
	})
}

func TestOIDCCallbackAdminGroupNeverDemotesLastAdmin(t *testing.T) {
	e := newOIDCEnv(t, func(s *config.OIDCSettings) { s.AdminGroup = "wiki-admins" })
	boss := testutil.CreateUser(t, e.store, "boss@example.test", "", true)

	claims := map[string]any{"sub": "boss", "email": "boss@example.test", "email_verified": true, "groups": []string{"wiki-admins"}}
	if res := e.signIn(claims, nil); res.status != 302 {
		t.Fatalf("link: %d", res.status)
	}
	claims["groups"] = []string{"other"}
	if res := e.signIn(claims, nil); res.status != 302 || !res.hasSession() {
		t.Fatalf("login without group: %d", res.status)
	}
	got, err := e.store.GetUser(context.Background(), boss.ID)
	if err != nil || !got.IsSystemAdmin {
		t.Fatalf("last admin must keep the flag: %v %+v", err, got)
	}

	testutil.CreateUser(t, e.store, "second@example.test", "", true)
	if res := e.signIn(claims, nil); res.status != 302 {
		t.Fatalf("login with spare admin: %d", res.status)
	}
	got, _ = e.store.GetUser(context.Background(), boss.ID)
	if got.IsSystemAdmin {
		t.Fatal("with a spare admin the group mapping must demote")
	}
}

func TestOIDCLoginNextParameter(t *testing.T) {
	e := newOIDCEnv(t, nil)
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	hasNext := func(next string) bool {
		resp, err := hc.Get(e.server.URL + "/api/v1/auth/oidc/login?next=" + url.QueryEscape(next))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		for _, c := range resp.Cookies() {
			if c.Name == "oidc_next" {
				return true
			}
		}
		return false
	}
	if !hasNext("/spaces/ENG?x=1") {
		t.Error("same-origin path must be remembered")
	}
	for _, bad := range []string{"//evil.test", "/\\evil.test", "https://evil.test", "/a\r\nb", "/a\tb", "/a\\b"} {
		if hasNext(bad) {
			t.Errorf("next %q must be rejected", bad)
		}
	}
}
