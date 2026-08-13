package types

type EmailSender interface {
	Send(messageConfig MessageConfig) (any, error)
}

type MessageConfig struct {
	From    string
	To      []string
	Subject string
	Mime    string
	Body    string
}

type StandardSenderConfig struct {
	SmtpHost     string
	SmtpPort     string
	SmtpUsername string
	SmtpPassword string
}

type MailData struct {
	Files        []FileInfo
	SenderEmail  string
	ExpiryDate   string
	OneTimeCode  string
	Provider     string
	ResetLink    string
	VerifyLink   string
	SupportEmail string
}

type FileInfo struct {
	FileName    string
	DownloadURL string
}
