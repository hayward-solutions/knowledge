package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func resolve(t *testing.T, trusted []string, remote string, headers map[string][]string) string {
	t.Helper()
	var prefixes []netip.Prefix
	for _, c := range trusted {
		prefixes = append(prefixes, netip.MustParsePrefix(c))
	}
	var got string
	h := RealIP(prefixes)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestRealIPNeverTrustsHeadersByDefault(t *testing.T) {
	got := resolve(t, nil, "203.0.113.9:4444", map[string][]string{
		"X-Forwarded-For": {"1.2.3.4"}, "X-Real-Ip": {"5.6.7.8"}, "True-Client-Ip": {"9.9.9.9"},
	})
	if got != "203.0.113.9:4444" {
		t.Fatalf("remote addr rewritten: %s", got)
	}
}

func TestRealIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	got := resolve(t, []string{"10.0.0.0/8"}, "203.0.113.9:4444", map[string][]string{"X-Forwarded-For": {"1.2.3.4"}})
	if got != "203.0.113.9:4444" {
		t.Fatalf("untrusted peer must not control the address: %s", got)
	}
}

func TestRealIPHonoursTrustedProxy(t *testing.T) {
	trusted := []string{"10.0.0.0/8", "fd00::/8"}
	cases := []struct {
		name    string
		remote  string
		headers map[string][]string
		want    string
	}{
		{"single entry", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"198.51.100.7"}}, "198.51.100.7"},
		{"spoofed left entry is ignored", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"6.6.6.6, 198.51.100.7"}}, "198.51.100.7"},
		{"trusted hops are skipped", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"198.51.100.7, 10.2.2.2, 10.3.3.3"}}, "198.51.100.7"},
		{"multiple header lines", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"6.6.6.6", "198.51.100.7"}}, "198.51.100.7"},
		{"ipv6 client", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"2001:db8::1"}}, "2001:db8::1"},
		{"ipv6 trusted peer", "[fd00::5]:80", map[string][]string{"X-Forwarded-For": {"198.51.100.7"}}, "198.51.100.7"},
		{"x-real-ip fallback", "10.1.1.1:80", map[string][]string{"X-Real-Ip": {"198.51.100.8"}}, "198.51.100.8"},
		{"xff wins over x-real-ip", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"198.51.100.7"}, "X-Real-Ip": {"198.51.100.8"}}, "198.51.100.7"},
		{"garbage entry leaves peer", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"198.51.100.7, not-an-ip"}}, "10.1.1.1:80"},
		{"all hops trusted leaves peer", "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {"10.2.2.2, 10.3.3.3"}}, "10.1.1.1:80"},
		{"no headers leaves peer", "10.1.1.1:80", nil, "10.1.1.1:80"},
		{"garbage x-real-ip leaves peer", "10.1.1.1:80", map[string][]string{"X-Real-Ip": {"nope"}}, "10.1.1.1:80"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolve(t, trusted, c.remote, c.headers); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestRealIPRejectsOversizedHeader(t *testing.T) {
	long := make([]byte, maxForwardedHeaderLen+10)
	for i := range long {
		long[i] = '1'
	}
	got := resolve(t, []string{"10.0.0.0/8"}, "10.1.1.1:80", map[string][]string{"X-Forwarded-For": {string(long)}})
	if got != "10.1.1.1:80" {
		t.Fatalf("oversized header must be ignored: %s", got)
	}
}
