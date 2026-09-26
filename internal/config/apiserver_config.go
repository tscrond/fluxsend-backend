package config

import (
	"encoding/csv"
	"fmt"
	"strings"

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
	AuthWhitelist      []string
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

	authWhitelist := whitelistValues(v, "api.email_whitelist")
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
		AuthWhitelist:      authWhitelist,
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

func whitelistValues(v *viper.Viper, key string) []string {
	rawValues := []string{}

	switch value := v.Get(key).(type) {
	case []string:
		rawValues = append(rawValues, value...)
	case []any:
		for _, item := range value {
			if s, ok := item.(string); ok {
				rawValues = append(rawValues, s)
			}
		}
	case string:
		raw := strings.TrimSpace(value)
		if raw != "" {
			r := csv.NewReader(strings.NewReader(raw))
			r.TrimLeadingSpace = true
			r.FieldsPerRecord = -1
			if fields, err := r.Read(); err == nil {
				rawValues = append(rawValues, fields...)
			} else {
				rawValues = append(rawValues, strings.Split(raw, ",")...)
			}
		}
	}

	if len(rawValues) == 0 {
		rawValues = append(rawValues, v.GetStringSlice(key)...)
	}

	expandedValues := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		expandedValues = append(expandedValues, parseCommaSeparatedValues(raw)...)
	}

	if len(expandedValues) > 0 {
		rawValues = expandedValues
	}

	values := make([]string, 0, len(rawValues))
	for _, value := range rawValues {
		normalized := strings.TrimSpace(strings.Trim(value, "\"'"))
		if normalized == "" {
			continue
		}
		values = append(values, strings.ToLower(normalized))
	}

	return values
}

func parseCommaSeparatedValues(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	r := csv.NewReader(strings.NewReader(trimmed))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	fields, err := r.Read()
	if err != nil {
		return strings.Split(trimmed, ",")
	}

	return fields
}
