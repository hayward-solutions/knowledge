package bootstrap_test

import (
	"context"
	"testing"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/bootstrap"
	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestAdminBootstrap(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()

	if err := bootstrap.Admin(ctx, s, config.AdminSettings{}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountUsers(ctx); n != 0 {
		t.Fatalf("no env must create nothing, got %d users", n)
	}

	cfg := config.AdminSettings{Email: "Admin@Example.test", Password: "Bootstrap-Passw0rd!", DisplayName: "Root"}
	if err := bootstrap.Admin(ctx, s, cfg); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUserByEmail(ctx, "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsSystemAdmin || u.DisplayName != "Root" || u.PasswordHash == nil {
		t.Fatalf("bootstrap user: %+v", u)
	}
	if ok, err := auth.VerifyPassword(cfg.Password, *u.PasswordHash); err != nil || !ok {
		t.Fatalf("bootstrap password does not verify: %v %v", ok, err)
	}

	cfg.Email = "other@example.test"
	if err := bootstrap.Admin(ctx, s, cfg); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountUsers(ctx); n != 1 {
		t.Fatalf("bootstrap must be skipped when users exist, got %d", n)
	}
}

func TestAdminBootstrapRejectsWeakPassword(t *testing.T) {
	s := testutil.NewDB(t)
	err := bootstrap.Admin(context.Background(), s, config.AdminSettings{Email: "a@example.test", Password: "weak"})
	if err == nil {
		t.Fatal("weak bootstrap password must be rejected")
	}
}
