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

func RenderMailTemplate(templateType string, emailData types.MailData) (string, error) {
	switch templateType {
	case "sharing":
		tmpl, err := template.New("share").Parse(sharingTemplate)
		if err != nil {
			log.Println(err)
			return "", err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, emailData); err != nil {
			return "", err
		}

		templatedBody := buf.String()
		return templatedBody, nil

	default:
		return "", errors.New("no available template")
	}
}
