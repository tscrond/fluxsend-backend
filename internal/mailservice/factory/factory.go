package factory

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	mailservice "github.com/tscrond/fluxsend-backend/internal/mailservice/mail"
	"github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

func NewEmailService(provider string) (types.EmailSender, error) {
	switch provider {
	case "ses":
		awsRegion := os.Getenv("AWS_REGION")
		accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(awsRegion), config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyId, secretAccessKey, ""),
		))
		if err != nil {
			return nil, err
		}

		return mailservice.NewSESEmailService(cfg)
	case "standard":
		// config here
		smtpHost := os.Getenv("SMTP_HOST")
		smtpPort := os.Getenv("SMTP_PORT")
		smtpUsername := os.Getenv("SMTP_USERNAME")
		smtpPassword := os.Getenv("SMTP_PASSWORD")

		cfg := &types.StandardSenderConfig{
			SmtpHost:     smtpHost,
			SmtpPort:     smtpPort,
			SmtpUsername: smtpUsername,
			SmtpPassword: smtpPassword,
		}

		return mailservice.NewStandardMailService(cfg)
	case "other":
		return nil, errors.New("not implemented")

	default:
		panic("unknown storage type")
	}
}
