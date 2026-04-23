package mailservice

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"

	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

type StandardEmailService struct {
	config *mailtypes.StandardSenderConfig
}

func NewStandardMailService(cfg *mailtypes.StandardSenderConfig) (*StandardEmailService, error) {
	return &StandardEmailService{
		config: cfg,
	}, nil
}

func (s *StandardEmailService) Send(config mailtypes.MessageConfig) (any, error) {

	fromHeader := fmt.Sprintf("From: FluxSend Notifications <%s>\r\n", config.From)
	toHeader := fmt.Sprintf("To: %s\r\n", strings.Join(config.To, ", "))

	msg := []byte(fromHeader + toHeader + config.Subject + config.Mime + "\r\n" + config.Body)
	auth := smtp.PlainAuth("", s.config.SmtpUsername, s.config.SmtpPassword, s.config.SmtpHost)

	if err := smtp.SendMail(s.config.SmtpHost+":"+s.config.SmtpPort, auth, config.From, config.To, msg); err != nil {
		log.Println(err)
		return nil, err
	}

	log.Printf("email sent to %s", toHeader)

	return nil, nil
}
