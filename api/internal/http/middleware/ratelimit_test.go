package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPLimiterBurstAndRefill(t *testing.T) {
	l := newIPLimiter(authBurst, authRefillEvery)
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

func TestIPLimiterSweep(t *testing.T) {
	l := newIPLimiter(authBurst, authRefillEvery)
	now := time.Now()
	l.allow("old", now)
	l.allow("trigger", now.Add(time.Hour))
	if _, ok := l.entries["old"]; ok {
		t.Fatal("idle entry should be swept")
	}
}

func TestLimiterMiddlewareResponds429(t *testing.T) {
	l := newIPLimiter(2, time.Hour)
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
