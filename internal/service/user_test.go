package service

import (
	"context"
	"database/sql"
	"errors"
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

// --- GetBucketData --------------------------------------------------------

func TestUserService_GetBucketData_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	const internalID = "internal-abc"
	const base = "fluxsend"
	bucket := base + "-" + internalID

	files := []sqlc.File{
		{
			FileName:    "notes.txt",
			FileType:    sql.NullString{Valid: true, String: "text/plain"},
			Md5Checksum: "csum1",
			Size:        sql.NullInt64{Valid: true, Int64: 256},
			CreatedAt:   time.Now(),
		},
	}
	q.EXPECT().GetFilesByOwner(gomock.Any(), userID).Return(files, nil)
	stor.EXPECT().GetBucketBaseName().Return(base)
	q.EXPECT().GetUserBucketById(gomock.Any(), userID).
		Return(sql.NullString{Valid: true, String: bucket}, nil)

	data, err := svc.GetBucketData(context.Background(), userID, internalID)
	require.NoError(t, err)
	assert.Equal(t, bucket, data.BucketName)
	require.Len(t, data.Objects, 1)
	assert.Equal(t, "notes.txt", data.Objects[0].Name)
}

func TestUserService_GetBucketData_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	dbErr := errors.New("db timeout")
	q.EXPECT().GetFilesByOwner(gomock.Any(), gomock.Any()).Return(nil, dbErr)

	_, err := svc.GetBucketData(context.Background(), uuid.New(), "internal")
	assert.ErrorIs(t, err, dbErr)
}

func TestUserService_GetBucketData_FallbackBucketName(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	const internalID = "fallback-id"

	q.EXPECT().GetFilesByOwner(gomock.Any(), userID).Return(nil, nil)
	// Bucket not in DB (null)
	q.EXPECT().GetUserBucketById(gomock.Any(), userID).
		Return(sql.NullString{Valid: false}, nil)
	stor.EXPECT().GetBucketBaseName().Return("base")

	data, err := svc.GetBucketData(context.Background(), userID, internalID)
	require.NoError(t, err)
	// Falls back to base-internalID
	assert.Equal(t, "base-"+internalID, data.BucketName)
}

// --- GetPrivateDownloadToken ----------------------------------------------

func TestUserService_GetPrivateDownloadToken_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	const fileName = "report.pdf"
	const token = "priv-dl-token-abc"

	q.EXPECT().GetPrivateDownloadTokenByFileName(gomock.Any(),
		sqlc.GetPrivateDownloadTokenByFileNameParams{FileName: fileName, OwnerID: userID}).
		Return(sql.NullString{Valid: true, String: token}, nil)

	tok, err := svc.GetPrivateDownloadToken(context.Background(), userID, fileName)
	require.NoError(t, err)
	assert.Equal(t, token, tok)
}

func TestUserService_GetPrivateDownloadToken_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	q.EXPECT().GetPrivateDownloadTokenByFileName(gomock.Any(), gomock.Any()).
		Return(sql.NullString{}, sql.ErrNoRows)

	_, err := svc.GetPrivateDownloadToken(context.Background(), uuid.New(), "ghost.pdf")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

// --- DeleteAccount --------------------------------------------------------

func TestUserService_DeleteAccount_WithStorageDeletion(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	const internalID = "user-internal-xyz"
	const userName = "testuser"

	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	stor.EXPECT().DeleteBucket(gomock.Any(), "fluxsend-"+internalID).Return(nil)
	q.EXPECT().DeleteAccount(gomock.Any(), userID).
		Return(sqlc.User{ID: userID, UserEmail: "user@example.com"}, nil)

	result, err := svc.DeleteAccount(context.Background(), userID, internalID, userName, true)
	require.NoError(t, err)
	assert.True(t, result.BucketDeleted)
	assert.Equal(t, "user@example.com", result.Email)
	assert.Equal(t, userID.String(), result.AccountID)
}

func TestUserService_DeleteAccount_SkipStorageDeletion(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	// storage.DeleteBucket should NOT be called when deleteStorageData=false
	q.EXPECT().DeleteAccount(gomock.Any(), userID).
		Return(sqlc.User{ID: userID, UserEmail: "user@example.com"}, nil)

	result, err := svc.DeleteAccount(context.Background(), userID, "internal-1", "user", false)
	require.NoError(t, err)
	assert.False(t, result.BucketDeleted)
}

func TestUserService_DeleteAccount_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewUserService(zap.NewNop().Sugar(), q, stor)

	userID := uuid.New()
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	dbErr := errors.New("constraint violation")
	q.EXPECT().DeleteAccount(gomock.Any(), userID).Return(sqlc.User{}, dbErr)

	_, err := svc.DeleteAccount(context.Background(), userID, "internal-1", "user", false)
	assert.Error(t, err)
}
