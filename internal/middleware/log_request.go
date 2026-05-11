package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/tscrond/fluxsend-backend/internal/logger"
	"go.uber.org/zap"
)

// wrappedResponseWriter captures the status code written by a handler.
type wrappedResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *wrappedResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// RequestLogger returns a chi-compatible middleware that:
//   - generates (or forwards) an X-Request-ID header,
//   - enriches the request context with a per-request SugaredLogger,
//   - logs method, path, status and latency as structured fields.
func RequestLogger(base *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = newRequestID()
			}
			w.Header().Set("X-Request-ID", reqID)

			reqLog := base.With("request_id", reqID)
			ctx := logger.WithContext(r.Context(), reqLog)

			wrapped := &wrappedResponseWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			reqLog.Infow("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"latency_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
