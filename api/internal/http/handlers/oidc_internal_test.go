package handlers

import (
	"strings"
	"testing"
)

func TestSafeNext(t *testing.T) {
	good := []string{"/", "/spaces", "/spaces/ENG/pages/1?x=y"}
	bad := []string{
		"", "spaces", "//evil.test", "/\\evil.test", "https://evil.test", "javascript:alert(1)",
		"/ok\r\nSet-Cookie: x=1", "/a\tb", "/a\x00b", "/a\x7fb", "/a\\b", "/ok/\\..\\evil", "/" + strings.Repeat("a", 3000),
		"http://evil.test/", "///evil.test", "/%zz",
	}
	for _, p := range good {
		if !safeNext(p) {
			t.Errorf("safeNext(%q) = false", p)
		}
	}
	for _, p := range bad {
		if safeNext(p) {
			t.Errorf("safeNext(%q) = true", p)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if e, ok := normalizeEmail("  Ada@Example.TEST "); !ok || e != "ada@example.test" {
		t.Errorf("got %q %v", e, ok)
	}
	for _, bad := range []string{"", "x", "a@", "Ada <ada@example.test>", "a b@example.test"} {
		if _, ok := normalizeEmail(bad); ok {
			t.Errorf("normalizeEmail(%q) accepted", bad)
		}
	}
}
