package mailservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

type SESEmailService struct {
	Client *ses.Client
}

func NewSESEmailService(cfg aws.Config) (*SESEmailService, error) {
	client := ses.NewFromConfig(cfg)

	return &SESEmailService{
		Client: client,
	}, nil
}

func (es *SESEmailService) Send(mailConfig mailtypes.MessageConfig) (any, error) {
	output, err := es.Client.SendEmail(context.TODO(), &ses.SendEmailInput{
		Source: aws.String(mailConfig.From),
		Destination: &types.Destination{
			ToAddresses: mailConfig.To,
		},
		Message: &types.Message{
			Subject: &types.Content{Data: aws.String(mailConfig.Subject)},
			Body: &types.Body{
				Html: &types.Content{Data: aws.String(mailConfig.Body)},
			},
		},
	})

	return output, err
}
