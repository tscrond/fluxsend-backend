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

type ObjectStorage interface {
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
