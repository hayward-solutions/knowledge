package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadRequiresDSN(t *testing.T) {
	if _, err := LoadFrom(env(nil)); err == nil {
		t.Fatal("expected error when POSTGRES_DSN is missing")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{"POSTGRES_DSN": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIPort != "8080" || cfg.MigrationsDir != "./migrations" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.AttachmentMaxBytes != 26214400 {
		t.Fatalf("attachment max = %d", cfg.AttachmentMaxBytes)
	}
	if cfg.OIDC.GroupsClaim != "groups" || cfg.OIDC.EmailClaim != "email" || cfg.OIDC.NameClaim != "name" || cfg.OIDC.ProviderName != "SSO" {
		t.Fatalf("unexpected oidc defaults: %+v", cfg.OIDC)
	}
	if !cfg.OIDC.RequireEmailVerified {
		t.Fatal("OIDC_REQUIRE_EMAIL_VERIFIED must default to true")
	}
	if cfg.Storage.Region != "us-east-1" || cfg.Storage.Bucket != "" {
		t.Fatalf("unexpected storage defaults: %+v", cfg.Storage)
	}
	if cfg.CookieSecure {
		t.Fatal("cookie secure should default to false")
	}
	if len(cfg.OIDC.Scopes) != 4 {
		t.Fatalf("scopes = %v", cfg.OIDC.Scopes)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"POSTGRES_DSN":                "postgres://x",
		"API_PORT":                    "9000",
		"APP_ORIGINS":                 "http://a.test, http://b.test ,",
		"COOKIE_SECURE":               "true",
		"STORAGE_S3_FORCE_PATH_STYLE": "true",
		"ATTACHMENT_MAX_BYTES":        "1024",
		"OIDC_SCOPES":                 "openid,email",
		"PUBLIC_BASE_URL":             "https://k.test/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIPort != "9000" || len(cfg.AppOrigins) != 2 || !cfg.CookieSecure || !cfg.Storage.ForcePathStyle {
		t.Fatalf("unexpected: %+v", cfg)
	}
	if cfg.AttachmentMaxBytes != 1024 || len(cfg.OIDC.Scopes) != 2 || cfg.PublicBaseURL != "https://k.test" {
		t.Fatalf("unexpected: %+v", cfg)
	}
}

func TestLoadInvalid(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"port":  {"POSTGRES_DSN": "x", "API_PORT": "abc"},
		"bytes": {"POSTGRES_DSN": "x", "ATTACHMENT_MAX_BYTES": "-1"},
	} {
		if _, err := LoadFrom(env(m)); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestLoadOIDCRequireEmailVerifiedOverride(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{"POSTGRES_DSN": "x", "OIDC_REQUIRE_EMAIL_VERIFIED": "false"}))
	if err != nil || cfg.OIDC.RequireEmailVerified {
		t.Fatalf("override failed: %v %+v", err, cfg.OIDC)
	}
}

func TestTrustedProxies(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{"POSTGRES_DSN": "x"}))
	if err != nil || len(cfg.TrustedProxies) != 0 {
		t.Fatalf("default must be empty: %v %v", err, cfg.TrustedProxies)
	}
	cfg, err = LoadFrom(env(map[string]string{"POSTGRES_DSN": "x", "TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.5 ,fd00::/8"}))
	if err != nil || len(cfg.TrustedProxies) != 3 {
		t.Fatalf("parse: %v %v", err, cfg.TrustedProxies)
	}
	if cfg.TrustedProxies[1].String() != "192.168.1.5/32" {
		t.Fatalf("bare ip: %v", cfg.TrustedProxies[1])
	}
	for _, bad := range []string{"nope", "10.0.0.0/99", "10.0.0.256"} {
		if _, err := LoadFrom(env(map[string]string{"POSTGRES_DSN": "x", "TRUSTED_PROXIES": bad})); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
