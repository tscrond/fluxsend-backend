package config

type AuthConfig struct {
	EnableGoogleAuth   bool
	EnableGithubAuth   bool
	EnablePasswordAuth bool
	GoogleOAuthConfig  *GoogleOAuthConfig
	GithubOAuthConfig  *GithubOAuthConfig
	TokenEncryptionKey string
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

type GithubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}
