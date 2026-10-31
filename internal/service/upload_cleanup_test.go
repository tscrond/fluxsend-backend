package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

func TestAbandonedUploadCleaner_CleanupOnce_PrivateUpload(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	storage := mocks.NewMockObjectStorage(ctrl)

	uploadID := uuid.New()
	ownerID := uuid.New()
	storageMapping := uuid.New()
	uploadRow := sqlc.FileUpload{
		ID:              uploadID,
		OwnerID:         ownerID,
		StorageBackend:  "s3",
		StorageUploadID: sql.NullString{Valid: true, String: "upload-123"},
		StorageMapping:  storageMapping,
		Status:          "uploading",
	}

	q.EXPECT().
		ListStaleFileUploads(gomock.Any(), gomock.Any()).
		Return([]sqlc.FileUpload{uploadRow}, nil)
	storage.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).Return(sql.NullString{Valid: true, String: "fluxsend-user"}, nil)
	storage.EXPECT().AbortMultipartUpload(gomock.Any(), "fluxsend-user", storageMapping.String(), "upload-123").Return(nil)
	q.EXPECT().AbortFileUpload(gomock.Any(), uploadID).Return(uploadRow, nil)
	q.EXPECT().DeleteFileUploadPartsByUploadID(gomock.Any(), uploadID).Return(nil)

	cleaner := NewAbandonedUploadCleaner(zap.NewNop().Sugar(), q, storage, time.Minute, 4*time.Hour, 50)
	result, err := cleaner.CleanupOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Cleaned)
	assert.Equal(t, 0, result.Failed)
}

func TestAbandonedUploadCleaner_CleanupOnce_WorkspaceUpload(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	storage := mocks.NewMockObjectStorage(ctrl)

	uploadID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	storageMapping := uuid.New()
	uploadRow := sqlc.FileUpload{
		ID:              uploadID,
		OwnerID:         ownerID,
		WorkspaceID:     uuid.NullUUID{UUID: workspaceID, Valid: true},
		StorageBackend:  "s3",
		StorageUploadID: sql.NullString{Valid: true, String: "upload-456"},
		StorageMapping:  storageMapping,
		Status:          "uploading",
	}

	q.EXPECT().
		ListStaleFileUploads(gomock.Any(), gomock.Any()).
		Return([]sqlc.FileUpload{uploadRow}, nil)
	storage.EXPECT().GetBucketBaseName().Return("fluxsend")
	storage.EXPECT().AbortMultipartUpload(gomock.Any(), "fluxsend", workspaceUploadObjectKey(workspaceID, storageMapping), "upload-456").Return(nil)
	q.EXPECT().AbortFileUpload(gomock.Any(), uploadID).Return(uploadRow, nil)
	q.EXPECT().DeleteFileUploadPartsByUploadID(gomock.Any(), uploadID).Return(nil)

	cleaner := NewAbandonedUploadCleaner(zap.NewNop().Sugar(), q, storage, time.Minute, 4*time.Hour, 50)
	result, err := cleaner.CleanupOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Cleaned)
	assert.Equal(t, 0, result.Failed)
}

func TestAbandonedUploadCleaner_CleanupOnce_StorageAbortFailureLeavesUploadForRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	storage := mocks.NewMockObjectStorage(ctrl)

	uploadID := uuid.New()
	ownerID := uuid.New()
	storageMapping := uuid.New()
	uploadRow := sqlc.FileUpload{
		ID:              uploadID,
		OwnerID:         ownerID,
		StorageBackend:  "s3",
		StorageUploadID: sql.NullString{Valid: true, String: "upload-789"},
		StorageMapping:  storageMapping,
		Status:          "uploading",
	}

	q.EXPECT().
		ListStaleFileUploads(gomock.Any(), gomock.Any()).
		Return([]sqlc.FileUpload{uploadRow}, nil)
	storage.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).Return(sql.NullString{Valid: true, String: "fluxsend-user"}, nil)
	storage.EXPECT().AbortMultipartUpload(gomock.Any(), "fluxsend-user", storageMapping.String(), "upload-789").Return(assert.AnError)

	cleaner := NewAbandonedUploadCleaner(zap.NewNop().Sugar(), q, storage, time.Minute, 4*time.Hour, 50)
	result, err := cleaner.CleanupOnce(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0, result.Cleaned)
	assert.Equal(t, 1, result.Failed)
}
