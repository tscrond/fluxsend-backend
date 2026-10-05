package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tscrond/fluxsend-backend/internal/config"
)

func TestHandleDownloadResponseProxiesWhenConfigured(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", "bytes 0-4/11")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("hello"))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer upstream.Close()

	handlers := &CoreHandlers{proxyDownloads: true}
	req := httptest.NewRequest(http.MethodGet, "/api/d/token", nil)
	req.Header.Set("Range", "bytes=0-4")
	rec := httptest.NewRecorder()

	mechanism, err := handlers.handleDownloadResponse(rec, req, upstream.URL+"/fluxsend/file", "file.txt", "download")
	require.NoError(t, err)
	assert.Equal(t, "proxy", mechanism)
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
	assert.Equal(t, "bytes 0-4/11", rec.Header().Get("Content-Range"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "sandbox", rec.Header().Get("Content-Security-Policy"))
}

func TestHandleDownloadResponseRedirectsWhenNotProxying(t *testing.T) {
	handlers := &CoreHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/d/token", nil)
	rec := httptest.NewRecorder()

	mechanism, err := handlers.handleDownloadResponse(rec, req, "http://minio:9000/fluxsend/file", "file.txt", "download")
	require.NoError(t, err)
	assert.Equal(t, "signed_url", mechanism)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "http://minio:9000/fluxsend/file", rec.Header().Get("Location"))
}

func TestPublicDownloadURLStaysOnAppOrigin(t *testing.T) {
	handlers := &CoreHandlers{backendConfig: config.BackendConfig{BackendEndpoint: "https://app.example.com/"}}

	assert.Equal(t, "https://app.example.com/d/tok123", handlers.publicDownloadURL("tok123", ""))
	assert.Equal(t, "https://app.example.com/d/tok123?mode=inline", handlers.publicDownloadURL("tok123", "inline"))
}
