package auth

import (
	"context"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/config"
)

func TestLoadOIDCDisabled(t *testing.T) {
	c, err := LoadOIDC(context.Background(), config.OIDCSettings{})
	if err != nil || c != nil {
		t.Fatalf("want nil,nil got %v,%v", c, err)
	}
}

func TestLoadOIDCPartial(t *testing.T) {
	_, err := LoadOIDC(context.Background(), config.OIDCSettings{IssuerURL: "https://issuer.test"})
	if err == nil {
		t.Fatal("expected partial configuration error")
	}
}

func TestGroupsFrom(t *testing.T) {
	if g := GroupsFrom([]any{"a", 3, "b"}); len(g) != 2 || g[1] != "b" {
		t.Fatalf("got %v", g)
	}
	if g := GroupsFrom("solo"); len(g) != 1 {
		t.Fatalf("got %v", g)
	}
	if g := GroupsFrom(nil); g != nil {
		t.Fatalf("got %v", g)
	}
	if !HasGroup([]string{"x", "admins"}, "admins") || HasGroup([]string{"x"}, "") || HasGroup(nil, "x") {
		t.Fatal("HasGroup broken")
	}
}
