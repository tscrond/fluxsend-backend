package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// BuildViper constructs a standalone Viper instance with a single precedence chain:
// defaults < optional config file < environment variables.
func BuildViper(cfgFile string) (*viper.Viper, error) {
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}

		v.AddConfigPath(home)
		v.SetConfigType("yaml")
		v.SetConfigName(".fluxsend-backend")
	}

	setDefaults(v)
	if err := bindEnvVars(v); err != nil {
		return nil, err
	}

	if err := v.ReadInConfig(); err != nil {
		if cfgFile != "" {
			return nil, fmt.Errorf("read config file %q: %w", cfgFile, err)
		}

		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read default config file: %w", err)
		}
	}

	return v, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("api.listen_port", "3000")
	v.SetDefault("api.mail_from", "noreply@fluxsend.invalid")
	v.SetDefault("api.enable_google_auth", false)
	v.SetDefault("api.enable_github_auth", false)
	v.SetDefault("api.enable_password_auth", false)
	v.SetDefault("mail.provider", "standard")
	v.SetDefault("cli.listen_port", "8091")
	v.SetDefault("cli.route_prefix", "/api")
}

func bindEnvVars(v *viper.Viper) error {
	envMap := map[string]string{
		"api.listen_port":                        "FLUXSEND_LISTEN_PORT",
		"api.google_client_id":                   "GOOGLE_CLIENT_ID",
		"api.google_client_secret":               "GOOGLE_CLIENT_SECRET",
		"api.github_client_id":                   "GITHUB_OAUTH_CLIENT_ID",
		"api.github_client_secret":               "GITHUB_OAUTH_CLIENT_SECRET",
		"api.token_encryption_key":               "TOKEN_ENCRYPTION_KEY",
		"api.frontend_endpoint":                  "FRONTEND_ENDPOINT",
		"api.backend_endpoint":                   "BACKEND_ENDPOINT",
		"api.mail_from":                          "MAIL_FROM",
		"api.enable_google_auth":                 "ENABLE_GOOGLE_AUTH",
		"api.enable_github_auth":                 "ENABLE_GITHUB_AUTH",
		"api.enable_password_auth":               "ENABLE_PASSWORD_AUTH",
		"api.db.host":                            "DB_HOST",
		"api.db.user":                            "POSTGRES_USER",
		"api.db.password":                        "POSTGRES_PASSWORD",
		"api.db.name":                            "POSTGRES_DB",
		"api.storage_provider":                   "STORAGE_PROVIDER",
		"api.aws_region":                         "AWS_REGION",
		"api.aws_access_key_id":                  "AWS_ACCESS_KEY_ID",
		"api.aws_secret_access_key":              "AWS_SECRET_ACCESS_KEY",
		"api.cloudfront.enable_downloads":        "ENABLE_CLOUDFRONT_DOWNLOADS",
		"api.cloudfront.domain":                  "CLOUDFRONT_DOMAIN",
		"api.cloudfront.key_pair_id":             "CLOUDFRONT_KEY_PAIR_ID",
		"api.cloudfront.private_key_path":        "CLOUDFRONT_PRIVATE_KEY_PATH",
		"storage.gcs_bucket_name":                "GCS_BUCKET_NAME",
		"storage.google_application_credentials": "GOOGLE_APPLICATION_CREDENTIALS",
		"storage.google_project_id":              "GOOGLE_PROJECT_ID",
		"storage.s3_bucket_name":                 "S3_BUCKET_NAME",
		"storage.aws_region":                     "AWS_REGION",
		"storage.minio_bucket_name":              "MINIO_BUCKET_NAME",
		"storage.minio_endpoint":                 "MINIO_ENDPOINT",
		"storage.minio_access_key":               "MINIO_ACCESS_KEY",
		"storage.minio_secret_key":               "MINIO_SECRET_KEY",
		"storage.minio_use_ssl":                  "MINIO_USE_SSL",
		"mail.aws_region":                        "AWS_REGION",
		"mail.aws_access_key_id":                 "AWS_ACCESS_KEY_ID",
		"mail.aws_secret_access_key":             "AWS_SECRET_ACCESS_KEY",
		"mail.provider":                          "MAIL_PROVIDER",
		"mail.smtp_host":                         "SMTP_HOST",
		"mail.smtp_port":                         "SMTP_PORT",
		"mail.smtp_username":                     "SMTP_USERNAME",
		"mail.smtp_password":                     "SMTP_PASSWORD",
		"app.env":                                "APP_ENV",
		"cli.listen_port":                        "FLUXSEND_API_LISTEN_PORT",
		"cli.backend_endpoint":                   "BACKEND_ENDPOINT",
		"cli.route_prefix":                       "FLUXSEND_API_ROUTE_PREFIX",
	}

	for key, envVar := range envMap {
		if err := v.BindEnv(key, envVar); err != nil {
			return fmt.Errorf("bind env %s to %s: %w", envVar, key, err)
		}
	}

	v.AutomaticEnv()

	return nil
}
