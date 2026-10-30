package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/hayward-solutions/knowledge/api/internal/config"
)

type OIDCConfig struct {
	Provider     *oidc.Provider
	OAuth2       *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
	Issuer       string
	AdminGroup   string
	GroupsClaim  string
	EmailClaim   string
	NameClaim    string
	RedirectURL  string
	ProviderName string
}

func LoadOIDC(ctx context.Context, s config.OIDCSettings) (*OIDCConfig, error) {
	if !s.Configured() {
		return nil, nil
	}
	if s.IssuerURL == "" || s.ClientID == "" || s.ClientSecret == "" || s.RedirectURL == "" {
		return nil, errors.New("OIDC partially configured: OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URL are all required")
	}
	provider, err := oidc.NewProvider(ctx, s.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}
	scopes := s.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}
	return &OIDCConfig{
		Provider: provider,
		OAuth2: &oauth2.Config{
			ClientID:     s.ClientID,
			ClientSecret: s.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  s.RedirectURL,
			Scopes:       scopes,
		},
		Verifier:     provider.Verifier(&oidc.Config{ClientID: s.ClientID}),
		Issuer:       s.IssuerURL,
		AdminGroup:   s.AdminGroup,
		GroupsClaim:  s.GroupsClaim,
		EmailClaim:   s.EmailClaim,
		NameClaim:    s.NameClaim,
		RedirectURL:  s.RedirectURL,
		ProviderName: s.ProviderName,
	}, nil
}

func GroupsFrom(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}

func HasGroup(groups []string, target string) bool {
	if target == "" {
		return false
	}
	for _, g := range groups {
		if g == target {
			return true
		}
	}
	return false
}
