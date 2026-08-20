package notify

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
		from = "noreply@fluxsend.invalid"
	}

	return Notifier{
		log:         log,
		emailSender: es,
		from:        from,
	}
}

func (n *Notifier) SendSharingNotification(sharedByUser, emailTo, expiryDate string, files []mailtypes.FileInfo) error {
	htmlBody, err := templates.RenderMailTemplate("sharing", mailtypes.MailData{
		Files:       files,
		SenderEmail: sharedByUser,
		ExpiryDate:  expiryDate,
	})
	if err != nil {
		n.log.Errorw("failed to render mail template", "error", err)
		return err
	}

	return n.sendTemplatedMail([]string{emailTo}, fmt.Sprintf("New File Transfer from %s", sharedByUser), htmlBody)
}

func (n *Notifier) SendConfirmationCode(emailTo, code, verifyLink string) error {
	htmlBody, err := templates.RenderMailTemplate("confirmation_code", mailtypes.MailData{
		OneTimeCode: code,
		VerifyLink:  verifyLink,
	})
	if err != nil {
		n.log.Errorw("failed to render confirmation code template", "error", err)
		return err
	}

	return n.sendTemplatedMail([]string{emailTo}, "Confirm your FluxSend email", htmlBody)
}

func (n *Notifier) SendPasswordResetLink(emailTo, resetLink string) error {
	htmlBody, err := templates.RenderMailTemplate("password_reset_link", mailtypes.MailData{
		ResetLink: resetLink,
	})
	if err != nil {
		n.log.Errorw("failed to render password reset template", "error", err)
		return err
	}

	return n.sendTemplatedMail([]string{emailTo}, "Reset your FluxSend password", htmlBody)
}

func (n *Notifier) sendTemplatedMail(to []string, subjectText, htmlBody string) error {
	mime := "\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n"

	output, err := n.emailSender.Send(mailtypes.MessageConfig{
		From:    n.from,
		To:      to,
		Subject: fmt.Sprintf("Subject: %s", subjectText),
		Mime:    mime,
		Body:    htmlBody,
	})
	if err != nil {
		n.log.Errorw("failed to send email", "error", err)
		return err
	}

	n.log.Infow("mail sent", "output", output, "to", to, "subject", subjectText)
	return nil
}
