package service

import (
	"context"
	"encoding/json"
	"database/sql"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

// newFileTestSvc creates a FileService backed by the given mocks.
func newFileTestSvc(q *mocks.MockQuerier, s *mocks.MockObjectStorage) FileService {
	return NewFileService(zap.NewNop().Sugar(), q, s, bluemonday.UGCPolicy(), nil)
}

func newFileTestSvcWithRepository(db *sql.DB, s *mocks.MockObjectStorage) FileService {
	repository := &apiKeyTestRepository{db: db, queries: sqlc.New(db)}
	return NewFileService(zap.NewNop().Sugar(), repository.Queries(), s, bluemonday.UGCPolicy(), repository)
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

// --- UploadPart ----------------------------------------------------------

func TestFileService_UploadPart_StorageReturnsNilResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvc(q, stor)

	uploadID := uuid.New()
	ownerID := uuid.New()
	storageMapping := uuid.New()
	const bucket = "fluxsend-internal-123"
	const storageUploadID = "multipart-upload-1"

	q.EXPECT().GetFileUploadById(gomock.Any(), uploadID).Return(sqlc.FileUpload{
		ID:              uploadID,
		OwnerID:         ownerID,
		StorageUploadID: sql.NullString{Valid: true, String: storageUploadID},
		StorageMapping:  storageMapping,
		Status:          "uploading",
	}, nil)
	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	q.EXPECT().GetUserBucketById(gomock.Any(), ownerID).Return(sql.NullString{Valid: true, String: bucket}, nil)
	stor.EXPECT().UploadPart(gomock.Any(), bucket, storageMapping.String(), storageUploadID, int32(1), gomock.Any(), int64(4)).Return(nil, nil)

	_, err := svc.UploadPart(context.Background(), uploadID.String(), 1, io.NopCloser(strings.NewReader("part")), 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty part result")
}

func TestFileService_CompleteUpload_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctrl := gomock.NewController(t)
	stor := mocks.NewMockObjectStorage(ctrl)
	svc := newFileTestSvcWithRepository(db, stor)

	uploadID := uuid.New()
	ownerID := uuid.New()
	storageMapping := uuid.New()
	partOneID := uuid.New()
	partTwoID := uuid.New()
	createdAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	partOneMetadata, err := json.Marshal(map[string]any{"etag": "etag-1"})
	require.NoError(t, err)
	partTwoMetadata, err := json.Marshal(map[string]any{"etag": "etag-2"})
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, owner_id, storage_backend, storage_upload_id, storage_mapping, file_name, file_type, expected_size, uploaded_size, status, created_at, updated_at FROM file_uploads\nWHERE id = $1\nORDER BY id\nLIMIT 1\n")).
		WithArgs(uploadID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "storage_backend", "storage_upload_id", "storage_mapping", "file_name", "file_type", "expected_size", "uploaded_size", "status", "created_at", "updated_at"}).
			AddRow(uploadID, ownerID, "s3", "storage-upload-1", storageMapping, "docs/report.pdf", "application/pdf", int64(8), int64(0), "uploading", createdAt, createdAt))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, file_name, md5_checksum, storage_mapping\nFROM files\nWHERE owner_id = $1 AND file_name = $2\n")).
		WithArgs(ownerID, "docs/report.pdf").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, upload_id, part_number, storage_metadata, size, created_at FROM file_upload_parts\nWHERE upload_id = $1\nORDER BY part_number\n")).
		WithArgs(uploadID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "upload_id", "part_number", "storage_metadata", "size", "created_at"}).
			AddRow(partOneID, uploadID, int32(1), partOneMetadata, int64(4), createdAt).
			AddRow(partTwoID, uploadID, int32(2), partTwoMetadata, int64(4), createdAt))

	stor.EXPECT().GetBucketBaseName().Return("fluxsend")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_bucket FROM users WHERE id = $1\n")).
		WithArgs(ownerID).
		WillReturnRows(sqlmock.NewRows([]string{"user_bucket"}).AddRow("fluxsend-internal-123"))

	stor.EXPECT().CompleteMultipartUpload(gomock.Any(), "fluxsend-internal-123", storageMapping.String(), "storage-upload-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, bucket string, key string, uploadIDArg string, parts []storagetypes.CompletedPart) (*storagetypes.CompleteMultipartUploadResult, error) {
			require.Len(t, parts, 2)
			assert.Equal(t, int32(1), parts[0].PartNumber)
			assert.Equal(t, "etag-1", parts[0].StorageMetadata["etag"])
			assert.Equal(t, int32(2), parts[1].PartNumber)
			assert.Equal(t, "etag-2", parts[1].StorageMetadata["etag"])
			return &storagetypes.CompleteMultipartUploadResult{ETag: "multipart-etag-2"}, nil
		})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO files (owner_id, file_name, file_type, size, md5_checksum, private_download_token, storage_mapping)\nVALUES ($1, $2, $3, $4, $5, $6, $7)\nRETURNING id, file_name, file_type, size, md5_checksum, private_download_token, owner_id, storage_mapping, created_at\n")).
		WithArgs(ownerID, "docs/report.pdf", sql.NullString{Valid: true, String: "application/pdf"}, sql.NullInt64{Valid: true, Int64: 8}, "multipart-etag-2", sqlmock.AnyArg(), storageMapping).
		WillReturnRows(sqlmock.NewRows([]string{"id", "file_name", "file_type", "size", "md5_checksum", "private_download_token", "owner_id", "storage_mapping", "created_at"}).
			AddRow(int32(1), "docs/report.pdf", "application/pdf", int64(8), "multipart-etag-2", "private-token", ownerID, storageMapping, createdAt))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE file_uploads\nSET\n  status = 'completed',\n  uploaded_size = $2,\n  updated_at = now()\nWHERE id = $1\n")).
		WithArgs(uploadID, int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := svc.CompleteUpload(context.Background(), uploadID.String())
	require.NoError(t, err)
	assert.Equal(t, uploadID.String(), result.UploadId)
	assert.Equal(t, "docs/report.pdf", result.FileName)
	assert.Equal(t, "multipart-etag-2", result.Md5Checksum)
	assert.Equal(t, int64(8), result.Size)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildCompletedParts_RejectsIncompleteUpload(t *testing.T) {
	metadata, err := json.Marshal(map[string]any{"etag": "etag-1"})
	require.NoError(t, err)

	_, _, err = buildCompletedParts([]sqlc.FileUploadPart{
		{
			PartNumber:      1,
			Size:            4,
			StorageMetadata: metadata,
		},
	}, 8)
	require.ErrorIs(t, err, ErrMultipartUploadIncomplete)
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

	err := svc.DeleteFile(context.Background(), ownerID, "doc.txt")
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

	err := svc.DeleteFile(context.Background(), ownerID, "missing.txt")
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

	err := svc.DeleteFile(context.Background(), ownerID, "doc.txt")
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

	deleted, failed, err := svc.DeleteFiles(context.Background(), ownerID, []string{"ok.txt", "missing.txt"})
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

	_, err := svc.DeleteFolder(context.Background(), ownerID, "folder", false)
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

	count, err := svc.DeleteFolder(context.Background(), ownerID, "emptydir", false)
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

	q.EXPECT().GetFileFromChecksum(gomock.Any(), sqlc.GetFileFromChecksumParams{
		OwnerID:     userID,
		Md5Checksum: checksum,
	}).Return(fileID, nil)
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
	userID := uuid.New()

	q.EXPECT().GetFileFromChecksum(gomock.Any(), sqlc.GetFileFromChecksumParams{
		OwnerID:     userID,
		Md5Checksum: "bad-checksum",
	}).Return(int32(0), sql.ErrNoRows)

	_, err := svc.GetNote(context.Background(), userID, "bad-checksum")
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

	q.EXPECT().GetFileFromChecksum(gomock.Any(), sqlc.GetFileFromChecksumParams{
		OwnerID:     userID,
		Md5Checksum: checksum,
	}).Return(fileID, nil)
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
