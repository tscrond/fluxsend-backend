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

func TestDeleteBucketRemovesEveryObjectVersion(t *testing.T) {
	var events []string
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("location"):
			io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("uploads"):
			events = append(events, "list-uploads")
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated></ListMultipartUploadsResult>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
			events = append(events, "list-versions")
			io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Version><Key>user1/a</Key><VersionId>v1</VersionId></Version><Version><Key>user1/a</Key><VersionId>v2</VersionId></Version><DeleteMarker><Key>user1/a</Key><VersionId>v3</VersionId></DeleteMarker></ListVersionsResult>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("versionId") != "":
			events = append(events, "delete-version")
			deleted = append(deleted, r.URL.Path+"@"+r.URL.Query().Get("versionId"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	handler := newTestHandler(t, srv.URL, "")
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}

	if got := strings.Join(events, ","); got != "list-uploads,list-versions,delete-version,delete-version,delete-version" {
		t.Errorf("request order = %q", got)
	}
	if got := strings.Join(deleted, ","); got != "/fluxsend/user1/a@v1,/fluxsend/user1/a@v2,/fluxsend/user1/a@v3" {
		t.Errorf("deleted versions = %q, want every version and delete marker", got)
	}
}

func TestDeleteBucketAbortsMultipartUploadsBeforeDeletingVersions(t *testing.T) {
	var events []string
	var abortedUploadID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("location"):
			io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("uploads"):
			events = append(events, "list-uploads")
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated><Upload><Key>user1/large.bin</Key><UploadId>upload-1</UploadId></Upload></ListMultipartUploadsResult>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") != "":
			events = append(events, "abort")
			abortedUploadID = r.URL.Query().Get("uploadId")
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
			events = append(events, "list-versions")
			io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated></ListVersionsResult>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	handler := newTestHandler(t, srv.URL, "")
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}

	if got := strings.Join(events, ","); got != "list-uploads,abort,list-versions" {
		t.Errorf("request order = %q", got)
	}
	if abortedUploadID != "upload-1" {
		t.Errorf("aborted upload ID = %q, want upload-1", abortedUploadID)
	}
}

func TestDeleteBucketReturnsErrorWhenAbortFails(t *testing.T) {
	versionListCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("location"):
			io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("uploads"):
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated><Upload><Key>user1/large.bin</Key><UploadId>upload-1</UploadId></Upload></ListMultipartUploadsResult>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") != "":
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
			versionListCalls++
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	handler := newTestHandler(t, srv.URL, "")
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err == nil {
		t.Fatal("DeleteBucket() error = nil, want error from failed multipart abort")
	}
	if versionListCalls != 0 {
		t.Errorf("version list calls = %d, want 0 after cleanup failure", versionListCalls)
	}
}
