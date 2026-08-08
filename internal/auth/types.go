package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/config"
)

type AuthProvider interface {
	Name() string
	GetAuthURL(state string) string
	HandleCallback(ctx context.Context, r *http.Request) (*AuthResult, error)
	Logout(ctx context.Context, accessToken string) error
}

type AuthResult struct {
	Provider       string
	ProviderUserID string
	Email          string
	EmailVerified  bool
	Name           string
	AvatarURL      string
	AccessToken    string
}

func InitAuthProviders(authConfig config.AuthConfig, providers ...string) (map[string]AuthProvider, error) {
	authProviders := make(map[string]AuthProvider)

	for _, provider := range providers {
		switch provider {
		case "google":
			var googleAuthProvider AuthProvider
			googleAuthProvider, err := NewGoogleAuthProvider(*authConfig.GoogleOAuthConfig)
			if err != nil {
				return nil, err
			}
			authProviders[provider] = googleAuthProvider
		case "github":
			authProviders[provider] = NewGithubAuthProvider(*authConfig.GithubOAuthConfig)
		case "password":
			authProviders[provider] = NewPasswordAuthProvider()
		default:
			return nil, errors.New("unknown_provider")
		}
	}
	return authProviders, nil
}
