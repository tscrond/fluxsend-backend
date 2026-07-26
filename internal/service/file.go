package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
)

// FileTreeEntry describes a single file within the virtual filesystem tree.
type FileTreeEntry struct {
	Name        string `json:"name"`
	FileType    string `json:"file_type"`
	Size        int64  `json:"size"`
	MD5Checksum string `json:"md5_checksum"`
}

// FilesTree is the result of a directory listing at a given path.
type FilesTree struct {
	Path    string          `json:"path"`
	Folders []string        `json:"folders"`
	Files   []FileTreeEntry `json:"files"`
}

// FileService encapsulates all business logic for file management.
type FileService interface {
	CreateUploadWithId(ctx context.Context, fd *filedata.CreateUploadIdParams) (*filedata.CreateUploadIdResponse, error)
	UploadPart(ctx context.Context, uploadId string, partNumber int32, body io.ReadCloser, size int64) (*filedata.UploadPartResult, error)
	AbortUpload(ctx context.Context, uploadId string) (*filedata.AbortUploadResult, error)
	CompleteUpload(ctx context.Context, uploadId string) (*filedata.CompleteUploadResult, error)
	Upload(ctx context.Context, fd *filedata.FileData) error
	GetFilesTree(ctx context.Context, userID uuid.UUID, path string) (*FilesTree, error)
	GetFolders(ctx context.Context, userID uuid.UUID, path string) ([]string, error)
	DeleteFile(ctx context.Context, userID uuid.UUID, fileName string) error
	DeleteFiles(ctx context.Context, userID uuid.UUID, fileNames []string) (deleted []string, failed []string, err error)
	DeleteFolder(ctx context.Context, userID uuid.UUID, folderPath string, recursive bool) (int, error)
	MoveFile(ctx context.Context, userID uuid.UUID, source, destination string) error
	MoveFolder(ctx context.Context, userID uuid.UUID, source, destination string) (int, error)
	GetNote(ctx context.Context, userID uuid.UUID, checksum string) (string, error)
	UpsertNote(ctx context.Context, userID uuid.UUID, checksum, content string) (string, error)
}

type fileService struct {
	log        *zap.SugaredLogger
	queries    sqlc.Querier
	storage    storagetypes.ObjectStorage
	sanitizer  *bluemonday.Policy
	repository repo.Repository
}

func NewFileService(log *zap.SugaredLogger, queries sqlc.Querier, storage storagetypes.ObjectStorage, sanitizer *bluemonday.Policy, repository repo.Repository) FileService {
	return &fileService{
		log:        log,
		queries:    queries,
		storage:    storage,
		sanitizer:  sanitizer,
		repository: repository,
	}
}

var (
	ErrMultipartUploadClosed      = errors.New("multipart upload is not open")
	ErrMultipartUploadIncomplete  = errors.New("multipart upload is incomplete")
	ErrMultipartUploadUnsupported = errors.New("multipart upload backend is not supported")
)

func (s *fileService) UploadPart(ctx context.Context, uploadId string, partNumber int32, body io.ReadCloser, size int64) (*filedata.UploadPartResult, error) {
	id, err := uuid.Parse(uploadId)
	if err != nil {
		return nil, fmt.Errorf("invalid_upload_id")
	}

	upload, err := s.queries.GetFileUploadById(ctx, id)
	if err != nil {
		return nil, err
	}

	if upload.Status != "uploading" {
		return nil, fmt.Errorf("upload_closed")
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), upload.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("resolving bucket: %w", err)
	}
	uploadPartResult, err := s.storage.UploadPart(
		ctx,
		bucket,
		upload.StorageMapping.String(),
		upload.StorageUploadID.String,
		partNumber,
		body,
		size,
	)
	if err != nil {
		s.log.Errorw("error uploading chunk to s3", "err", err)
		return nil, err
	}
	if uploadPartResult == nil {
		return nil, fmt.Errorf("%w: storage backend returned empty part result", storagetypes.ErrUploadFailed)
	}

	metadata, err := json.Marshal(uploadPartResult.StorageMetadata)
	if err != nil {
		return nil, err
	}

	_, err = s.queries.SaveFileUploadPart(ctx, sqlc.SaveFileUploadPartParams{
		ID:              upload.ID,
		PartNumber:      partNumber,
		StorageMetadata: metadata,
		Size:            uploadPartResult.Size,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMultipartUploadClosed
		}
		logger.FromContext(ctx).Errorw("error updating file upload parts DB ", "chunk_id", partNumber, "error", err)
		return nil, err
	}

	return &filedata.UploadPartResult{
		PartNumber:      uploadPartResult.PartNumber,
		Size:            uploadPartResult.Size,
		StorageMetadata: uploadPartResult.StorageMetadata,
	}, nil
}

func (s *fileService) AbortUpload(ctx context.Context, uploadId string) (*filedata.AbortUploadResult, error) {
	id, err := uuid.Parse(uploadId)
	if err != nil {
		return nil, fmt.Errorf("invalid_upload_id")
	}

	upload, err := s.queries.GetFileUploadById(ctx, id)
	if err != nil {
		return nil, err
	}

	switch upload.Status {
	case "completed", "aborted", "failed":
		return uploadAbortResult(upload), nil
	case "uploading":
	default:
		return uploadAbortResult(upload), nil
	}

	abortedUpload, err := s.queries.AbortFileUpload(ctx, upload.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			refreshed, refreshErr := s.queries.GetFileUploadById(ctx, upload.ID)
			if refreshErr != nil {
				return nil, refreshErr
			}
			return uploadAbortResult(refreshed), nil
		}
		return nil, err
	}

	storageBackend := strings.TrimSpace(strings.ToLower(abortedUpload.StorageBackend))
	if storageBackend == "" {
		storageBackend = "s3"
	}

	if storageBackend != "s3" || !abortedUpload.StorageUploadID.Valid || strings.TrimSpace(abortedUpload.StorageUploadID.String) == "" {
		if err := s.queries.DeleteFileUploadPartsByUploadID(ctx, abortedUpload.ID); err != nil {
			return nil, err
		}
		return uploadAbortResult(abortedUpload), nil
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), abortedUpload.OwnerID)
	if err != nil {
		_, _ = s.queries.FailFileUpload(ctx, abortedUpload.ID)
		return nil, fmt.Errorf("resolving bucket: %w", err)
	}

	if err := s.storage.AbortMultipartUpload(ctx, bucket, abortedUpload.StorageMapping.String(), abortedUpload.StorageUploadID.String); err != nil {
		_, _ = s.queries.FailFileUpload(ctx, abortedUpload.ID)
		return nil, err
	}

	if err := s.queries.DeleteFileUploadPartsByUploadID(ctx, abortedUpload.ID); err != nil {
		return nil, err
	}

	return uploadAbortResult(abortedUpload), nil
}

func (s *fileService) CompleteUpload(ctx context.Context, uploadId string) (*filedata.CompleteUploadResult, error) {
	id, err := uuid.Parse(uploadId)
	if err != nil {
		return nil, fmt.Errorf("invalid_upload_id")
	}

	upload, err := s.queries.GetFileUploadById(ctx, id)
	if err != nil {
		return nil, err
	}

	if upload.Status == "completed" {
		file, getErr := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
			OwnerID:  upload.OwnerID,
			FileName: upload.FileName,
		})
		if getErr != nil {
			return nil, getErr
		}

		return &filedata.CompleteUploadResult{
			UploadId:    upload.ID.String(),
			FileName:    file.FileName,
			Md5Checksum: file.Md5Checksum,
			Size:        upload.UploadedSize,
		}, nil
	}

	if upload.Status != "uploading" {
		return nil, ErrMultipartUploadClosed
	}

	storageBackend := strings.TrimSpace(strings.ToLower(upload.StorageBackend))
	if storageBackend == "" {
		storageBackend = "s3"
	}
	if storageBackend != "s3" {
		return nil, ErrMultipartUploadUnsupported
	}

	if !upload.StorageUploadID.Valid || strings.TrimSpace(upload.StorageUploadID.String) == "" {
		return nil, fmt.Errorf("%w: missing storage upload id", storagetypes.ErrUploadFailed)
	}

	_, err = s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  upload.OwnerID,
		FileName: upload.FileName,
	})
	if err == nil {
		return nil, storagetypes.ErrFileAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	partRows, err := s.queries.ListFileUploadPartsByUploadID(ctx, upload.ID)
	if err != nil {
		return nil, err
	}

	parts, uploadedSize, err := buildCompletedParts(partRows, upload.ExpectedSize)
	if err != nil {
		return nil, err
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), upload.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("resolving bucket: %w", err)
	}

	if s.repository == nil {
		return nil, fmt.Errorf("multipart completion requires repository transaction support")
	}

	completed, err := s.storage.CompleteMultipartUpload(
		ctx,
		bucket,
		upload.StorageMapping.String(),
		upload.StorageUploadID.String,
		parts,
	)
	if err != nil {
		s.log.Errorw("error completing multipart upload", "upload_id", upload.ID, "err", err)
		return nil, err
	}
	if completed == nil {
		return nil, fmt.Errorf("%w: storage backend returned empty completion result", storagetypes.ErrUploadFailed)
	}

	checksum := strings.TrimSpace(completed.ETag)
	if checksum == "" {
		checksum = upload.StorageMapping.String()
	}

	privateDownloadToken, err := pkg.RandToken(32)
	if err != nil {
		return nil, err
	}

	tx, err := s.repository.BeginTx(ctx, nil)
	if err != nil {
		s.handleCompletedUploadFailure(ctx, upload, bucket, "begin tx failure")
		return nil, err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txq := s.repository.Queries().WithTx(tx)
	file, err := txq.InsertFile(ctx, sqlc.InsertFileParams{
		OwnerID:              upload.OwnerID,
		FileName:             upload.FileName,
		FileType:             upload.FileType,
		Size:                 sql.NullInt64{Valid: true, Int64: uploadedSize},
		Md5Checksum:          checksum,
		PrivateDownloadToken: sql.NullString{Valid: true, String: privateDownloadToken},
		StorageMapping:       upload.StorageMapping,
	})
	if err != nil {
		s.handleCompletedUploadFailure(ctx, upload, bucket, "file insert failure")
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storagetypes.ErrFileAlreadyExists
		}
		return nil, err
	}

	if _, err := txq.CompleteFileUpload(ctx, sqlc.CompleteFileUploadParams{
		ID:           upload.ID,
		UploadedSize: uploadedSize,
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			committed = true

			refreshed, refreshErr := s.queries.GetFileUploadById(ctx, upload.ID)
			if refreshErr == nil {
				switch refreshed.Status {
				case "completed":
					fileRow, fileErr := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
						OwnerID:  refreshed.OwnerID,
						FileName: refreshed.FileName,
					})
					if fileErr == nil {
						return &filedata.CompleteUploadResult{
							UploadId:    refreshed.ID.String(),
							FileName:    fileRow.FileName,
							Md5Checksum: fileRow.Md5Checksum,
							Size:        refreshed.UploadedSize,
						}, nil
					}
				case "aborted":
					s.cleanupCompletedMultipartObject(ctx, upload, bucket, "upload completion closed")
					return nil, ErrMultipartUploadClosed
				case "failed":
					s.cleanupCompletedMultipartObject(ctx, upload, bucket, "upload completion closed")
					return nil, ErrMultipartUploadClosed
				}
			}

			s.handleCompletedUploadFailure(ctx, upload, bucket, "upload completion closed")
			return nil, ErrMultipartUploadClosed
		}
		s.handleCompletedUploadFailure(ctx, upload, bucket, "upload completion update failure")
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		s.handleCompletedUploadFailure(ctx, upload, bucket, "commit failure")
		return nil, err
	}
	committed = true

	return &filedata.CompleteUploadResult{
		UploadId:    upload.ID.String(),
		FileName:    file.FileName,
		Md5Checksum: file.Md5Checksum,
		Size:        uploadedSize,
	}, nil
}

func uploadAbortResult(upload sqlc.FileUpload) *filedata.AbortUploadResult {
	return &filedata.AbortUploadResult{
		UploadId:     upload.ID.String(),
		Status:       upload.Status,
		UploadedSize: upload.UploadedSize,
	}
}

func (s *fileService) handleCompletedUploadFailure(ctx context.Context, upload sqlc.FileUpload, bucket, reason string) {
	if _, err := s.queries.FailFileUpload(ctx, upload.ID); err != nil {
		logger.FromContext(ctx).Errorw("error marking upload failed", "upload_id", upload.ID, "reason", reason, "error", err)
	}
	s.cleanupCompletedMultipartObject(ctx, upload, bucket, reason)
}

func (s *fileService) cleanupCompletedMultipartObject(ctx context.Context, upload sqlc.FileUpload, bucket, reason string) {
	if delErr := s.storage.DeleteObjectFromBucket(ctx, upload.StorageMapping.String(), bucket); delErr != nil {
		logger.FromContext(ctx).Errorw("error deleting completed multipart object after failure", "upload_id", upload.ID, "reason", reason, "object", upload.StorageMapping.String(), "error", delErr)
	}
}

func (s *fileService) CreateUploadWithId(ctx context.Context, fd *filedata.CreateUploadIdParams) (*filedata.CreateUploadIdResponse, error) {

	fileName := fd.FileName
	if fd.Folder != "" {
		fileName = fd.Folder + "/" + fileName
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), fd.OwnerUserID)
	if err != nil {
		return nil, err
	}

	storageMapping := uuid.New()
	uploadId, err := s.storage.CreateMultipartUpload(ctx, bucket, storageMapping.String(), fd.ContentType)
	if err != nil {
		logger.FromContext(ctx).Errorw("error creating file upload ID", "error", err)
		return nil, err
	}
	if uploadId == nil || strings.TrimSpace(*uploadId) == "" {
		return nil, fmt.Errorf("%w: storage backend returned empty upload id", storagetypes.ErrUploadFailed)
	}
	storageBackend := strings.TrimSpace(strings.ToLower(fd.StorageBackend))
	if storageBackend == "" {
		storageBackend = "s3"
	}

	result, err := s.queries.CreateFileUpload(ctx, sqlc.CreateFileUploadParams{
		OwnerID:         fd.OwnerUserID,
		StorageBackend:  storageBackend,
		StorageUploadID: sql.NullString{Valid: true, String: *uploadId},
		StorageMapping:  storageMapping,
		FileName:        fileName,
		FileType:        sql.NullString{Valid: strings.TrimSpace(fd.ContentType) != "", String: fd.ContentType},
		ExpectedSize:    fd.Size,
		Status:          "uploading",
	})
	if err != nil {
		logger.FromContext(ctx).Errorw("error creating new file upload", "error", err)
		return nil, err
	}

	return &filedata.CreateUploadIdResponse{
		UploadId:  result.ID.String(),
		ChunkSize: pkg.OptimalChunkSize(fd.Size),
	}, nil
}

func (s *fileService) Upload(ctx context.Context, fd *filedata.FileData) error {
	fileName := fd.RequestHeaders.Filename
	if fd.Folder != "" {
		fileName = fd.Folder + "/" + fileName
	}

	_, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  fd.OwnerUserID,
		FileName: fileName,
	})
	if err == nil {
		return storagetypes.ErrFileAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		logger.FromContext(ctx).Errorw("error checking existing file", "error", err)
		return storagetypes.ErrUploadFailed
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), fd.OwnerUserID)
	if err != nil {
		return err
	}

	contentType := fd.RequestHeaders.Header.Get("Content-Type")
	if contentType == "" {
		buffer := make([]byte, 512)
		_, readErr := fd.MultipartFile.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		fd.MultipartFile.Seek(0, io.SeekStart)
		contentType = http.DetectContentType(buffer)
	}

	storageMapping := uuid.New()

	result, err := s.storage.PutObject(ctx, bucket, storageMapping.String(), fd.MultipartFile, fd.RequestHeaders.Size, contentType)
	if err != nil {
		logger.FromContext(ctx).Errorw("error uploading file", "error", err)
		return err
	}

	privateDownloadToken, err := pkg.RandToken(32)
	if err != nil {
		logger.FromContext(ctx).Errorw("error generating private download token", "error", err)
		return err
	}

	file, err := s.queries.InsertFile(ctx, sqlc.InsertFileParams{
		OwnerID:              fd.OwnerUserID,
		FileName:             fileName,
		FileType:             sql.NullString{Valid: true, String: result.ContentType},
		Size:                 sql.NullInt64{Valid: true, Int64: result.Size},
		Md5Checksum:          result.MD5,
		PrivateDownloadToken: sql.NullString{Valid: true, String: privateDownloadToken},
		StorageMapping:       storageMapping,
	})
	if err != nil {
		logger.FromContext(ctx).Errorw("error inserting file to DB, removing object from storage", "error", err)
		if delErr := s.storage.DeleteObjectFromBucket(ctx, storageMapping.String(), bucket); delErr != nil {
			logger.FromContext(ctx).Errorw("error deleting object during rollback", "object", storageMapping.String(), "error", delErr)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return storagetypes.ErrFileAlreadyExists
		}
		return storagetypes.ErrUploadFailed
	}
	logger.FromContext(ctx).Infow("file uploaded", "file", fileName, "checksum", file.Md5Checksum)
	return nil
}

func buildCompletedParts(rows []sqlc.FileUploadPart, expectedSize int64) ([]storagetypes.CompletedPart, int64, error) {
	if len(rows) == 0 {
		return nil, 0, ErrMultipartUploadIncomplete
	}

	parts := make([]storagetypes.CompletedPart, 0, len(rows))
	var uploadedSize int64
	expectedPartNumber := int32(1)

	for _, row := range rows {
		if row.PartNumber != expectedPartNumber {
			return nil, 0, ErrMultipartUploadIncomplete
		}
		if row.Size <= 0 {
			return nil, 0, ErrMultipartUploadIncomplete
		}

		metadata := map[string]any{}
		if len(row.StorageMetadata) > 0 {
			if err := json.Unmarshal(row.StorageMetadata, &metadata); err != nil {
				return nil, 0, fmt.Errorf("%w: invalid multipart storage metadata", storagetypes.ErrTypeConversion)
			}
		}

		parts = append(parts, storagetypes.CompletedPart{
			PartNumber:      row.PartNumber,
			Size:            row.Size,
			StorageMetadata: metadata,
		})
		uploadedSize += row.Size
		expectedPartNumber++
	}

	if uploadedSize != expectedSize {
		return nil, 0, ErrMultipartUploadIncomplete
	}

	return parts, uploadedSize, nil
}

func (s *fileService) GetFilesTree(ctx context.Context, userID uuid.UUID, path string) (*FilesTree, error) {
	filesByOwner, err := s.queries.GetFilesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}

	foldersSet := map[string]struct{}{}
	files := make([]FileTreeEntry, 0)
	for _, f := range filesByOwner {
		rel, include := relativeToPath(f.FileName, path)
		if !include || rel == "" {
			continue
		}
		if slash := strings.Index(rel, "/"); slash >= 0 {
			foldersSet[rel[:slash]] = struct{}{}
			continue
		}
		files = append(files, FileTreeEntry{
			Name:        f.FileName,
			FileType:    f.FileType.String,
			Size:        f.Size.Int64,
			MD5Checksum: f.Md5Checksum,
		})
	}

	folders := make([]string, 0, len(foldersSet))
	for folder := range foldersSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	return &FilesTree{Path: path, Folders: folders, Files: files}, nil
}

func (s *fileService) GetFolders(ctx context.Context, userID uuid.UUID, path string) ([]string, error) {
	filesByOwner, err := s.queries.GetFilesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}

	foldersSet := map[string]struct{}{}
	for _, f := range filesByOwner {
		rel, include := relativeToPath(f.FileName, path)
		if !include || rel == "" {
			continue
		}
		if slash := strings.Index(rel, "/"); slash >= 0 {
			foldersSet[rel[:slash]] = struct{}{}
		}
	}

	folders := make([]string, 0, len(foldersSet))
	for folder := range foldersSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	return folders, nil
}

func (s *fileService) DeleteFile(ctx context.Context, userID uuid.UUID, fileName string) error {
	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID)
	if err != nil {
		return fmt.Errorf("resolving bucket: %w", err)
	}

	fileRow, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  userID,
		FileName: fileName,
	})
	if err != nil {
		return err
	}

	if err := s.storage.DeleteObjectFromBucket(ctx, fileRow.StorageMapping.String(), bucket); err != nil {
		logger.FromContext(ctx).Warnw("issues deleting object from storage (non-fatal)", "error", err)
	}

	return s.queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
		OwnerID:  userID,
		FileName: fileName,
	})
}

func (s *fileService) DeleteFiles(ctx context.Context, userID uuid.UUID, fileNames []string) (deleted []string, failed []string, err error) {
	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving bucket: %w", err)
	}

	type resolvedEntry struct {
		logicalName string
		objectKey   string
	}

	resolved := make([]resolvedEntry, 0, len(fileNames))
	storageKeys := make([]string, 0, len(fileNames))

	for _, name := range fileNames {
		if name == "" {
			continue
		}
		fileRow, lookupErr := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
			OwnerID:  userID,
			FileName: name,
		})
		if lookupErr != nil {
			logger.FromContext(ctx).Warnw("resolving file from DB", "file", name, "error", lookupErr)
			failed = append(failed, name)
			continue
		}
		resolved = append(resolved, resolvedEntry{logicalName: name, objectKey: fileRow.StorageMapping.String()})
		storageKeys = append(storageKeys, fileRow.StorageMapping.String())
	}

	if deleteErr := s.storage.DeleteObjectsFromBucket(ctx, storageKeys, bucket); deleteErr != nil {
		logger.FromContext(ctx).Warnw("issues bulk-deleting objects from storage (non-fatal)", "error", deleteErr)
	}

	for _, entry := range resolved {
		if dbErr := s.queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
			OwnerID:  userID,
			FileName: entry.logicalName,
		}); dbErr != nil {
			logger.FromContext(ctx).Warnw("deleting file from DB", "file", entry.logicalName, "error", dbErr)
			failed = append(failed, entry.logicalName)
			continue
		}
		deleted = append(deleted, entry.logicalName)
	}
	return deleted, failed, nil
}

func (s *fileService) DeleteFolder(ctx context.Context, userID uuid.UUID, folderPath string, recursive bool) (int, error) {
	filesByOwner, err := s.queries.GetFilesByOwner(ctx, userID)
	if err != nil {
		return 0, err
	}

	prefix := folderPrefix(folderPath)
	toDelete := make([]sqlc.File, 0)
	for _, f := range filesByOwner {
		if strings.HasPrefix(f.FileName, prefix) {
			toDelete = append(toDelete, f)
		}
	}

	if len(toDelete) > 0 && !recursive {
		return 0, ErrRecursiveRequired
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID)
	if err != nil {
		return 0, fmt.Errorf("resolving bucket: %w", err)
	}

	count := 0
	for _, f := range toDelete {
		if err := s.storage.DeleteObjectFromBucket(ctx, f.StorageMapping.String(), bucket); err != nil {
			return count, fmt.Errorf("deleting object %q from storage: %w", f.FileName, err)
		}
		if err := s.queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
			OwnerID:  userID,
			FileName: f.FileName,
		}); err != nil {
			return count, fmt.Errorf("deleting file %q from DB: %w", f.FileName, err)
		}
		count++
	}
	return count, nil
}

func (s *fileService) MoveFile(ctx context.Context, userID uuid.UUID, source, destination string) error {
	sourceFile, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  userID,
		FileName: source,
	})
	if err != nil {
		return err
	}
	return s.queries.UpdateFileNameByID(ctx, sqlc.UpdateFileNameByIDParams{
		FileName: destination,
		ID:       sourceFile.ID,
	})
}

func (s *fileService) MoveFolder(ctx context.Context, userID uuid.UUID, source, destination string) (int, error) {
	filesByOwner, err := s.queries.GetFilesByOwner(ctx, userID)
	if err != nil {
		return 0, err
	}

	sourcePrefix := folderPrefix(source)
	toMove := make([]sqlc.File, 0)
	for _, f := range filesByOwner {
		if strings.HasPrefix(f.FileName, sourcePrefix) {
			toMove = append(toMove, f)
		}
	}

	if len(toMove) == 0 {
		return 0, sql.ErrNoRows
	}

	moved := 0
	for _, f := range toMove {
		relative := strings.TrimPrefix(f.FileName, sourcePrefix)
		newPath := destination + "/" + relative
		if err := s.queries.UpdateFileNameByID(ctx, sqlc.UpdateFileNameByIDParams{
			FileName: newPath,
			ID:       f.ID,
		}); err != nil {
			return moved, fmt.Errorf("renaming file %q: %w", f.FileName, err)
		}
		moved++
	}
	return moved, nil
}

func (s *fileService) GetNote(ctx context.Context, userID uuid.UUID, checksum string) (string, error) {
	fileID, err := s.findOwnedFileIDByChecksum(ctx, userID, checksum)
	if err != nil {
		return "", err
	}
	note, err := s.queries.GetNoteForFileById(ctx, sqlc.GetNoteForFileByIdParams{
		UserID: userID,
		FileID: sql.NullInt32{Valid: true, Int32: fileID},
	})
	if err != nil {
		return "", err
	}
	return note.Content, nil
}

func (s *fileService) UpsertNote(ctx context.Context, userID uuid.UUID, checksum, content string) (string, error) {
	sanitized := s.sanitizer.Sanitize(content)
	if utf8.RuneCountInString(sanitized) > 500 {
		return "", ErrNoteTooLong
	}
	fileID, err := s.findOwnedFileIDByChecksum(ctx, userID, checksum)
	if err != nil {
		return "", err
	}
	if _, err := s.queries.UpdateNoteForFile(ctx, sqlc.UpdateNoteForFileParams{
		UserID:  userID,
		FileID:  sql.NullInt32{Valid: true, Int32: fileID},
		Content: sanitized,
	}); err != nil {
		return "", err
	}
	return sanitized, nil
}

func (s *fileService) findOwnedFileIDByChecksum(ctx context.Context, userID uuid.UUID, checksum string) (int32, error) {
	return s.queries.GetFileFromChecksum(ctx, sqlc.GetFileFromChecksumParams{
		OwnerID:     userID,
		Md5Checksum: checksum,
	})
}

// resolveUserBucketName resolves the stored bucket name for a user, falling back
// to constructing it from the base name and users.id.
func resolveUserBucketName(ctx context.Context, queries sqlc.Querier, baseBucketName string, userID uuid.UUID) (string, error) {
	stored, err := queries.GetUserBucketById(ctx, userID)
	if err != nil {
		return "", err
	}
	if stored.Valid && strings.TrimSpace(stored.String) != "" {
		return strings.TrimSpace(stored.String), nil
	}
	return pkg.GetUserBucketName(baseBucketName, userID.String()), nil
}

func relativeToPath(fullPath, currentPath string) (string, bool) {
	if currentPath == "" {
		return fullPath, true
	}
	prefix := currentPath + "/"
	if strings.HasPrefix(fullPath, prefix) {
		return strings.TrimPrefix(fullPath, prefix), true
	}
	return "", false
}

func folderPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "/"
}
