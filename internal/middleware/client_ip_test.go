package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
)

func TestTrustedProxyClientIP(t *testing.T) {
	tests := []struct {
		name           string
		trustedProxies []string
		remoteAddr     string
		forwardedFor   string
		want           string
	}{
		{
			name:         "direct client ignores forwarded header",
			remoteAddr:   "198.51.100.10:1234",
			forwardedFor: "203.0.113.20",
			want:         "198.51.100.10",
		},
		{
			name:           "trusted proxy supplies client",
			trustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12"},
			remoteAddr:     "10.0.0.5:1234",
			forwardedFor:   "203.0.113.20, 172.18.0.2",
			want:           "203.0.113.20",
		},
		{
			name:           "untrusted peer cannot spoof forwarded header",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "198.51.100.10:1234",
			forwardedFor:   "203.0.113.20",
			want:           "198.51.100.10",
		},
		{
			name:           "trusted proxy without forwarded header falls back to peer",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "10.0.0.5:1234",
			want:           "10.0.0.5",
		},
		{
			name:           "leftmost spoofed entry is ignored",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "10.0.0.5:1234",
			forwardedFor:   "192.0.2.99, 203.0.113.20, 10.0.0.4",
			want:           "203.0.113.20",
		},
		{
			name:           "trusted proxy can be configured as single ip",
			trustedProxies: []string{"10.0.0.5"},
			remoteAddr:     "10.0.0.5:1234",
			forwardedFor:   "203.0.113.20",
			want:           "203.0.113.20",
		},
		{
			name:           "trusted ipv6 peer with zone is accepted",
			trustedProxies: []string{"fe80::/10"},
			remoteAddr:     "[fe80::1%eth0]:1234",
			forwardedFor:   "203.0.113.20",
			want:           "203.0.113.20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			handler := TrustedProxyClientIP(tt.trustedProxies)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = chimiddleware.GetClientIP(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			assert.Equal(t, tt.want, got)
		})
	}
}
