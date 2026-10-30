package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const maxForwardedHeaderLen = 4096

func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	if len(trusted) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip, ok := forwardedClient(r, trusted); ok {
				r.RemoteAddr = ip.String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isTrusted(trusted []netip.Prefix, a netip.Addr) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func forwardedClient(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !isTrusted(trusted, peer.Unmap()) {
		return netip.Addr{}, false
	}

	xff := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if len(xff) > maxForwardedHeaderLen {
		return netip.Addr{}, false
	}
	if strings.TrimSpace(xff) != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil {
				return netip.Addr{}, false
			}
			a = a.Unmap()
			if isTrusted(trusted, a) {
				continue
			}
			return a, true
		}
		return netip.Addr{}, false
	}

	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" && len(v) <= 64 {
		if a, err := netip.ParseAddr(v); err == nil {
			return a.Unmap(), true
		}
	}
	return netip.Addr{}, false
}
