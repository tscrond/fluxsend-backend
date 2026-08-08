package config

import (
	"strings"

	"github.com/spf13/viper"
)

type BaseRuntimeConfig struct {
	DB               DBConfig
	BackendEndpoint  string
	FrontendEndpoint string
	MailFrom         string
	Storage          StorageConfig
	Mail             MailConfig
	CloudFront       CloudFrontConfig
}

type StorageConfig struct {
	Provider                     string
	GCSBucketName                string
	GoogleApplicationCredentials string
	GoogleProjectID              string
	S3BucketName                 string
	AWSRegion                    string
}

type MailConfig struct {
	Provider           string
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
}

type CloudFrontConfig struct {
	EnableDownloads bool
	Domain          string
	PrivateKeyPath  string
	KeyPairID       string
}

func NewBaseRuntimeConfig(v *viper.Viper) (*BaseRuntimeConfig, error) {
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
	backendEndpoint, err := requiredString(v, "api.backend_endpoint")
	if err != nil {
		return nil, err
	}
	frontendEndpoint, err := requiredString(v, "api.frontend_endpoint")
	if err != nil {
		return nil, err
	}
	mailFrom, err := requiredString(v, "api.mail_from")
	if err != nil {
		return nil, err
	}
	storageProvider := strings.ToLower(strings.TrimSpace(v.GetString("api.storage_provider")))
	if storageProvider == "" {
		if v.GetString("api.aws_region") != "" || v.GetString("api.aws_access_key_id") != "" || v.GetString("api.aws_secret_access_key") != "" {
			storageProvider = "s3"
		} else {
			storageProvider = "gcs"
		}
	}

	baseRuntimeConfig := &BaseRuntimeConfig{
		DB: DBConfig{
			Host:     dbHost,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
		},
		BackendEndpoint:  backendEndpoint,
		FrontendEndpoint: frontendEndpoint,
		MailFrom:         mailFrom,
		Storage: StorageConfig{
			Provider:                     storageProvider,
			GCSBucketName:                v.GetString("storage.gcs_bucket_name"),
			GoogleApplicationCredentials: v.GetString("storage.google_application_credentials"),
			GoogleProjectID:              v.GetString("storage.google_project_id"),
			S3BucketName:                 v.GetString("storage.s3_bucket_name"),
			AWSRegion:                    v.GetString("storage.aws_region"),
		},
		Mail: MailConfig{
			Provider:           strings.ToLower(strings.TrimSpace(v.GetString("mail.provider"))),
			AWSRegion:          v.GetString("mail.aws_region"),
			AWSAccessKeyID:     v.GetString("mail.aws_access_key_id"),
			AWSSecretAccessKey: v.GetString("mail.aws_secret_access_key"),
			SMTPHost:           v.GetString("mail.smtp_host"),
			SMTPPort:           v.GetString("mail.smtp_port"),
			SMTPUsername:       v.GetString("mail.smtp_username"),
			SMTPPassword:       v.GetString("mail.smtp_password"),
		},
		CloudFront: CloudFrontConfig{
			EnableDownloads: v.GetBool("api.cloudfront.enable_downloads"),
			Domain:          v.GetString("api.cloudfront.domain"),
			PrivateKeyPath:  v.GetString("api.cloudfront.private_key_path"),
			KeyPairID:       v.GetString("api.cloudfront.key_pair_id"),
		},
	}

	if baseRuntimeConfig.Mail.Provider == "" {
		baseRuntimeConfig.Mail.Provider = "standard"
	}

	return baseRuntimeConfig, nil
}
