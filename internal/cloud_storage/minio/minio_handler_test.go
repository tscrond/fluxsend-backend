package minio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestHandler(t *testing.T, internalEndpoint, publicEndpoint string) *MinioBucketHandler {
	t.Helper()
	storage, err := NewMinioBucketHandler(zap.NewNop().Sugar(), "fluxsend", internalEndpoint, publicEndpoint, "key", "secret", "", false)
	if err != nil {
		t.Fatalf("NewMinioBucketHandler() error = %v", err)
	}
	handler, ok := storage.(*MinioBucketHandler)
	if !ok {
		t.Fatalf("unexpected handler type %T", storage)
	}
	return handler
}

// locationServer answers the bucket-location lookup minio-go performs when the
// client has no fixed region (i.e. the internal operations client).
func locationServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signedURLHost(t *testing.T, handler *MinioBucketHandler) *url.URL {
	t.Helper()
	signed, err := handler.GenerateSignedURL(context.Background(), "fluxsend", "file.txt", time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("GenerateSignedURL() error = %v", err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parsing signed URL %q: %v", signed, err)
	}
	return parsed
}

func TestGenerateSignedURLUsesPublicEndpoint(t *testing.T) {
	handler := newTestHandler(t, "http://minio:9000", "https://s3.test.fluxsend.app")

	parsed := signedURLHost(t, handler)
	if parsed.Scheme != "https" {
		t.Errorf("scheme = %q, want https", parsed.Scheme)
	}
	if parsed.Host != "s3.test.fluxsend.app" {
		t.Errorf("host = %q, want s3.test.fluxsend.app", parsed.Host)
	}
	if !strings.HasPrefix(parsed.Path, "/fluxsend/") {
		t.Errorf("path = %q, want /fluxsend/... prefix", parsed.Path)
	}
}

func TestGenerateSignedURLUsesInternalEndpointWithoutPublic(t *testing.T) {
	srv := locationServer(t)
	handler := newTestHandler(t, srv.URL, "")

	parsed := signedURLHost(t, handler)
	if parsed.Host != strings.TrimPrefix(srv.URL, "http://") {
		t.Errorf("host = %q, want %q", parsed.Host, strings.TrimPrefix(srv.URL, "http://"))
	}
}

func TestPublicEndpointRequiresHTTPS(t *testing.T) {
	if _, err := NewMinioBucketHandler(zap.NewNop().Sugar(), "fluxsend", "http://minio:9000", "http://s3.test.fluxsend.app", "key", "secret", "", false); err == nil {
		t.Fatal("expected an error for a non-TLS public endpoint")
	}
}
