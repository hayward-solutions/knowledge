package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	authBurst        = 10
	authRefillEvery  = 6 * time.Second
	limiterIdleAfter = 30 * time.Minute
	limiterSweepEach = 10 * time.Minute
)

type ipLimiter struct {
	mu        sync.Mutex
	burst     int
	every     time.Duration
	entries   map[string]*limiterEntry
	lastSweep time.Time
}

type limiterEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

func newIPLimiter(burst int, every time.Duration) *ipLimiter {
	return &ipLimiter{
		burst:     burst,
		every:     every,
		entries:   map[string]*limiterEntry{},
		lastSweep: time.Now(),
	}
}

func (l *ipLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > limiterSweepEach {
		for k, e := range l.entries {
			if now.Sub(e.seen) > limiterIdleAfter {
				delete(l.entries, k)
			}
		}
		l.lastSweep = now
	}
	e, ok := l.entries[key]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.entries[key] = e
	}
	e.seen = now
	return e.limiter.AllowN(now, 1)
}

var defaultAuthLimiter = newIPLimiter(authBurst, authRefillEvery)

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func AuthLimiter() func(http.Handler) http.Handler {
	return limiterMiddleware(defaultAuthLimiter)
}

func limiterMiddleware(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r), time.Now()) {
				w.Header().Set("Retry-After", "60")
				WriteError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
