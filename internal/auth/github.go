package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GithubAuthProvider struct {
	oauthConfig *oauth2.Config
}

func NewGithubAuthProvider(cfg config.GithubOAuthConfig) AuthProvider {
	oauthConf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     github.Endpoint,
	}
	return &GithubAuthProvider{oauthConfig: oauthConf}
}

func (gp *GithubAuthProvider) Name() string {
	return "github"
}

func (gp *GithubAuthProvider) GetAuthURL(state string) string {
	return fmt.Sprintf(
		"%s?scope=user:email&client_id=%s&state=%s",
		gp.oauthConfig.Endpoint.AuthURL,
		gp.oauthConfig.ClientID,
		state,
	)
}

func (gp *GithubAuthProvider) HandleCallback(ctx context.Context, r *http.Request) (*AuthResult, error) {
	oauthErr := r.URL.Query().Get("error")
	if oauthErr != "" {
		return nil, fmt.Errorf("github oauth error: %s (%s)", oauthErr, r.URL.Query().Get("error_description"))
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("missing authorization code")
	}

	token, err := gp.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth token exchange failed")
	}

	client := gp.oauthConfig.Client(ctx, token)

	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("github userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github userinfo returned status %d", resp.StatusCode)
	}

	var userInfo struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decoding github userinfo: %w", err)
	}

	// GitHub users can hide their email; fetch from /user/emails if missing
	email := userInfo.Email
	emailVerified := false

	emailResp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return nil, fmt.Errorf("github /user/emails request failed: %w", err)
	}
	defer emailResp.Body.Close()

	if emailResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github /user/emails returned status %d", emailResp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(emailResp.Body).Decode(&emails); err != nil {
		return nil, fmt.Errorf("decoding github emails: %w", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			email = e.Email
			emailVerified = true
			break
		}
	}

	if email == "" {
		return nil, fmt.Errorf("no verified primary email on github account")
	}

	// Use login as display name if no name is set
	name := userInfo.Name
	if name == "" {
		name = userInfo.Login
	}

	return &AuthResult{
		Provider:       "github",
		ProviderUserID: fmt.Sprintf("%d", userInfo.ID),
		Email:          email,
		EmailVerified:  emailVerified,
		Name:           name,
		AvatarURL:      userInfo.AvatarURL,
		AccessToken:    token.AccessToken,
	}, nil
}

func (gp *GithubAuthProvider) Logout(ctx context.Context, accessToken string) error {
	return nil
}
