package mail

import (
	mailnotify "github.com/tscrond/fluxsend-backend/internal/mailservice/notify"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"go.uber.org/zap"
)

type Notifier = mailnotify.Notifier

func NewMailNotifier(log *zap.SugaredLogger, es mailtypes.EmailSender, from string) Notifier {
	return mailnotify.NewMailNotifier(log, es, from)
}
