package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

func TestAdminService_DeleteUser_DeletesStorageFirst(t *testing.T) {
	ctrl := gomock.NewController(t)
	stor := mocks.NewMockObjectStorage(ctrl)

	userID := uuid.New()
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := NewAdminService(zap.NewNop().Sugar(), &apiKeyTestRepository{db: db, queries: sqlc.New(db)}, stor)

	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	workspaceID := uuid.New()
	objectID := uuid.New()
	dbMock.ExpectQuery(regexp.QuoteMeta("SELECT workspace_id, id AS object_id\nFROM workspace_files\nWHERE uploaded_by = $1\n  AND file_type IS DISTINCT FROM 'inode/directory'\nUNION\nSELECT workspace_id, storage_mapping AS object_id\nFROM file_uploads\nWHERE owner_id = $1\n  AND workspace_id IS NOT NULL")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "object_id"}).AddRow(workspaceID, objectID))
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	stor.EXPECT().DeleteObjectsFromBucket(
		gomock.Any(),
		[]string{workspaceID.String() + "/" + objectID.String()},
		"fluxsend",
	).Return(nil, nil)
	stor.EXPECT().DeleteBucket(gomock.Any(), "fluxsend-"+userID.String()).Return(nil)
	dbMock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM workspaces WHERE owner_id = $1")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(workspaceID))
	stor.EXPECT().DeletePrefix(gomock.Any(), workspaceID.String()+"/").Return(nil)
	dbMock.ExpectQuery(regexp.QuoteMeta("DELETE FROM users WHERE id = $1")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_bucket", "user_email", "created_at", "plan_id"}).
			AddRow(userID, nil, "user@example.com", time.Now(), nil))

	user, err := svc.DeleteUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, userID, user.ID)
}

func TestAdminService_DeleteUser_StorageErrorKeepsUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	stor := mocks.NewMockObjectStorage(ctrl)

	userID := uuid.New()
	storageErr := errors.New("storage unavailable")
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := NewAdminService(zap.NewNop().Sugar(), &apiKeyTestRepository{db: db, queries: sqlc.New(db)}, stor)

	dbMock.ExpectQuery(regexp.QuoteMeta("SELECT workspace_id, id AS object_id\nFROM workspace_files\nWHERE uploaded_by = $1\n  AND file_type IS DISTINCT FROM 'inode/directory'\nUNION\nSELECT workspace_id, storage_mapping AS object_id\nFROM file_uploads\nWHERE owner_id = $1\n  AND workspace_id IS NOT NULL")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "object_id"}))
	dbMock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM workspaces WHERE owner_id = $1")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	stor.EXPECT().DeleteBucket(gomock.Any(), "fluxsend-"+userID.String()).Return(storageErr)
	// No repository expectation: the user row must survive failed cleanup.

	_, err = svc.DeleteUser(context.Background(), userID)
	assert.ErrorIs(t, err, storageErr)
}
