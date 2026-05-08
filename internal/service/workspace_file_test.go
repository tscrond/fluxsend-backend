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

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

// --- GetWorkspaceFilesTree ------------------------------------------------

func TestWorkspaceFileService_GetWorkspaceFilesTree_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	uploader := uuid.New()
	now := time.Now()

	allFiles := []sqlc.WorkspaceFile{
		{
			ID:          fileID,
			WorkspaceID: wsID,
			UploadedBy:  uploader,
			FileName:    "report.pdf",
			FileType:    sql.NullString{Valid: true, String: "application/pdf"},
			Size:        1024,
			Path:        "/",
			CreatedAt:   now,
		},
	}

	q.EXPECT().GetWorkspaceFiles(gomock.Any(), wsID).Return(allFiles, nil)
	q.EXPECT().GetWorkspaceFoldersAtPathWithCreators(gomock.Any(),
		sqlc.GetWorkspaceFoldersAtPathWithCreatorsParams{WorkspaceID: wsID, Path: "/"}).
		Return([]sqlc.GetWorkspaceFoldersAtPathWithCreatorsRow{}, nil)
	q.EXPECT().GetWorkspaceFilesAtPathWithUploaders(gomock.Any(),
		sqlc.GetWorkspaceFilesAtPathWithUploadersParams{WorkspaceID: wsID, Path: "/"}).
		Return([]sqlc.GetWorkspaceFilesAtPathWithUploadersRow{
			{
				ID:            fileID,
				FileName:      "report.pdf",
				FileType:      sql.NullString{Valid: true, String: "application/pdf"},
				Size:          1024,
				Md5Checksum:   sql.NullString{Valid: true, String: "abc123"},
				UploadedBy:    uploader,
				UploaderEmail: "alice@example.com",
				CreatedAt:     now,
				Path:          "/",
			},
		}, nil)

	tree, err := svc.GetWorkspaceFilesTree(context.Background(), wsID, "/")
	require.NoError(t, err)
	assert.Equal(t, "/", tree.Path)
	require.Len(t, tree.Files, 1)
	assert.Equal(t, "report.pdf", tree.Files[0].Name)
	assert.Equal(t, "alice@example.com", tree.Files[0].UploadedByEmail)
	assert.Empty(t, tree.Folders)
}

func TestWorkspaceFileService_GetWorkspaceFilesTree_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	dbErr := errors.New("connection refused")
	q.EXPECT().GetWorkspaceFiles(gomock.Any(), gomock.Any()).Return(nil, dbErr)

	_, err := svc.GetWorkspaceFilesTree(context.Background(), uuid.New(), "/")
	assert.ErrorIs(t, err, dbErr)
}

func TestWorkspaceFileService_GetWorkspaceFilesTree_VirtualFolder(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	uploader := uuid.New()
	now := time.Now()

	// File lives in /docs subfolder
	allFiles := []sqlc.WorkspaceFile{
		{
			ID:          fileID,
			WorkspaceID: wsID,
			UploadedBy:  uploader,
			FileName:    "notes.txt",
			FileType:    sql.NullString{Valid: true, String: "text/plain"},
			Size:        512,
			Path:        "/docs",
			CreatedAt:   now,
		},
	}

	q.EXPECT().GetWorkspaceFiles(gomock.Any(), wsID).Return(allFiles, nil)
	q.EXPECT().GetWorkspaceFoldersAtPathWithCreators(gomock.Any(),
		sqlc.GetWorkspaceFoldersAtPathWithCreatorsParams{WorkspaceID: wsID, Path: "/"}).
		Return([]sqlc.GetWorkspaceFoldersAtPathWithCreatorsRow{}, nil)
	q.EXPECT().GetWorkspaceFilesAtPathWithUploaders(gomock.Any(),
		sqlc.GetWorkspaceFilesAtPathWithUploadersParams{WorkspaceID: wsID, Path: "/"}).
		Return([]sqlc.GetWorkspaceFilesAtPathWithUploadersRow{}, nil)

	tree, err := svc.GetWorkspaceFilesTree(context.Background(), wsID, "/")
	require.NoError(t, err)
	assert.Empty(t, tree.Files)
	// "docs" virtual folder should appear
	require.Len(t, tree.Folders, 1)
	assert.Equal(t, "docs", tree.Folders[0].Name)
}

// --- CreateWorkspaceFolder ------------------------------------------------

func TestWorkspaceFileService_CreateWorkspaceFolder_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	creatorID := uuid.New()
	folderID := uuid.New()
	now := time.Now()

	q.EXPECT().CreateWorkspaceFolder(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p sqlc.CreateWorkspaceFolderParams) (sqlc.WorkspaceFile, error) {
			assert.Equal(t, wsID, p.WorkspaceID)
			assert.Equal(t, creatorID, p.UploadedBy)
			assert.Equal(t, "docs", p.FileName)
			assert.Equal(t, "/", p.Path)
			return sqlc.WorkspaceFile{
				ID:          folderID,
				WorkspaceID: wsID,
				UploadedBy:  creatorID,
				FileName:    "docs",
				FileType:    sql.NullString{Valid: true, String: "inode/directory"},
				Path:        "/",
				CreatedAt:   now,
			}, nil
		})

	result, err := svc.CreateWorkspaceFolder(context.Background(), wsID, creatorID, "docs", "/")
	require.NoError(t, err)
	assert.Equal(t, "docs", result.Name)
	assert.Equal(t, "/", result.Path)
	assert.Equal(t, wsID, result.WorkspaceId)
}

func TestWorkspaceFileService_CreateWorkspaceFolder_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	dbErr := errors.New("constraint violation")
	q.EXPECT().CreateWorkspaceFolder(gomock.Any(), gomock.Any()).Return(sqlc.WorkspaceFile{}, dbErr)

	_, err := svc.CreateWorkspaceFolder(context.Background(), uuid.New(), uuid.New(), "folder", "/")
	assert.ErrorIs(t, err, dbErr)
}

// --- RemoveWorkspaceFile --------------------------------------------------

func TestWorkspaceFileService_RemoveWorkspaceFile_Owner(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	ownerID := uuid.New()
	const bucket = "workspace-bucket"

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), sqlc.GetWorkspaceFileByIdParams{
		ID: fileID, WorkspaceID: wsID,
	}).Return(sqlc.WorkspaceFile{ID: fileID, WorkspaceID: wsID, UploadedBy: ownerID}, nil)
	stor.EXPECT().GetBucketBaseName().Return(bucket)
	stor.EXPECT().DeleteObjectFromBucket(gomock.Any(), wsID.String()+"/"+fileID.String(), bucket).Return(nil)
	q.EXPECT().DeleteWorkspaceFileById(gomock.Any(), sqlc.DeleteWorkspaceFileByIdParams{
		ID: fileID, WorkspaceID: wsID,
	}).Return(nil)

	err := svc.RemoveWorkspaceFile(context.Background(), wsID, fileID, ownerID, "owner")
	assert.NoError(t, err)
}

func TestWorkspaceFileService_RemoveWorkspaceFile_EditorOwnFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	editorID := uuid.New()
	const bucket = "ws-bucket"

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{ID: fileID, UploadedBy: editorID}, nil)
	stor.EXPECT().GetBucketBaseName().Return(bucket)
	stor.EXPECT().DeleteObjectFromBucket(gomock.Any(), gomock.Any(), bucket).Return(nil)
	q.EXPECT().DeleteWorkspaceFileById(gomock.Any(), gomock.Any()).Return(nil)

	err := svc.RemoveWorkspaceFile(context.Background(), wsID, fileID, editorID, "editor")
	assert.NoError(t, err)
}

func TestWorkspaceFileService_RemoveWorkspaceFile_EditorForbiddenOtherFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	uploaderID := uuid.New()
	editorID := uuid.New() // different from uploader

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{ID: fileID, UploadedBy: uploaderID}, nil)

	err := svc.RemoveWorkspaceFile(context.Background(), wsID, fileID, editorID, "editor")
	assert.ErrorIs(t, err, ErrWsForbidden)
}

func TestWorkspaceFileService_RemoveWorkspaceFile_ViewerForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	viewerID := uuid.New()

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{ID: fileID, UploadedBy: viewerID}, nil)

	// Viewers are forbidden even for their own files
	err := svc.RemoveWorkspaceFile(context.Background(), wsID, fileID, viewerID, "viewer")
	assert.ErrorIs(t, err, ErrWsForbidden)
}

func TestWorkspaceFileService_RemoveWorkspaceFile_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{}, sql.ErrNoRows)

	err := svc.RemoveWorkspaceFile(context.Background(), uuid.New(), uuid.New(), uuid.New(), "owner")
	assert.ErrorIs(t, err, ErrWsFileNotFound)
}

// --- MoveWorkspaceFile ----------------------------------------------------

func TestWorkspaceFileService_MoveWorkspaceFile_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	ownerID := uuid.New()

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), sqlc.GetWorkspaceFileByIdParams{
		ID: fileID, WorkspaceID: wsID,
	}).Return(sqlc.WorkspaceFile{ID: fileID, WorkspaceID: wsID, UploadedBy: ownerID}, nil)
	q.EXPECT().MoveWorkspaceFile(gomock.Any(), sqlc.MoveWorkspaceFileParams{
		Path: "/archive", ID: fileID, WorkspaceID: wsID,
	}).Return(nil)

	err := svc.MoveWorkspaceFile(context.Background(), wsID, fileID, "/archive", ownerID, "owner")
	assert.NoError(t, err)
}

func TestWorkspaceFileService_MoveWorkspaceFile_EditorForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	editorID := uuid.New()
	realOwner := uuid.New()

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{ID: fileID, WorkspaceID: wsID, UploadedBy: realOwner}, nil)

	err := svc.MoveWorkspaceFile(context.Background(), wsID, fileID, "/archive", editorID, "editor")
	assert.ErrorIs(t, err, ErrWsForbidden)
}

func TestWorkspaceFileService_MoveWorkspaceFile_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{}, sql.ErrNoRows)

	err := svc.MoveWorkspaceFile(context.Background(), uuid.New(), uuid.New(), "/dst", uuid.New(), "owner")
	assert.ErrorIs(t, err, ErrWsFileNotFound)
}

// --- GetWorkspaceFileDownloadInfo ----------------------------------------

func TestWorkspaceFileService_GetWorkspaceFileDownloadInfo_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	wsID := uuid.New()
	fileID := uuid.New()
	const bucket = "workspace-store"

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), sqlc.GetWorkspaceFileByIdParams{
		ID: fileID, WorkspaceID: wsID,
	}).Return(sqlc.WorkspaceFile{
		ID:          fileID,
		WorkspaceID: wsID,
		FileName:    "design.fig",
		FileType:    sql.NullString{Valid: true, String: "application/octet-stream"},
	}, nil)
	stor.EXPECT().GetBucketBaseName().Return(bucket)

	info, err := svc.GetWorkspaceFileDownloadInfo(context.Background(), wsID, fileID)
	require.NoError(t, err)
	assert.Equal(t, bucket, info.Bucket)
	assert.Equal(t, "design.fig", info.FileName)
	assert.Equal(t, wsID.String()+"/"+fileID.String(), info.ObjectKey)
}

func TestWorkspaceFileService_GetWorkspaceFileDownloadInfo_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := NewWorkspaceFileService(q, stor)

	q.EXPECT().GetWorkspaceFileById(gomock.Any(), gomock.Any()).
		Return(sqlc.WorkspaceFile{}, sql.ErrNoRows)

	_, err := svc.GetWorkspaceFileDownloadInfo(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrWsFileNotFound)
}
