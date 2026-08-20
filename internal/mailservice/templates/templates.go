package templates

import (
	"bytes"
	"errors"
	"html/template"
	"log"

	_ "embed"

	types "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

//go:embed share.html
var sharingTemplate string

//go:embed confirmation_code.html
var confirmationCodeTemplate string

//go:embed password_reset_link.html
var passwordResetLinkTemplate string

func RenderMailTemplate(templateType string, emailData types.MailData) (string, error) {
	switch templateType {
	case "sharing":
		return renderTemplate("share", sharingTemplate, emailData)
	case "confirmation_code":
		return renderTemplate("confirmation_code", confirmationCodeTemplate, emailData)
	case "password_reset_link":
		return renderTemplate("password_reset_link", passwordResetLinkTemplate, emailData)

	default:
		return "", errors.New("no available template")
	}
}

func renderTemplate(name, body string, emailData types.MailData) (string, error) {
	tmpl, err := template.New(name).Parse(body)
	if err != nil {
		log.Println(err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, emailData); err != nil {
		return "", err
	}

	return buf.String(), nil
}
