package config

type AuthConfig struct {
	GoogleOAuthConfig  GoogleOAuthConfig
	GithubOAuthConfig  GithubOAuthConfig
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
