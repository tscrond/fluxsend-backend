package s3

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

func newTestS3Handler(t *testing.T, endpoint string) *S3BucketHandler {
	t.Helper()
	client := awss3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}, func(o *awss3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &S3BucketHandler{
		log:            zap.NewNop().Sugar(),
		Client:         client,
		BaseBucketName: "fluxsend",
	}
}

func TestDeleteBucketDeletesEveryVersionPage(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	listCalls := 0
	deleteCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path != "/fluxsend" {
			t.Errorf("request path = %q, want /fluxsend", r.URL.Path)
		}
		if r.Method == http.MethodGet && r.URL.Query().Has("uploads") {
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated></ListMultipartUploadsResult>`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Query().Has("versions") {
			listCalls++
			if r.URL.Query().Get("version-id-marker") == "" {
				io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>true</IsTruncated><NextKeyMarker>user1/a</NextKeyMarker><NextVersionIdMarker>v1</NextVersionIdMarker><Version><Key>user1/a</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`)
				return
			}
			io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Version><Key>user1/a</Key><VersionId>v2</VersionId></Version><DeleteMarker><Key>user1/a</Key><VersionId>v3</VersionId></DeleteMarker></ListVersionsResult>`)
			return
		}

		var req struct {
			Objects []struct {
				Key       string `xml:"Key"`
				VersionId string `xml:"VersionId"`
			} `xml:"Object"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding delete body: %v", err)
		}
		mu.Lock()
		deleteCalls++
		for _, obj := range req.Objects {
			deleted = append(deleted, obj.Key+"@"+obj.VersionId)
		}
		mu.Unlock()
		io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></DeleteResult>`)
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}

	if listCalls != 2 {
		t.Errorf("list calls = %d, want 2", listCalls)
	}
	if deleteCalls != 2 {
		t.Errorf("delete calls = %d, want one batch per page", deleteCalls)
	}
	if got := strings.Join(deleted, ","); got != "user1/a@v1,user1/a@v2,user1/a@v3" {
		t.Errorf("deleted versions = %q, want every version and delete marker", got)
	}
}

func TestDeleteBucketAbortsMultipartUploadsBeforeDeletingVersions(t *testing.T) {
	var events []string
	var abortedUploadID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodGet && r.URL.Query().Get("prefix") != "user1/" {
			t.Errorf("prefix = %q, want user1/", r.URL.Query().Get("prefix"))
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("uploads"):
			events = append(events, "list-uploads")
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated><Upload><Key>user1/large.bin</Key><UploadId>upload-1</UploadId></Upload></ListMultipartUploadsResult>`)
		case r.Method == http.MethodDelete:
			events = append(events, "abort")
			abortedUploadID = r.URL.Query().Get("uploadId")
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
			events = append(events, "list-versions")
			io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Version><Key>user1/large.bin</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`)
		case r.Method == http.MethodPost && r.URL.Query().Has("delete"):
			events = append(events, "delete-versions")
			io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></DeleteResult>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}

	if got := strings.Join(events, ","); got != "list-uploads,abort,list-versions,delete-versions" {
		t.Errorf("request order = %q", got)
	}
	if abortedUploadID != "upload-1" {
		t.Errorf("aborted upload ID = %q, want upload-1", abortedUploadID)
	}
}

func TestDeleteBucketReturnsErrorWhenDeleteFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodGet && r.URL.Query().Has("uploads") {
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated></ListMultipartUploadsResult>`)
			return
		}
		if r.Method == http.MethodGet {
			io.WriteString(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Version><Key>user1/a</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`)
			return
		}
		io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Error><Key>user1/a</Key><Code>AccessDenied</Code></Error></DeleteResult>`)
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err == nil {
		t.Fatal("DeleteBucket() error = nil, want error from failed object deletion")
	}
}

func TestDeleteBucketReturnsErrorWhenAbortFails(t *testing.T) {
	versionListCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("uploads"):
			io.WriteString(w, `<ListMultipartUploadsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>fluxsend</Bucket><IsTruncated>false</IsTruncated><Upload><Key>user1/large.bin</Key><UploadId>upload-1</UploadId></Upload></ListMultipartUploadsResult>`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
		case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
			versionListCalls++
		}
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	if err := handler.DeleteBucket(context.Background(), "fluxsend-user1"); err == nil {
		t.Fatal("DeleteBucket() error = nil, want error from failed multipart abort")
	}
	if versionListCalls != 0 {
		t.Errorf("version list calls = %d, want 0 after cleanup failure", versionListCalls)
	}
}

func TestDeleteObjectsFromBucketReportsFailedKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path != "/fluxsend" {
			t.Errorf("request path = %q, want /fluxsend", r.URL.Path)
		}
		io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Error><Key>user1/bad</Key><Code>AccessDenied</Code></Error></DeleteResult>`)
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	failed, err := handler.DeleteObjectsFromBucket(context.Background(), []string{"ok", "bad"}, "fluxsend-user1")
	if err == nil {
		t.Fatal("DeleteObjectsFromBucket() error = nil, want error")
	}
	if got := strings.Join(failed, ","); got != "bad" {
		t.Errorf("failed keys = %q, want bad", got)
	}
}

func TestDeleteObjectsFromBucketReturnsUnprocessedBatchesOnRequestFailure(t *testing.T) {
	deleteCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleteCalls++
		w.Header().Set("Content-Type", "application/xml")

		var req struct {
			Objects []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding delete body: %v", err)
		}
		if len(req.Objects) > 0 && req.Objects[0].Key == "user1/key1000" {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
			return
		}
		io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></DeleteResult>`)
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	keys := make([]string, 2003)
	for i := range keys {
		keys[i] = fmt.Sprintf("key%d", i)
	}

	failed, err := handler.DeleteObjectsFromBucket(context.Background(), keys, "fluxsend-user1")
	if err == nil {
		t.Fatal("DeleteObjectsFromBucket() error = nil, want error")
	}
	if deleteCalls != 2 {
		t.Errorf("delete calls = %d, want 2", deleteCalls)
	}
	if got := strings.Join(failed, ","); got != strings.Join(keys[1000:], ",") {
		t.Errorf("failed keys = %v, want failed and unattempted batches %v", failed, keys[1000:])
	}
}

func TestDeleteObjectsFromBucketBatchesInThousands(t *testing.T) {
	deleteCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleteCalls++
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></DeleteResult>`)
	}))
	t.Cleanup(srv.Close)

	handler := newTestS3Handler(t, srv.URL)
	keys := make([]string, 1001)
	for i := range keys {
		keys[i] = "key"
	}
	failed, err := handler.DeleteObjectsFromBucket(context.Background(), keys, "fluxsend-user1")
	if err != nil {
		t.Fatalf("DeleteObjectsFromBucket() error = %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("failed keys = %v, want none", failed)
	}
	if deleteCalls != 2 {
		t.Errorf("delete calls = %d, want 2", deleteCalls)
	}
}
