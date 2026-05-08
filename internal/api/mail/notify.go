package mail

import (
	"fmt"

	templates "github.com/tscrond/fluxsend-backend/internal/mailservice/templates"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"go.uber.org/zap"
)

type Notifier struct {
	log         *zap.SugaredLogger
	emailSender mailtypes.EmailSender
	from        string
}

func NewMailNotifier(log *zap.SugaredLogger, es mailtypes.EmailSender, from string) Notifier {
	if from == "" {
		from = "noreply@fluxsend.com"
	}

	return Notifier{
		log:         log,
		emailSender: es,
		from:        from,
	}
}

func (n *Notifier) SendSharingNotification(sharedByUser, emailTo, expiryDate string, files []mailtypes.FileInfo) error {

	to := []string{emailTo}
	subject := fmt.Sprintf("Subject: New File Transfer from %s", sharedByUser)
	mime := "\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n"

	messageConfig := mailtypes.MessageConfig{
		From:    n.from,
		To:      to,
		Subject: subject,
		Mime:    mime,
	}

	htmlBody, err := templates.RenderMailTemplate("sharing", mailtypes.MailData{
		Files:       files,
		SenderEmail: sharedByUser,
		ExpiryDate:  expiryDate,
	})

	if err != nil {
		n.log.Errorw("failed to render mail template", "error", err)
		return err
	}

	messageConfig.Body = htmlBody

	output, err := n.emailSender.Send(messageConfig)
	if err != nil {
		n.log.Errorw("failed to send email", "error", err)
		return err
	}

	n.log.Infow("mail sent", "output", output)

	return nil
}
