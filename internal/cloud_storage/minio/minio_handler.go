package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/pkg"
	"go.uber.org/zap"
)

type MinioBucketHandler struct {
	log            *zap.SugaredLogger
	Client         *minio.Client
	Core           *minio.Core
	UseSSL         bool
	BaseBucketName string
}

func NewMinioBucketHandler(log *zap.SugaredLogger, bucketName string, endpoint, accessKeyID, secretAccessKey string, useSSL bool) (types.ObjectStorage, error) {
	// Accept both "host:port" and "http(s)://host:port" endpoints; minio-go
	// expects a bare host:port and derives the scheme from the Secure flag.
	if strings.HasPrefix(endpoint, "https://") {
		useSSL = true
		endpoint = strings.TrimPrefix(endpoint, "https://")
	} else if strings.HasPrefix(endpoint, "http://") {
		useSSL = false
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}
	endpoint = strings.TrimSuffix(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("MINIO_ENDPOINT is empty for STORAGE_PROVIDER=minio")
	}

	minioClient, err := minio.NewCore(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Errorf("Failed to initialize MinIO client: %v", err)
		return nil, err
	}
	return &MinioBucketHandler{
		log:            log,
		Client:         minioClient.Client,
		Core:           minioClient,
		UseSSL:         useSSL,
		BaseBucketName: bucketName,
	}, nil
}

// s3Key builds the S3 object key with the user prefix.
func s3Key(userId, fileName string) string {
	return userId + "/" + fileName
}

// extractUserIdFromBucket parses the userId from "<baseName>-<userId>" format.
func (b *MinioBucketHandler) extractUserIdFromBucket(bucket string) string {
	return pkg.ExtractUserIdFromBucketName(b.BaseBucketName, bucket)
}

func (b *MinioBucketHandler) CreateMultipartUpload(ctx context.Context, bucket, uploadPath, contentType string) (*string, error) {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := uploadPath
	if userId != "" {
		objectKey = s3Key(userId, uploadPath)
	}

	out, err := b.Core.NewMultipartUpload(ctx, b.BaseBucketName, objectKey, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrUploadFailed, err)
	}
	return &out, nil
}

func (b *MinioBucketHandler) UploadPart(ctx context.Context, bucket string, key string, uploadID string, partNumber int32, body io.Reader, size int64) (*types.UploadPartResult, error) {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := key
	if userId != "" {
		objectKey = s3Key(userId, key)
	}

	out, err := b.Core.PutObjectPart(ctx, b.BaseBucketName, objectKey, uploadID, int(partNumber), body, size, minio.PutObjectPartOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrUploadFailed, err)

	}
	metadata := map[string]any{}
	if etag := strings.TrimSpace(strings.Trim(out.ETag, "\"")); etag != "" {
		metadata["etag"] = etag
	}

	return &types.UploadPartResult{
		PartNumber:      partNumber,
		Size:            out.Size,
		StorageMetadata: metadata,
	}, nil
}

func (b *MinioBucketHandler) CompleteMultipartUpload(ctx context.Context, bucket string, key string, uploadID string, parts []types.CompletedPart) (*types.CompleteMultipartUploadResult, error) {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := key
	if userId != "" {
		objectKey = s3Key(userId, key)
	}

	completedParts := make([]minio.CompletePart, 0, len(parts))
	for _, part := range parts {
		etagValue, ok := part.StorageMetadata["etag"]
		if !ok {
			return nil, fmt.Errorf("%w: missing etag for part %d", types.ErrTypeConversion, part.PartNumber)
		}

		etag, ok := etagValue.(string)
		if !ok || strings.TrimSpace(etag) == "" {
			return nil, fmt.Errorf("%w: invalid etag for part %d", types.ErrTypeConversion, part.PartNumber)
		}

		completedParts = append(completedParts, minio.CompletePart{
			PartNumber: int(part.PartNumber),
			ETag:       etag,
		})
	}

	out, err := b.Core.CompleteMultipartUpload(ctx, b.BaseBucketName, objectKey, uploadID, completedParts, minio.PutObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrUploadFailed, err)
	}

	return &types.CompleteMultipartUploadResult{
		ETag: strings.TrimSpace(strings.Trim(aws.ToString(&out.ETag), "\"")),
	}, nil
}
func (b *MinioBucketHandler) GetBucketBaseName() string {
	return b.BaseBucketName
}

func (b *MinioBucketHandler) AbortMultipartUpload(ctx context.Context, bucket string, key string, uploadID string) error {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := key
	if userId != "" {
		objectKey = s3Key(userId, key)
	}

	err := b.Core.AbortMultipartUpload(ctx, b.BaseBucketName, objectKey, uploadID)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrUploadFailed, err)
	}
	return nil
}

func (b *MinioBucketHandler) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	exists, err := b.Core.BucketExists(ctx, fullBucketName)
	if err != nil {
		return false, fmt.Errorf("%w: %v", errors.New("ErrBucketCheckFailed"), err)
	}
	return exists, nil
}
func (b *MinioBucketHandler) CreateBucketIfNotExists(ctx context.Context, userId string) error {
	exists, err := b.BucketExists(ctx, b.BaseBucketName)
	if err != nil {
		return err
	}
	if !exists {
		err := b.Core.MakeBucket(ctx, b.BaseBucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("%w: %v", errors.New("ErrBucketCreationFailed"), err)
		}
		b.log.Infow("created bucket", "bucket", b.BaseBucketName)
	}
	return nil
}

func (b *MinioBucketHandler) GetUserBucketData(ctx context.Context, id string) (any, error) {
	prefix := id + "/"
	output := b.Client.ListObjects(ctx, b.BaseBucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var objects []mappings.ObjectMedatata
	for obj := range output {
		key := obj.Key
		if obj.Err != nil {
			b.log.Errorw("error listing objects in bucket", "bucket", b.BaseBucketName, "error", obj.Err)
			return nil, fmt.Errorf("%s: %v", "ErrListObjectsFailed", obj.Err)
		}
		name := strings.TrimPrefix(key, prefix)

		objects = append(objects, mappings.ObjectMedatata{
			Name:        name,
			ContentType: obj.ContentType,
			Created:     obj.LastModified,
			Updated:     obj.LastModified,
			MD5:         strings.TrimSpace(strings.Trim(obj.ETag, "\"")),
			Size:        obj.Size,
			Bucket:      b.BaseBucketName,
		})
	}

	bucketData := &mappings.BucketData{
		BucketName: pkg.GetUserBucketName(b.BaseBucketName, id),
		Objects:    objects,
	}

	return bucketData, nil
}

func (b *MinioBucketHandler) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error) {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	reqParams := url.Values{}
	if contentDisposition != "" {
		reqParams.Set("response-content-disposition", contentDisposition)
	}

	signedURL, err := b.Client.PresignedGetObject(ctx, b.BaseBucketName, objectKey, time.Until(expiresAt), reqParams)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errors.New("ErrGenerateSignedURLFailed"), err)
	}
	return signedURL.String(), nil
}
func (b *MinioBucketHandler) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	if err := b.Client.RemoveObject(ctx, b.BaseBucketName, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove %q failed: %w", objectKey, err)
	}
	b.log.Infow("deleted object from bucket", "bucket", b.BaseBucketName, "object", objectKey)
	return nil
}

func (b *MinioBucketHandler) DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, object := range objects {
		userId := b.extractUserIdFromBucket(bucket)
		objectKey := object
		if userId != "" {
			objectKey = s3Key(userId, object)
		}

		if err := b.Client.RemoveObject(ctx, b.BaseBucketName, objectKey, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("remove %q failed: %w", objectKey, err)
		}
		b.log.Infow("deleted object from bucket", "bucket", b.BaseBucketName, "object", objectKey)
	}
	return nil
}

func (b *MinioBucketHandler) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	userId := b.extractUserIdFromBucket(bucket)
	if userId == "" {
		return fmt.Errorf("cannot extract userId from bucket name: %s", bucket)
	}
	sourceKey := source
	destinationKey := destination
	if userId != "" {
		sourceKey = s3Key(userId, source)
		destinationKey = s3Key(userId, destination)
	}

	_, err := b.Client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: b.BaseBucketName,
		Object: destinationKey,
	}, minio.CopySrcOptions{
		Bucket: b.BaseBucketName,
		Object: sourceKey,
	})
	if err != nil {
		return fmt.Errorf("copy %q -> %q failed: %w", sourceKey, destinationKey, err)
	}

	err = b.Client.RemoveObject(ctx, b.BaseBucketName, sourceKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("remove %q failed: %w", sourceKey, err)
	}

	return nil
}

func (b *MinioBucketHandler) DeleteBucket(ctx context.Context, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	userId := b.extractUserIdFromBucket(bucket)
	if userId == "" {
		return fmt.Errorf("cannot extract userId from bucket name: %s", bucket)
	}

	prefix := userId + "/"
	output := b.Client.ListObjects(ctx, b.BaseBucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var objectKeys []string
	for obj := range output {
		if obj.Err != nil {
			b.log.Errorw("error listing objects in bucket", "bucket", b.BaseBucketName, "error", obj.Err)
			return fmt.Errorf("%s: %v", "ErrListObjectsFailed", obj.Err)
		}
		objectKeys = append(objectKeys, obj.Key)
	}

	for _, objectID := range objectKeys {
		err := b.Client.RemoveObject(ctx, b.BaseBucketName, objectID, minio.RemoveObjectOptions{})
		if err != nil {
			b.log.Errorw("error removing object from bucket", "bucket", b.BaseBucketName, "object", objectID, "error", err)
			return fmt.Errorf("failed_deleting_user_objects")
		}
	}

	b.log.Infow("deleted all objects for user prefix", "prefix", prefix)
	return nil
}

func (b *MinioBucketHandler) Close() error {
	if b.Client != nil {
		b.Client.CredContext().Client.CloseIdleConnections()
	}
	if b.Core != nil {
		b.Core.CredContext().Client.CloseIdleConnections()
	}
	return nil
}

func (b *MinioBucketHandler) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*types.PutObjectResult, error) {
	return nil, fmt.Errorf("%s: not implemented", "NotImplementedError")
}
