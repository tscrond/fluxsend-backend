package factory

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	mailservice "github.com/tscrond/fluxsend-backend/internal/mailservice/mail"
	"github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

type ProviderConfig struct {
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
}

func NewEmailService(provider string, cfg ProviderConfig) (types.EmailSender, error) {
	switch provider {
	case "ses":
		sesCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(cfg.AWSRegion), config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		))
		if err != nil {
			return nil, err
		}

		return mailservice.NewSESEmailService(sesCfg)
	case "standard":
		standardCfg := &types.StandardSenderConfig{
			SmtpHost:     cfg.SMTPHost,
			SmtpPort:     cfg.SMTPPort,
			SmtpUsername: cfg.SMTPUsername,
			SmtpPassword: cfg.SMTPPassword,
		}

		return mailservice.NewStandardMailService(standardCfg)
	case "other":
		return nil, errors.New("not implemented")

	default:
		panic("unknown storage type")
	}
}
