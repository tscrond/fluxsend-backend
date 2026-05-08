package s3

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/pkg"
)

type S3BucketHandler struct {
	log            *zap.SugaredLogger
	Client         *s3.Client
	PresignClient  *s3.PresignClient
	BaseBucketName string
	Region         string
}

func NewS3BucketHandler(log *zap.SugaredLogger, bucketName, region string) (types.ObjectStorage, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		log.Errorw("error loading AWS config", "error", err)
		return nil, err
	}

	client := s3.NewFromConfig(cfg)
	presignClient := s3.NewPresignClient(client)

	return &S3BucketHandler{
		log:            log,
		Client:         client,
		PresignClient:  presignClient,
		BaseBucketName: bucketName,
		Region:         region,
	}, nil
}

// s3Key builds the S3 object key with the user prefix.
func s3Key(userId, fileName string) string {
	return userId + "/" + fileName
}

// extractUserIdFromBucket parses the userId from "<baseName>-<userId>" format.
func (b *S3BucketHandler) extractUserIdFromBucket(bucket string) string {
	return pkg.ExtractUserIdFromBucketName(b.BaseBucketName, bucket)
}

func (b *S3BucketHandler) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*types.PutObjectResult, error) {
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := key
	if userId != "" {
		objectKey = s3Key(userId, key)
	}

	hasher := md5.New()
	teeReader := io.TeeReader(r, hasher)

	_, err := b.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.BaseBucketName),
		Key:           aws.String(objectKey),
		Body:          teeReader,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		b.log.Errorw("error uploading file", "error", err)
		return nil, fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	md5Hash := hex.EncodeToString(hasher.Sum(nil))

	return &types.PutObjectResult{
		MD5:         md5Hash,
		Size:        size,
		ContentType: contentType,
	}, nil
}

func (b *S3BucketHandler) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	_, err := b.Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(b.BaseBucketName),
	})
	if err != nil {
		var notFound *s3types.NotFound
		if errors.As(err, &notFound) {
			b.log.Infow("bucket does not exist", "bucket", b.BaseBucketName)
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (b *S3BucketHandler) CreateBucketIfNotExists(ctx context.Context, userId string) error {
	exists, err := b.BucketExists(ctx, b.BaseBucketName)
	if err != nil {
		b.log.Errorw("error checking for bucket", "error", err)
		return err
	}
	if !exists {
		_, err := b.Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(b.BaseBucketName),
			CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(b.Region),
			},
		})
		if err != nil {
			b.log.Errorw("error creating storage bucket", "error", err)
			return err
		}
		b.log.Infow("bucket created", "bucket", b.BaseBucketName)
	}
	return nil
}

func (b *S3BucketHandler) GetUserBucketData(ctx context.Context, id string) (any, error) {
	prefix := id + "/"

	output, err := b.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.BaseBucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		b.log.Errorw("error listing objects", "bucket", b.BaseBucketName, "error", err)
		return nil, err
	}

	var objects []mappings.ObjectMedatata
	for _, obj := range output.Contents {
		key := aws.ToString(obj.Key)
		// strip the userId prefix for the display name
		name := strings.TrimPrefix(key, prefix)
		if name == "" {
			continue
		}

		// get detailed attrs per object
		head, err := b.Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(b.BaseBucketName),
			Key:    obj.Key,
		})

		contentType := ""
		if err == nil && head.ContentType != nil {
			contentType = *head.ContentType
		}

		etag := ""
		if obj.ETag != nil {
			etag = strings.Trim(*obj.ETag, "\"")
		}

		objects = append(objects, mappings.ObjectMedatata{
			Name:        name,
			ContentType: contentType,
			Created:     safeTime(obj.LastModified),
			Updated:     safeTime(obj.LastModified),
			MD5:         etag,
			Size:        derefInt64(obj.Size),
			Bucket:      b.BaseBucketName,
		})
	}

	bucketData := &mappings.BucketData{
		BucketName:   pkg.GetUserBucketName(b.BaseBucketName, id),
		StorageClass: "STANDARD",
		TimeCreated:  time.Time{},
		Labels:       nil,
		Objects:      objects,
	}

	return bucketData, nil
}

func (b *S3BucketHandler) getUserBucketName(userInternalID string) string {
	return pkg.GetUserBucketName(b.BaseBucketName, userInternalID)
}

func (b *S3BucketHandler) GetBucketBaseName() string {
	return b.BaseBucketName
}

func (b *S3BucketHandler) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error) {
	// bucket may be "<base>-<userId>" from DB; extract userId to build the real S3 key
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(objectKey),
	}
	if contentDisposition != "" {
		input.ResponseContentDisposition = aws.String(contentDisposition)
	}

	presignResult, err := b.PresignClient.PresignGetObject(ctx, input, s3.WithPresignExpires(time.Until(expiresAt)))
	if err != nil {
		return "", fmt.Errorf("Bucket(%q).PresignGetObject: %w", b.BaseBucketName, err)
	}

	return presignResult.URL, nil
}

func (b *S3BucketHandler) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// bucket is "<base>-<userId>"; extract userId to build the S3 key
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	_, err := b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("Object(%q).Delete: %w", objectKey, err)
	}

	b.log.Infow("object deleted", "bucket", b.BaseBucketName, "key", objectKey)
	return nil
}

func (b *S3BucketHandler) DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, object := range objects {
		// bucket is "<base>-<userId>"; extract userId to build the S3 key
		userId := b.extractUserIdFromBucket(bucket)
		objectKey := object
		if userId != "" {
			objectKey = s3Key(userId, object)
		}

		_, err := b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(b.BaseBucketName),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			return fmt.Errorf("Object(%q).Delete: %w", objectKey, err)
		}

		b.log.Infow("object deleted", "bucket", b.BaseBucketName, "key", objectKey)
	}

	return nil
}

func (b *S3BucketHandler) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	userId := b.extractUserIdFromBucket(bucket)
	sourceKey := source
	destinationKey := destination
	if userId != "" {
		sourceKey = s3Key(userId, source)
		destinationKey = s3Key(userId, destination)
	}

	copySource := b.BaseBucketName + "/" + sourceKey
	_, err := b.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(b.BaseBucketName),
		CopySource: aws.String(copySource),
		Key:        aws.String(destinationKey),
	})
	if err != nil {
		return fmt.Errorf("copy %q -> %q failed: %w", sourceKey, destinationKey, err)
	}

	_, err = b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(sourceKey),
	})
	if err != nil {
		return fmt.Errorf("delete source %q after copy failed: %w", sourceKey, err)
	}

	return nil
}

func (b *S3BucketHandler) DeleteBucket(ctx context.Context, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// bucket is "<base>-<userId>"; delete all objects under the user's prefix
	userId := b.extractUserIdFromBucket(bucket)
	if userId == "" {
		return fmt.Errorf("cannot extract userId from bucket name: %s", bucket)
	}

	prefix := userId + "/"
	output, err := b.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.BaseBucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		b.log.Errorw("failed fetching bucket info", "error", err)
		return err
	}

	if len(output.Contents) > 0 {
		var objectIds []s3types.ObjectIdentifier
		for _, obj := range output.Contents {
			objectIds = append(objectIds, s3types.ObjectIdentifier{
				Key: obj.Key,
			})
			b.log.Infow("deleting object", "key", aws.ToString(obj.Key))
		}

		_, err = b.Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.BaseBucketName),
			Delete: &s3types.Delete{Objects: objectIds},
		})
		if err != nil {
			b.log.Errorw("error batch deleting objects", "error", err)
			return fmt.Errorf("failed_deleting_user_objects")
		}
	}

	b.log.Infow("deleted all objects for user prefix", "prefix", prefix)
	return nil
}

func (b *S3BucketHandler) Close() error {
	return nil
}

func safeTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

func derefInt64(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}
