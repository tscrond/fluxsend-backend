package metrics

import (
	"context"
	"io"
	"strings"
	"time"

	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

func (m *Metrics) WrapObjectStorage(provider string, next storagetypes.ObjectStorage) storagetypes.ObjectStorage {
	if !m.Enabled() || next == nil {
		return next
	}

	return &instrumentedObjectStorage{
		provider: normalizeStorageProvider(provider),
		next:     next,
		metrics:  m,
	}
}

func (m *Metrics) WrapEmailSender(provider string, next mailtypes.EmailSender) mailtypes.EmailSender {
	if !m.Enabled() || next == nil {
		return next
	}

	return &instrumentedEmailSender{
		provider: normalizeMailProvider(provider),
		next:     next,
		metrics:  m,
	}
}

type instrumentedObjectStorage struct {
	provider string
	next     storagetypes.ObjectStorage
	metrics  *Metrics
}

func (s *instrumentedObjectStorage) UploadPart(ctx context.Context, bucket string, key string, uploadID string, partNumber int32, body io.Reader, size int64) (*storagetypes.UploadPartResult, error) {
	started := time.Now()
	result, err := s.next.UploadPart(ctx, bucket, key, uploadID, partNumber, body, size)
	s.observe("upload_part", size, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) AbortMultipartUpload(ctx context.Context, bucket string, key string, uploadID string) error {
	started := time.Now()
	err := s.next.AbortMultipartUpload(ctx, bucket, key, uploadID)
	s.observe("abort_multipart_upload", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) CompleteMultipartUpload(ctx context.Context, bucket string, key string, uploadID string, parts []storagetypes.CompletedPart) (*storagetypes.CompleteMultipartUploadResult, error) {
	started := time.Now()
	result, err := s.next.CompleteMultipartUpload(ctx, bucket, key, uploadID, parts)
	s.observe("complete_multipart_upload", 0, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) CreateMultipartUpload(ctx context.Context, bucket, uploadPath, contentType string) (*string, error) {
	started := time.Now()
	result, err := s.next.CreateMultipartUpload(ctx, bucket, uploadPath, contentType)
	s.observe("create_multipart_upload", 0, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*storagetypes.PutObjectResult, error) {
	started := time.Now()
	result, err := s.next.PutObject(ctx, bucket, key, r, size, contentType)
	s.observe("put_object", size, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	started := time.Now()
	result, err := s.next.BucketExists(ctx, fullBucketName)
	s.observe("bucket_exists", 0, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) CreateBucketIfNotExists(ctx context.Context, userId string) error {
	started := time.Now()
	err := s.next.CreateBucketIfNotExists(ctx, userId)
	s.observe("create_bucket_if_not_exists", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) GetUserBucketData(ctx context.Context, id string) (any, error) {
	started := time.Now()
	result, err := s.next.GetUserBucketData(ctx, id)
	s.observe("get_user_bucket_data", 0, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) GetBucketBaseName() string {
	return s.next.GetBucketBaseName()
}

func (s *instrumentedObjectStorage) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error) {
	started := time.Now()
	result, err := s.next.GenerateSignedURL(ctx, bucket, object, expiresAt, contentDisposition)
	s.observe("generate_signed_url", 0, started, err)
	return result, err
}

func (s *instrumentedObjectStorage) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {
	started := time.Now()
	err := s.next.DeleteObjectFromBucket(ctx, object, bucket)
	s.observe("delete_object", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error {
	started := time.Now()
	err := s.next.DeleteObjectsFromBucket(ctx, objects, bucket)
	s.observe("delete_objects", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	started := time.Now()
	err := s.next.MoveObjectInBucket(ctx, source, destination, bucket)
	s.observe("move_object", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) DeleteBucket(ctx context.Context, bucket string) error {
	started := time.Now()
	err := s.next.DeleteBucket(ctx, bucket)
	s.observe("delete_bucket", 0, started, err)
	return err
}

func (s *instrumentedObjectStorage) Close() error {
	return s.next.Close()
}

func (s *instrumentedObjectStorage) observe(operation string, size int64, started time.Time, err error) {
	outcome := outcomeLabel(err)
	s.metrics.storageOperations.WithLabelValues(s.provider, operation, outcome).Inc()
	s.metrics.storageDuration.WithLabelValues(s.provider, operation).Observe(time.Since(started).Seconds())
	if err == nil && size > 0 {
		s.metrics.storageBytesTotal.WithLabelValues(s.provider, operation).Add(float64(size))
	}
}

type instrumentedEmailSender struct {
	provider string
	next     mailtypes.EmailSender
	metrics  *Metrics
}

func (s *instrumentedEmailSender) Send(messageConfig mailtypes.MessageConfig) (any, error) {
	started := time.Now()
	result, err := s.next.Send(messageConfig)
	s.metrics.emailSendTotal.WithLabelValues(s.provider, outcomeLabel(err)).Inc()
	s.metrics.emailSendDuration.WithLabelValues(s.provider).Observe(time.Since(started).Seconds())
	return result, err
}

func normalizeStorageProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "gcs", "s3", "minio":
		return provider
	default:
		return "unknown"
	}
}

func normalizeMailProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "ses":
		return "ses"
	case "standard", "smtp":
		return "smtp"
	default:
		return "unknown"
	}
}

func outcomeLabel(err error) string {
	if err == nil {
		return "success"
	}
	return "error"
}
