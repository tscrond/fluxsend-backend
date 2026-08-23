package factory

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/gcs"
	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/minio"
	s3handler "github.com/tscrond/fluxsend-backend/internal/cloud_storage/s3"
	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
)

type ProviderConfig struct {
	GCSBucketName                string
	GoogleApplicationCredentials string
	GoogleProjectID              string
	S3BucketName                 string
	AWSRegion                    string
	MinioBucketName              string
	MinioEndpoint                string
	MinioAccessKey               string
	MinioSecretKey               string
	MinioUseSSL                  bool
}

// TODO: use bucketMode parameter to define if storage backends:
// - give each user one bucket
// OR
// - use a single bucket with per-user prefixing

func NewStorageProvider(log *zap.SugaredLogger, provider string, cfg ProviderConfig) (types.ObjectStorage, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))

	switch provider {
	case "gcs":
		bucketName := cfg.GCSBucketName
		svcaccountPath := cfg.GoogleApplicationCredentials
		googleProjectID := cfg.GoogleProjectID

		if bucketName == "" {
			return nil, errors.New("missing GCS_BUCKET_NAME for STORAGE_PROVIDER=gcs")
		}
		if svcaccountPath == "" {
			return nil, errors.New("missing GOOGLE_APPLICATION_CREDENTIALS for STORAGE_PROVIDER=gcs")
		}
		if googleProjectID == "" {
			return nil, errors.New("missing GOOGLE_PROJECT_ID for STORAGE_PROVIDER=gcs")
		}

		return gcs.NewGCSBucketHandler(log, svcaccountPath, bucketName, googleProjectID)
	case "s3":
		bucketName := cfg.S3BucketName
		if bucketName == "" {
			// Backward-compatible fallback for older env setups.
			bucketName = cfg.GCSBucketName
		}
		region := cfg.AWSRegion

		if bucketName == "" {
			return nil, errors.New("missing S3_BUCKET_NAME for STORAGE_PROVIDER=s3")
		}
		if region == "" {
			return nil, errors.New("missing AWS_REGION for STORAGE_PROVIDER=s3")
		}

		return s3handler.NewS3BucketHandler(log, bucketName, region)
	case "minio":
		bucketName := cfg.MinioBucketName
		if bucketName == "" {
			// Backward-compatible fallbacks for older env setups.
			bucketName = cfg.S3BucketName
		}
		if bucketName == "" {
			bucketName = cfg.GCSBucketName
		}
		if bucketName == "" {
			return nil, errors.New("missing MINIO_BUCKET_NAME for STORAGE_PROVIDER=minio")
		}
		if cfg.MinioEndpoint == "" {
			return nil, errors.New("missing MINIO_ENDPOINT for STORAGE_PROVIDER=minio")
		}
		if cfg.MinioAccessKey == "" {
			return nil, errors.New("missing MINIO_ACCESS_KEY for STORAGE_PROVIDER=minio")
		}
		if cfg.MinioSecretKey == "" {
			return nil, errors.New("missing MINIO_SECRET_KEY for STORAGE_PROVIDER=minio")
		}

		return minio.NewMinioBucketHandler(log, bucketName, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioUseSSL)
	default:
		return nil, fmt.Errorf("unknown storage type %q, expected one of: gcs, s3, minio", provider)
	}
}
