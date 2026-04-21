package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/tscrond/fluxsend-backend/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleAuthProvider struct {
	oauthConfig *oauth2.Config
}

func NewGoogleAuthProvider(cfg config.GoogleOAuthConfig) (AuthProvider, error) {
	oauthConf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     google.Endpoint,
	}
	return &GoogleAuthProvider{oauthConfig: oauthConf}, nil
}

func (gp *GoogleAuthProvider) Name() string {
	return "google"
}

func (gp *GoogleAuthProvider) GetAuthURL(state string) string {
	return gp.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (gp *GoogleAuthProvider) HandleCallback(ctx context.Context, r *http.Request) (*AuthResult, error) {
	oauthErr := r.URL.Query().Get("error")
	if oauthErr != "" {
		return nil, fmt.Errorf("oauth error: %s (%s)", oauthErr, r.URL.Query().Get("error_description"))
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("missing authorization code")
	}

	token, err := gp.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	client := gp.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}

	emailVerified := "false"
	if userInfo.EmailVerified {
		emailVerified = "true"
	}

	return &AuthResult{
		Provider:       "google",
		ProviderUserID: userInfo.ID,
		Email:          userInfo.Email,
		EmailVerified:  emailVerified,
		Name:           userInfo.Name,
		AvatarURL:      userInfo.Picture,
		AccessToken:    token.AccessToken,
	}, nil
}

func (gp *GoogleAuthProvider) Logout(ctx context.Context, accessToken string) error {
	params := url.Values{}
	params.Set("token", accessToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/revoke?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token revocation failed with status %d", resp.StatusCode)
	}
	return nil
}
