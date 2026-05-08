package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct{}

// New builds a SugaredLogger. In production (APP_ENV=production) it outputs
// JSON; otherwise it outputs a human-readable console format with colours.
func New() *zap.SugaredLogger {
	var cfg zap.Config

	if os.Getenv("APP_ENV") == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	base, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		panic("failed to initialise logger: " + err.Error())
	}

	return base.Sugar()
}

// WithContext stores the given logger in the context so that request-scoped
// fields (e.g. request_id) are automatically included in every log line made
// inside that request's call-stack.
func WithContext(ctx context.Context, log *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, contextKey{}, log)
}

// FromContext retrieves the logger stored by WithContext. If none was stored it
// returns a no-op logger so callers never need to nil-check.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	if l, ok := ctx.Value(contextKey{}).(*zap.SugaredLogger); ok && l != nil {
		return l
	}
	return zap.NewNop().Sugar()
}
