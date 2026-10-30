package middleware

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	authBurst        = 10
	authRefillEvery  = 6 * time.Second
	emailBurst       = 5
	emailRefillEvery = 12 * time.Second
	maxLimiterKeys   = 10000
	maxLimiterKeyLen = 64
)

type keyLimiter struct {
	mu      sync.Mutex
	burst   int
	every   time.Duration
	max     int
	entries map[string]*list.Element
	order   *list.List
}

type limiterEntry struct {
	key     string
	limiter *rate.Limiter
}

func newKeyLimiter(burst int, every time.Duration, max int) *keyLimiter {
	return &keyLimiter{
		burst:   burst,
		every:   every,
		max:     max,
		entries: map[string]*list.Element{},
		order:   list.New(),
	}
}

func boundedKey(key string) string {
	if len(key) <= maxLimiterKeyLen {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (l *keyLimiter) allow(key string, now time.Time) bool {
	key = boundedKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.entries[key]; ok {
		l.order.MoveToFront(el)
		return el.Value.(*limiterEntry).limiter.AllowN(now, 1)
	}
	e := &limiterEntry{key: key, limiter: rate.NewLimiter(rate.Every(l.every), l.burst)}
	l.entries[key] = l.order.PushFront(e)
	for len(l.entries) > l.max {
		oldest := l.order.Back()
		l.order.Remove(oldest)
		delete(l.entries, oldest.Value.(*limiterEntry).key)
	}
	return e.limiter.AllowN(now, 1)
}

func (l *keyLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

type AuthLimits struct {
	ip    *keyLimiter
	email *keyLimiter
}

func NewAuthLimits() *AuthLimits {
	return &AuthLimits{
		ip:    newKeyLimiter(authBurst, authRefillEvery, maxLimiterKeys),
		email: newKeyLimiter(emailBurst, emailRefillEvery, maxLimiterKeys),
	}
}

var defaultAuthLimits = NewAuthLimits()

func (a *AuthLimits) IP() func(http.Handler) http.Handler {
	return limiterMiddleware(a.ip)
}

func (a *AuthLimits) AllowEmail(email string) bool {
	if a == nil {
		return true
	}
	return a.email.allow(email, time.Now())
}

func AuthLimiter() func(http.Handler) http.Handler {
	return defaultAuthLimits.IP()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func limiterMiddleware(l *keyLimiter) func(http.Handler) http.Handler {
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
