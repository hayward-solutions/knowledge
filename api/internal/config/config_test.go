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
