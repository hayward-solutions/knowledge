package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

const DefaultAttachmentMaxBytes int64 = 26214400

type AdminSettings struct {
	Email       string
	Password    string
	DisplayName string
}

type OIDCSettings struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	AdminGroup   string
	GroupsClaim  string
	EmailClaim   string
	NameClaim    string
	ProviderName string

	RequireEmailVerified bool
}

func (o OIDCSettings) Configured() bool {
	return o.IssuerURL != "" || o.ClientID != "" || o.ClientSecret != "" || o.RedirectURL != ""
}

type StorageSettings struct {
	Bucket         string
	Region         string
	Endpoint       string
	ForcePathStyle bool
	AccessKeyID    string
	SecretKey      string
	SessionToken   string
}

type Config struct {
	PostgresDSN        string
	APIPort            string
	MigrationsDir      string
	AppOrigins         []string
	PublicBaseURL      string
	CookieSecure       bool
	Admin              AdminSettings
	OIDC               OIDCSettings
	Storage            StorageSettings
	AttachmentMaxBytes int64
	TrustedProxies     []netip.Prefix
}

func Load() (*Config, error) {
	return LoadFrom(os.Getenv)
}

func LoadStorage() StorageSettings {
	return loadStorage(os.Getenv)
}

func LoadFrom(getenv func(string) string) (*Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		PostgresDSN:   strings.TrimSpace(getenv("POSTGRES_DSN")),
		APIPort:       get("API_PORT", "8080"),
		MigrationsDir: get("MIGRATIONS_DIR", "./migrations"),
		AppOrigins:    splitList(getenv("APP_ORIGINS")),
		PublicBaseURL: strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/"),
		CookieSecure:  parseBool(getenv("COOKIE_SECURE"), false),
		Admin: AdminSettings{
			Email:       get("ADMIN_EMAIL", ""),
			Password:    getenv("ADMIN_PASSWORD"),
			DisplayName: get("ADMIN_DISPLAY_NAME", ""),
		},
		OIDC: OIDCSettings{
			IssuerURL:    get("OIDC_ISSUER_URL", ""),
			ClientID:     get("OIDC_CLIENT_ID", ""),
			ClientSecret: getenv("OIDC_CLIENT_SECRET"),
			RedirectURL:  get("OIDC_REDIRECT_URL", ""),
			Scopes:       splitList(getenv("OIDC_SCOPES")),
			AdminGroup:   get("OIDC_ADMIN_GROUP", ""),
			GroupsClaim:  get("OIDC_GROUPS_CLAIM", "groups"),
			EmailClaim:   get("OIDC_EMAIL_CLAIM", "email"),
			NameClaim:    get("OIDC_NAME_CLAIM", "name"),
			ProviderName: get("OIDC_PROVIDER_NAME", "SSO"),

			RequireEmailVerified: parseBool(getenv("OIDC_REQUIRE_EMAIL_VERIFIED"), true),
		},
		Storage:            loadStorage(getenv),
		AttachmentMaxBytes: DefaultAttachmentMaxBytes,
	}

	if cfg.PostgresDSN == "" {
		return nil, errors.New("POSTGRES_DSN is required")
	}
	if port, err := strconv.Atoi(cfg.APIPort); err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("API_PORT %q is not a valid port", cfg.APIPort)
	}
	if v := strings.TrimSpace(getenv("ATTACHMENT_MAX_BYTES")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("ATTACHMENT_MAX_BYTES %q must be a positive integer", v)
		}
		cfg.AttachmentMaxBytes = n
	}
	proxies, err := ParseTrustedProxies(splitList(getenv("TRUSTED_PROXIES")))
	if err != nil {
		return nil, err
	}
	cfg.TrustedProxies = proxies
	if len(cfg.OIDC.Scopes) == 0 {
		cfg.OIDC.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	return cfg, nil
}

func loadStorage(getenv func(string) string) StorageSettings {
	region := strings.TrimSpace(getenv("STORAGE_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	return StorageSettings{
		Bucket:         strings.TrimSpace(getenv("STORAGE_S3_BUCKET")),
		Region:         region,
		Endpoint:       strings.TrimSpace(getenv("STORAGE_S3_ENDPOINT")),
		ForcePathStyle: parseBool(getenv("STORAGE_S3_FORCE_PATH_STYLE"), false),
		AccessKeyID:    getenv("AWS_ACCESS_KEY_ID"),
		SecretKey:      getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:   getenv("AWS_SESSION_TOKEN"),
	}
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(e, "/") {
			p, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES entry %q is not a valid CIDR", e)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES entry %q is not a valid IP or CIDR", e)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}
