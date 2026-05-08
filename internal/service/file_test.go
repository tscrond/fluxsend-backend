package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

// newFileTestSvc creates a FileService backed by the given mocks.
func newFileTestSvc(q *mocks.MockQuerier, s *mocks.MockObjectStorage) FileService {
	return NewFileService(q, s, bluemonday.UGCPolicy())
}

// --- GetFilesTree ----------------------------------------------------------

func TestFileService_GetFilesTree_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	files := []sqlc.File{
		{FileName: "doc.txt", FileType: sql.NullString{Valid: true, String: "text/plain"}, Size: sql.NullInt64{Valid: true, Int64: 512}, Md5Checksum: "abc123"},
		{FileName: "imgs/photo.jpg", FileType: sql.NullString{Valid: true, String: "image/jpeg"}, Size: sql.NullInt64{Valid: true, Int64: 1024}, Md5Checksum: "def456"},
	}

	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).Return(files, nil)

	tree, err := svc.GetFilesTree(context.Background(), ownerID, "")

	require.NoError(t, err)
	assert.Equal(t, "", tree.Path)
	assert.Len(t, tree.Files, 1)
	assert.Equal(t, "doc.txt", tree.Files[0].Name)
	assert.Len(t, tree.Folders, 1)
	assert.Equal(t, "imgs", tree.Folders[0])
}

func TestFileService_GetFilesTree_SubPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	files := []sqlc.File{
		{FileName: "imgs/photo.jpg"},
		{FileName: "imgs/sub/nested.jpg"},
		{FileName: "doc.txt"},
	}

	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).Return(files, nil)

	tree, err := svc.GetFilesTree(context.Background(), ownerID, "imgs")

	require.NoError(t, err)
	assert.Equal(t, "imgs", tree.Path)
	assert.Len(t, tree.Files, 1)
	assert.Equal(t, "imgs/photo.jpg", tree.Files[0].Name)
	assert.Len(t, tree.Folders, 1)
	assert.Equal(t, "sub", tree.Folders[0])
}

func TestFileService_GetFilesTree_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	dbErr := errors.New("db unavailable")
	q.EXPECT().GetFilesByOwner(gomock.Any(), gomock.Any()).Return(nil, dbErr)

	_, err := svc.GetFilesTree(context.Background(), uuid.New(), "")
	assert.ErrorIs(t, err, dbErr)
}

func TestFileService_GetFilesTree_EmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	q.EXPECT().GetFilesByOwner(gomock.Any(), gomock.Any()).Return(nil, nil)

	tree, err := svc.GetFilesTree(context.Background(), uuid.New(), "")
	require.NoError(t, err)
	assert.Empty(t, tree.Files)
	assert.Empty(t, tree.Folders)
}

// --- GetFolders -----------------------------------------------------------

func TestFileService_GetFolders_ReturnsUniqueSorted(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	files := []sqlc.File{
		{FileName: "z-folder/a.txt"},
		{FileName: "a-folder/b.txt"},
		{FileName: "a-folder/c.txt"},
		{FileName: "root.txt"},
	}
	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).Return(files, nil)

	folders, err := svc.GetFolders(context.Background(), ownerID, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"a-folder", "z-folder"}, folders)
}

// --- DeleteFile -----------------------------------------------------------

func TestFileService_DeleteFile_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	storageID := uuid.New()
	const bucketBase = "fluxsend"
	const internalID = "internal-123"

	stor.EXPECT().GetBucketBaseName().Return(bucketBase)
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).
		Return(sql.NullString{Valid: true, String: bucketBase + "-" + internalID}, nil)
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), sqlc.GetFileByOwnerAndNameParams{
		OwnerID: ownerID, FileName: "doc.txt",
	}).Return(sqlc.GetFileByOwnerAndNameRow{StorageMapping: storageID}, nil)
	stor.EXPECT().DeleteObjectFromBucket(gomock.Any(), storageID.String(), bucketBase+"-"+internalID).
		Return(nil)
	q.EXPECT().DeleteFileByNameAndId(gomock.Any(), sqlc.DeleteFileByNameAndIdParams{
		OwnerID: ownerID, FileName: "doc.txt",
	}).Return(nil)

	err := svc.DeleteFile(context.Background(), ownerID, internalID, "doc.txt")
	assert.NoError(t, err)
}

func TestFileService_DeleteFile_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	const internalID = "internal-123"

	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).
		Return(sql.NullString{Valid: false}, nil)
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), gomock.Any()).
		Return(sqlc.GetFileByOwnerAndNameRow{}, sql.ErrNoRows)

	err := svc.DeleteFile(context.Background(), ownerID, internalID, "missing.txt")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestFileService_DeleteFile_StorageErrorNonFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	storageID := uuid.New()
	const bucket = "fluxsend-internal-123"

	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).
		Return(sql.NullString{Valid: true, String: bucket}, nil)
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), gomock.Any()).
		Return(sqlc.GetFileByOwnerAndNameRow{StorageMapping: storageID}, nil)
	stor.EXPECT().DeleteObjectFromBucket(gomock.Any(), storageID.String(), bucket).
		Return(errors.New("storage error")) // non-fatal
	q.EXPECT().DeleteFileByNameAndId(gomock.Any(), gomock.Any()).Return(nil)

	err := svc.DeleteFile(context.Background(), ownerID, "internal-123", "doc.txt")
	assert.NoError(t, err) // storage error is logged but not returned
}

// --- DeleteFiles ----------------------------------------------------------

func TestFileService_DeleteFiles_PartialSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	storageID := uuid.New()
	const bucket = "fluxsend-internal-456"

	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).
		Return(sql.NullString{Valid: true, String: bucket}, nil)

	// First file resolves OK
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), sqlc.GetFileByOwnerAndNameParams{OwnerID: ownerID, FileName: "ok.txt"}).
		Return(sqlc.GetFileByOwnerAndNameRow{StorageMapping: storageID}, nil)
	// Second file missing from DB
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), sqlc.GetFileByOwnerAndNameParams{OwnerID: ownerID, FileName: "missing.txt"}).
		Return(sqlc.GetFileByOwnerAndNameRow{}, sql.ErrNoRows)

	stor.EXPECT().DeleteObjectsFromBucket(gomock.Any(), []string{storageID.String()}, bucket).Return(nil)
	q.EXPECT().DeleteFileByNameAndId(gomock.Any(), sqlc.DeleteFileByNameAndIdParams{OwnerID: ownerID, FileName: "ok.txt"}).
		Return(nil)

	deleted, failed, err := svc.DeleteFiles(context.Background(), ownerID, "internal-456", []string{"ok.txt", "missing.txt"})
	require.NoError(t, err)
	assert.Equal(t, []string{"ok.txt"}, deleted)
	assert.Equal(t, []string{"missing.txt"}, failed)
}

// --- MoveFile -------------------------------------------------------------

func TestFileService_MoveFile_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	fileID := int32(42)

	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), sqlc.GetFileByOwnerAndNameParams{OwnerID: ownerID, FileName: "src.txt"}).
		Return(sqlc.GetFileByOwnerAndNameRow{ID: fileID}, nil)
	q.EXPECT().UpdateFileNameByID(gomock.Any(), sqlc.UpdateFileNameByIDParams{FileName: "dst/src.txt", ID: fileID}).
		Return(nil)

	err := svc.MoveFile(context.Background(), ownerID, "src.txt", "dst/src.txt")
	assert.NoError(t, err)
}

func TestFileService_MoveFile_SourceNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), gomock.Any()).
		Return(sqlc.GetFileByOwnerAndNameRow{}, sql.ErrNoRows)

	err := svc.MoveFile(context.Background(), uuid.New(), "ghost.txt", "dst/ghost.txt")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

// --- MoveFolder -----------------------------------------------------------

func TestFileService_MoveFolder_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	files := []sqlc.File{
		{ID: 1, FileName: "old/a.txt"},
		{ID: 2, FileName: "old/b.txt"},
		{ID: 3, FileName: "other/x.txt"},
	}
	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).Return(files, nil)
	q.EXPECT().UpdateFileNameByID(gomock.Any(), sqlc.UpdateFileNameByIDParams{FileName: "new/a.txt", ID: 1}).Return(nil)
	q.EXPECT().UpdateFileNameByID(gomock.Any(), sqlc.UpdateFileNameByIDParams{FileName: "new/b.txt", ID: 2}).Return(nil)

	moved, err := svc.MoveFolder(context.Background(), ownerID, "old", "new")
	require.NoError(t, err)
	assert.Equal(t, 2, moved)
}

func TestFileService_MoveFolder_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	q.EXPECT().GetFilesByOwner(gomock.Any(), gomock.Any()).
		Return([]sqlc.File{{FileName: "other/x.txt"}}, nil)

	_, err := svc.MoveFolder(context.Background(), uuid.New(), "nonexistent", "dst")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

// --- DeleteFolder ---------------------------------------------------------

func TestFileService_DeleteFolder_NonRecursiveFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).
		Return([]sqlc.File{{OwnerID: ownerID, FileName: "folder/file.txt", StorageMapping: uuid.New()}}, nil)

	_, err := svc.DeleteFolder(context.Background(), ownerID, "internal-1", "folder", false)
	assert.ErrorIs(t, err, ErrRecursiveRequired)
}

func TestFileService_DeleteFolder_EmptyFolderSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	ownerID := uuid.New()
	q.EXPECT().GetFilesByOwner(gomock.Any(), ownerID).
		Return([]sqlc.File{{FileName: "other/file.txt"}}, nil)
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).
		Return(sql.NullString{Valid: true, String: "fluxsend-internal-1"}, nil)

	count, err := svc.DeleteFolder(context.Background(), ownerID, "internal-1", "emptydir", false)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// --- GetNote / UpsertNote -------------------------------------------------

func TestFileService_GetNote_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	userID := uuid.New()
	fileID := int32(7)
	const checksum = "abc123"
	const noteContent = "This is a note"

	q.EXPECT().GetFileFromChecksum(gomock.Any(), checksum).Return(fileID, nil)
	q.EXPECT().GetNoteForFileById(gomock.Any(), sqlc.GetNoteForFileByIdParams{
		UserID: userID,
		FileID: sql.NullInt32{Valid: true, Int32: fileID},
	}).Return(sqlc.Note{Content: noteContent}, nil)

	content, err := svc.GetNote(context.Background(), userID, checksum)
	require.NoError(t, err)
	assert.Equal(t, noteContent, content)
}

func TestFileService_GetNote_FileNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	q.EXPECT().GetFileFromChecksum(gomock.Any(), "bad-checksum").Return(int32(0), sql.ErrNoRows)

	_, err := svc.GetNote(context.Background(), uuid.New(), "bad-checksum")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestFileService_UpsertNote_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	userID := uuid.New()
	fileID := int32(7)
	const checksum = "abc123"
	const content = "My note"

	q.EXPECT().GetFileFromChecksum(gomock.Any(), checksum).Return(fileID, nil)
	q.EXPECT().UpdateNoteForFile(gomock.Any(), sqlc.UpdateNoteForFileParams{
		UserID:  userID,
		FileID:  sql.NullInt32{Valid: true, Int32: fileID},
		Content: content,
	}).Return(sqlc.Note{Content: content}, nil)

	saved, err := svc.UpsertNote(context.Background(), userID, checksum, content)
	require.NoError(t, err)
	assert.Equal(t, content, saved)
}

func TestFileService_UpsertNote_TooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	longContent := strings.Repeat("x", 501)

	_, err := svc.UpsertNote(context.Background(), uuid.New(), "checksum", longContent)
	assert.ErrorIs(t, err, ErrNoteTooLong)
}
