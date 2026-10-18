package s3

import (
	"context"
	"encoding/xml"
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

func TestDeleteBucketDeletesEveryPage(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	listCalls := 0
	deleteCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path != "/fluxsend" {
			t.Errorf("request path = %q, want /fluxsend", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			listCalls++
			if r.URL.Query().Get("continuation-token") == "" {
				io.WriteString(w, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>true</IsTruncated><NextContinuationToken>page-2</NextContinuationToken><Contents><Key>user1/a</Key></Contents></ListBucketResult>`)
				return
			}
			io.WriteString(w, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Contents><Key>user1/b</Key></Contents></ListBucketResult>`)
			return
		}

		var req struct {
			Objects []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding delete body: %v", err)
		}
		mu.Lock()
		deleteCalls++
		for _, obj := range req.Objects {
			deleted = append(deleted, obj.Key)
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
	if got := strings.Join(deleted, ","); got != "user1/a,user1/b" {
		t.Errorf("deleted objects = %q, want both pages", got)
	}
}

func TestDeleteBucketReturnsErrorWhenDeleteFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodGet {
			io.WriteString(w, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>fluxsend</Name><Prefix>user1/</Prefix><IsTruncated>false</IsTruncated><Contents><Key>user1/a</Key></Contents></ListBucketResult>`)
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
