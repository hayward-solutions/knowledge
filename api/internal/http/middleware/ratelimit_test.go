package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKeyLimiterBurstAndRefill(t *testing.T) {
	l := newKeyLimiter(authBurst, authRefillEvery, maxLimiterKeys)
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !l.allow("1.2.3.4", now) {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if l.allow("1.2.3.4", now) {
		t.Fatal("11th request should be limited")
	}
	if !l.allow("5.6.7.8", now) {
		t.Fatal("other ip should not be limited")
	}
	if l.allow("1.2.3.4", now.Add(3*time.Second)) {
		t.Fatal("no token should be available after 3s")
	}
	if !l.allow("1.2.3.4", now.Add(7*time.Second)) {
		t.Fatal("one token should have refilled after 7s")
	}
	if l.allow("1.2.3.4", now.Add(7*time.Second)) {
		t.Fatal("only one token should have refilled")
	}
}

func TestKeyLimiterEvictsLeastRecentlyUsed(t *testing.T) {
	l := newKeyLimiter(1, time.Hour, 3)
	now := time.Now()
	for _, k := range []string{"a", "b", "c"} {
		if !l.allow(k, now) {
			t.Fatalf("%s first request", k)
		}
	}
	if l.allow("a", now) {
		t.Fatal("a is exhausted and still tracked")
	}
	if !l.allow("d", now) {
		t.Fatal("d first request")
	}
	if l.size() != 3 {
		t.Fatalf("size = %d, want 3", l.size())
	}
	if !l.allow("b", now) {
		t.Fatal("b was the least recently used and must have been evicted (fresh bucket)")
	}
	if l.allow("a", now) {
		t.Fatal("a was recently used and must still be tracked")
	}
}

func TestKeyLimiterBoundsMemoryUnderFlood(t *testing.T) {
	l := newKeyLimiter(5, time.Minute, maxLimiterKeys)
	now := time.Now()
	for i := 0; i < maxLimiterKeys*3; i++ {
		l.allow(fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255), now)
	}
	if l.size() != maxLimiterKeys {
		t.Fatalf("size = %d, want %d", l.size(), maxLimiterKeys)
	}
}

func TestKeyLimiterLongKeysAreHashed(t *testing.T) {
	l := newKeyLimiter(1, time.Hour, 10)
	now := time.Now()
	long := strings.Repeat("x", 5000)
	if !l.allow(long, now) || l.allow(long, now) {
		t.Fatal("long key must still be limited consistently")
	}
	for k := range l.entries {
		if len(k) > maxLimiterKeyLen {
			t.Fatalf("stored key length %d", len(k))
		}
	}
	if !l.allow(long+"y", now) {
		t.Fatal("different long keys must not collide")
	}
}

func TestLimiterMiddlewareResponds429(t *testing.T) {
	l := newKeyLimiter(2, time.Hour, 100)
	h := limiterMiddleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	codes := []int{}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
		if rec.Code == http.StatusTooManyRequests {
			if rec.Header().Get("Retry-After") == "" || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("429 response headers wrong: %v", rec.Header())
			}
		}
	}
	if codes[0] != 204 || codes[1] != 204 || codes[2] != 429 {
		t.Fatalf("codes = %v", codes)
	}
}

func TestEmailLimiterBurstFiveIndependentPerEmail(t *testing.T) {
	a := NewAuthLimits()
	for i := 0; i < 5; i++ {
		if !a.AllowEmail("victim@example.test") {
			t.Fatalf("attempt %d should be allowed", i)
		}
	}
	if a.AllowEmail("victim@example.test") {
		t.Fatal("6th attempt for the same email must be limited")
	}
	if !a.AllowEmail("other@example.test") {
		t.Fatal("other emails are independent")
	}
}

func TestAuthLimitsAreIndependentInstances(t *testing.T) {
	a, b := NewAuthLimits(), NewAuthLimits()
	for i := 0; i < 6; i++ {
		a.AllowEmail("x@example.test")
	}
	if !b.AllowEmail("x@example.test") {
		t.Fatal("instances must not share state")
	}
}
