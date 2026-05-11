package factory

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/gcs"
	s3handler "github.com/tscrond/fluxsend-backend/internal/cloud_storage/s3"
	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
)

// TODO: use bucketMode parameter to define if storage backends:
// - give each user one bucket
// OR
// - use a single bucket with per-user prefixing

func NewStorageProvider(log *zap.SugaredLogger, provider string) (types.ObjectStorage, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))

	switch provider {
	case "gcs":
		bucketName := os.Getenv("GCS_BUCKET_NAME")
		svcaccountPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		googleProjectID := os.Getenv("GOOGLE_PROJECT_ID")

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
		bucketName := os.Getenv("S3_BUCKET_NAME")
		if bucketName == "" {
			// Backward-compatible fallback for older env setups.
			bucketName = os.Getenv("GCS_BUCKET_NAME")
		}
		region := os.Getenv("AWS_REGION")

		if bucketName == "" {
			return nil, errors.New("missing S3_BUCKET_NAME for STORAGE_PROVIDER=s3")
		}
		if region == "" {
			return nil, errors.New("missing AWS_REGION for STORAGE_PROVIDER=s3")
		}

		return s3handler.NewS3BucketHandler(log, bucketName, region)
	case "minio":
		return nil, errors.New("not implemented")
	default:
		return nil, fmt.Errorf("unknown storage type %q, expected one of: gcs, s3, minio", provider)
	}
}
