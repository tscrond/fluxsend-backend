package types

import (
	"context"
	"io"
	"time"
)

type PutObjectResult struct {
	MD5         string
	Size        int64
	ContentType string
}

type UploadPartResult struct {
	PartNumber int32
	Size       int64

	// Provider-specific data needed for CompleteMultipartUpload
	StorageMetadata map[string]any
}

type CompletedPart struct {
	PartNumber int32
	Size       int64

	// Provider-specific data needed to finalize a multipart upload.
	StorageMetadata map[string]any
}

type CompleteMultipartUploadResult struct {
	ETag string
}

type ObjectStorage interface {
	UploadPart(ctx context.Context, bucket string, key string, uploadID string, partNumber int32, body io.Reader, size int64) (*UploadPartResult, error)
	CompleteMultipartUpload(ctx context.Context, bucket string, key string, uploadID string, parts []CompletedPart) (*CompleteMultipartUploadResult, error)
	CreateMultipartUpload(ctx context.Context, bucket, uploadPath, contentType string) (*string, error)
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*PutObjectResult, error)
	BucketExists(ctx context.Context, fullBucketName string) (bool, error)
	CreateBucketIfNotExists(ctx context.Context, userId string) error
	GetUserBucketData(ctx context.Context, id string) (any, error)
	GetBucketBaseName() string
	GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error)
	DeleteObjectFromBucket(ctx context.Context, object, bucket string) error
	DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error
	MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error
	DeleteBucket(ctx context.Context, bucket string) error
	Close() error
}
