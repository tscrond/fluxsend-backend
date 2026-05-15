package config

import (
	"os"

	"github.com/tscrond/fluxsend-backend/pkg"
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
}

func NewAPIServerConfig() *APIServerConfig {
	apiConfig := APIServerConfig{
		ListenPort:         os.Getenv("FLUXSEND_LISTEN_PORT"),
		GoogleClientID:     pkg.ReadConfigRequired("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: pkg.ReadConfigRequired("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:     pkg.ReadConfigRequired("GITHUB_OAUTH_CLIENT_ID"),
		GitHubClientSecret: pkg.ReadConfigRequired("GITHUB_OAUTH_CLIENT_SECRET"),
		TokenEncryptionKey: pkg.ReadConfigRequired("TOKEN_ENCRYPTION_KEY"),
		FrontendEndpoint:   pkg.ReadConfigRequired("FRONTEND_ENDPOINT"),
		BackendEndpoint:    pkg.ReadConfigRequired("BACKEND_ENDPOINT"),
		MailFrom:           os.Getenv("MAIL_FROM"),
		DB: DBConfig{
			Host:     pkg.ReadConfigRequired("DB_HOST"),
			User:     pkg.ReadConfigRequired("POSTGRES_USER"),
			Password: pkg.ReadConfigRequired("POSTGRES_PASSWORD"),
			Name:     pkg.ReadConfigRequired("POSTGRES_DB"),
		},
	}
	if apiConfig.ListenPort == "" {
		apiConfig.ListenPort = "3000"
	}
	if apiConfig.MailFrom == "" {
		apiConfig.MailFrom = "noreply@fluxsend.com"
	}

	return &apiConfig
}

func (cfg *APIServerConfig) ConnString() string {
	return cfg.DB.ConnString()
}
