package types

import (
	"context"
	"time"

	"github.com/tscrond/dropper/internal/filedata"
)

type ObjectStorage interface {
	SendFileToBucket(ctx context.Context, data *filedata.FileData) error
	BucketExists(ctx context.Context, fullBucketName string) (bool, error)
	CreateBucketIfNotExists(ctx context.Context, userId string) error
	GetUserBucketData(ctx context.Context, id string) (any, error)
	GetUserBucketName(ctx context.Context) (string, error)
	GetBucketBaseName() string
	GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time) (string, error)
	DeleteObjectFromBucket(ctx context.Context, object, bucket string) error
	MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error
	DeleteBucket(ctx context.Context, bucket string) error
	Close() error
}
