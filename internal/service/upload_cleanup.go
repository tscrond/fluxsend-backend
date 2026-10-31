package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"go.uber.org/zap"
)

const (
	DefaultAbandonedUploadCleanupInterval       = 30 * time.Minute
	DefaultAbandonedUploadMaxAge                = 4 * time.Hour
	DefaultAbandonedUploadBatchSize       int32 = 200
)

type AbandonedUploadCleanupResult struct {
	Cleaned int
	Failed  int
}

type AbandonedUploadCleaner struct {
	log       *zap.SugaredLogger
	queries   sqlc.Querier
	storage   storagetypes.ObjectStorage
	interval  time.Duration
	maxAge    time.Duration
	batchSize int32
}

func NewAbandonedUploadCleaner(
	log *zap.SugaredLogger,
	queries sqlc.Querier,
	storage storagetypes.ObjectStorage,
	interval time.Duration,
	maxAge time.Duration,
	batchSize int32,
) *AbandonedUploadCleaner {
	if interval <= 0 {
		interval = DefaultAbandonedUploadCleanupInterval
	}
	if maxAge <= 0 {
		maxAge = DefaultAbandonedUploadMaxAge
	}
	if batchSize <= 0 {
		batchSize = DefaultAbandonedUploadBatchSize
	}
	return &AbandonedUploadCleaner{
		log:       log,
		queries:   queries,
		storage:   storage,
		interval:  interval,
		maxAge:    maxAge,
		batchSize: batchSize,
	}
}

func (c *AbandonedUploadCleaner) Start(ctx context.Context) {
	go c.run(ctx)
}

func (c *AbandonedUploadCleaner) run(ctx context.Context) {
	c.cleanupAndLog(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanupAndLog(ctx)
		}
	}
}

func (c *AbandonedUploadCleaner) cleanupAndLog(ctx context.Context) {
	result, err := c.CleanupOnce(ctx)
	if err != nil {
		c.log.Errorw("abandoned multipart upload cleanup failed", "error", err, "cleaned", result.Cleaned, "failed", result.Failed)
		return
	}

	if result.Cleaned > 0 || result.Failed > 0 {
		c.log.Infow("abandoned multipart upload cleanup completed", "cleaned", result.Cleaned, "failed", result.Failed)
	}
}

func (c *AbandonedUploadCleaner) CleanupOnce(ctx context.Context) (AbandonedUploadCleanupResult, error) {
	cutoff := time.Now().Add(-c.maxAge)
	staleUploads, err := c.queries.ListStaleFileUploads(ctx, sqlc.ListStaleFileUploadsParams{
		UpdatedAt: cutoff,
		Limit:     c.batchSize,
	})
	if err != nil {
		return AbandonedUploadCleanupResult{}, err
	}

	result := AbandonedUploadCleanupResult{}
	var errs []error
	for _, upload := range staleUploads {
		if err := c.cleanupUpload(ctx, upload); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("upload %s: %w", upload.ID, err))
			continue
		}
		result.Cleaned++
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

func (c *AbandonedUploadCleaner) cleanupUpload(ctx context.Context, upload sqlc.FileUpload) error {
	storageBackend := strings.TrimSpace(strings.ToLower(upload.StorageBackend))
	if storageBackend == "" {
		storageBackend = "s3"
	}

	if storageBackend == "s3" && upload.StorageUploadID.Valid && strings.TrimSpace(upload.StorageUploadID.String) != "" {
		bucket, objectKey, err := c.resolveUploadStorageLocation(ctx, upload)
		if err != nil {
			return err
		}
		if err := c.storage.AbortMultipartUpload(ctx, bucket, objectKey, upload.StorageUploadID.String); err != nil {
			return err
		}
	}

	abortedUpload, err := c.queries.AbortFileUpload(ctx, upload.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	if err := c.queries.DeleteFileUploadPartsByUploadID(ctx, abortedUpload.ID); err != nil {
		return err
	}

	return nil
}

func (c *AbandonedUploadCleaner) resolveUploadStorageLocation(ctx context.Context, upload sqlc.FileUpload) (bucket string, objectKey string, err error) {
	if upload.WorkspaceID.Valid {
		return c.storage.GetBucketBaseName(), workspaceUploadObjectKey(upload.WorkspaceID.UUID, upload.StorageMapping), nil
	}

	bucket, err = resolveUserBucketName(ctx, c.queries, c.storage.GetBucketBaseName(), upload.OwnerID)
	if err != nil {
		return "", "", err
	}
	return bucket, upload.StorageMapping.String(), nil
}
