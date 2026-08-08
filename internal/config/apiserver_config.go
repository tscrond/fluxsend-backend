package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type APIServerConfig struct {
	ListenPort         string
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	TokenEncryptionKey string
	FrontendEndpoint   string
	BackendEndpoint    string
	MailFrom           string
	DB                 DBConfig
	EnableGoogleAuth   bool
	EnableGitHubAuth   bool
	EnablePasswordAuth bool
}

func NewAPIServerConfig(v *viper.Viper) (*APIServerConfig, error) {
	googleClientID := v.GetString("api.google_client_id")
	googleClientSecret := v.GetString("api.google_client_secret")
	githubClientID := v.GetString("api.github_client_id")
	githubClientSecret := v.GetString("api.github_client_secret")

	tokenEncryptionKey := v.GetString("api.token_encryption_key")
	frontendEndpoint, err := requiredString(v, "api.frontend_endpoint")
	if err != nil {
		return nil, err
	}
	backendEndpoint, err := requiredString(v, "api.backend_endpoint")
	if err != nil {
		return nil, err
	}
	dbHost, err := requiredString(v, "api.db.host")
	if err != nil {
		return nil, err
	}
	dbUser, err := requiredString(v, "api.db.user")
	if err != nil {
		return nil, err
	}
	dbPassword, err := requiredString(v, "api.db.password")
	if err != nil {
		return nil, err
	}
	dbName, err := requiredString(v, "api.db.name")
	if err != nil {
		return nil, err
	}
	var enableGoogleAuth, enableGitHubAuth, enablePasswordAuth bool
	if enableGoogleAuth = v.GetBool("api.enable_google_auth"); enableGoogleAuth {
		tokenEncryptionKey, err = requiredString(v, "api.token_encryption_key")
		if err != nil {
			return nil, err
		}
		googleClientID, err = requiredString(v, "api.google_client_id")
		if err != nil {
			return nil, err
		}
		googleClientSecret, err = requiredString(v, "api.google_client_secret")
		if err != nil {
			return nil, err
		}
	}
	if enableGitHubAuth = v.GetBool("api.enable_github_auth"); enableGitHubAuth {
		tokenEncryptionKey, err = requiredString(v, "api.token_encryption_key")
		if err != nil {
			return nil, err
		}
		githubClientID, err = requiredString(v, "api.github_client_id")
		if err != nil {
			return nil, err
		}
		githubClientSecret, err = requiredString(v, "api.github_client_secret")
		if err != nil {
			return nil, err
		}
	}
	enablePasswordAuth = v.GetBool("api.enable_password_auth")
	if enablePasswordAuth {
		tokenEncryptionKey, err = requiredString(v, "api.token_encryption_key")
		if err != nil {
			return nil, err
		}
	}

	apiConfig := APIServerConfig{
		ListenPort:         v.GetString("api.listen_port"),
		GoogleClientID:     googleClientID,
		GoogleClientSecret: googleClientSecret,
		GitHubClientID:     githubClientID,
		GitHubClientSecret: githubClientSecret,
		TokenEncryptionKey: tokenEncryptionKey,
		FrontendEndpoint:   frontendEndpoint,
		BackendEndpoint:    backendEndpoint,
		MailFrom:           v.GetString("api.mail_from"),
		DB: DBConfig{
			Host:     dbHost,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
		},
		EnableGoogleAuth:   enableGoogleAuth,
		EnableGitHubAuth:   enableGitHubAuth,
		EnablePasswordAuth: enablePasswordAuth,
	}

	return &apiConfig, nil
}

func (cfg *APIServerConfig) ConnString() string {
	return cfg.DB.ConnString()
}

func requiredString(v *viper.Viper, key string) (string, error) {
	value := v.GetString(key)
	if value == "" {
		return "", fmt.Errorf("missing required config: %s", key)
	}
	return value, nil
}
