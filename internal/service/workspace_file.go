package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/lib/pq"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrWsForbidden      = errors.New("insufficient permissions")
	ErrWsFileNotFound   = errors.New("workspace file not found")
	ErrWsFolderNotFound = errors.New("workspace folder not found")
)

// ── Result types ─────────────────────────────────────────────────────────────

type WorkspaceFileResult struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Name        string    `json:"file_name"`
	Type        string    `json:"file_type"`
	Size        int64     `json:"size"`
	Path        string    `json:"path"`
	Md5Checksum string    `json:"md5_checksum"`
	CreatedAt   string    `json:"created_at"`
	UploadedBy  uuid.UUID `json:"uploaded_by"`
}

type WorkspaceFolderResult struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   string    `json:"created_at"`
}

type WorkspaceFileTreeEntry struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	FileType        string    `json:"file_type"`
	Size            int64     `json:"size"`
	MD5Checksum     string    `json:"md5_checksum"`
	UploadedBy      uuid.UUID `json:"uploaded_by"`
	UploadedByEmail string    `json:"uploaded_by_email"`
	CreatedAt       string    `json:"created_at"`
}

type WorkspaceFolderTreeEntry struct {
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	CreatedByEmail string `json:"created_by_email,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type WorkspaceFilesTree struct {
	Path    string                     `json:"path"`
	Files   []WorkspaceFileTreeEntry   `json:"files"`
	Folders []WorkspaceFolderTreeEntry `json:"folders"`
}

// ── Service interface ─────────────────────────────────────────────────────────

type WorkspaceFileDownloadInfo struct {
	ObjectKey string
	FileName  string
	Bucket    string
}

type WorkspaceFileService interface {
	// viewer+ — read-only
	GetWorkspaceFiles(ctx context.Context, workspaceId uuid.UUID) ([]WorkspaceFileResult, error)
	GetWorkspaceFilesTree(ctx context.Context, workspaceId uuid.UUID, path string) (WorkspaceFilesTree, error)
	GetWorkspaceFileDownloadInfo(ctx context.Context, workspaceId, fileId uuid.UUID) (*WorkspaceFileDownloadInfo, error)

	// editor+ — write; callerID/callerRole enforces ownership for editors
	CreateWorkspaceUpload(ctx context.Context, params *filedata.CreateWorkspaceUploadParams) (*filedata.CreateUploadIdResponse, error)
	UploadWorkspacePart(ctx context.Context, workspaceId uuid.UUID, uploadId string, partNumber int32, body io.ReadCloser, size int64) (*filedata.UploadPartResult, error)
	AbortWorkspaceUpload(ctx context.Context, workspaceId uuid.UUID, uploadId string) (*filedata.AbortUploadResult, error)
	CompleteWorkspaceUpload(ctx context.Context, workspaceId uuid.UUID, uploadId string) (*filedata.CompleteUploadResult, error)
	CreateWorkspaceFiles(ctx context.Context, workspaceId uuid.UUID, fd []filedata.WorkspaceFileData) ([]WorkspaceFileResult, error)
	CreateWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, creatorID uuid.UUID, folderName, parentPath string) (*WorkspaceFolderResult, error)
	RemoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, callerID uuid.UUID, callerRole string) error
	RemoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, folderPath string, callerID uuid.UUID, callerRole string) error
	MoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, destination string, callerID uuid.UUID, callerRole string) error
	MoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, sourcePath, destPath string, callerID uuid.UUID, callerRole string) (int, error)
}

// ── Constructor ───────────────────────────────────────────────────────────────

func NewWorkspaceFileService(log *zap.SugaredLogger, queries sqlc.Querier, storage storagetypes.ObjectStorage) WorkspaceFileService {
	return newWorkspaceFileService(log, queries, storage, nil)
}

func NewWorkspaceFileServiceWithRepository(log *zap.SugaredLogger, queries sqlc.Querier, storage storagetypes.ObjectStorage, repository repo.Repository) WorkspaceFileService {
	return newWorkspaceFileService(log, queries, storage, repository)
}

func newWorkspaceFileService(log *zap.SugaredLogger, queries sqlc.Querier, storage storagetypes.ObjectStorage, repository repo.Repository) WorkspaceFileService {
	return &workspaceFileService{log: log, queries: queries, storage: storage, repository: repository}
}

type workspaceFileService struct {
	log        *zap.SugaredLogger
	queries    sqlc.Querier
	storage    storagetypes.ObjectStorage
	repository repo.Repository
}

// ── Permission helpers ────────────────────────────────────────────────────────

func wsCanWrite(role string) bool {
	return role == "owner" || role == "admin" || role == "editor"
}

func wsCanAdmin(role string) bool {
	return role == "owner" || role == "admin"
}

// checkWsOwnership passes for admin/owner unconditionally; for editor it
// requires the resource to belong to the caller.
func checkWsOwnership(callerRole string, uploadedBy uuid.UUID, callerID uuid.UUID) error {
	if wsCanAdmin(callerRole) {
		return nil
	}
	if callerRole == "editor" && uploadedBy == callerID {
		return nil
	}
	return ErrWsForbidden
}

// ── Path helpers ──────────────────────────────────────────────────────────────

// wsNormPath normalises an arbitrary path string to the canonical form used in
// the workspace_files table (leading slash, no trailing slash, "/" for root).
func wsNormPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// wsParentPath returns the parent directory of a canonical path.
// e.g. "/docs/sub" → "/docs", "/docs" → "/", "/" → "/"
func wsParentPath(fullPath string) string {
	if fullPath == "/" {
		return "/"
	}
	idx := strings.LastIndex(fullPath, "/")
	if idx <= 0 {
		return "/"
	}
	return fullPath[:idx]
}

// wsFolderName returns the last component of a canonical path.
func wsFolderName(fullPath string) string {
	if fullPath == "/" {
		return ""
	}
	return fullPath[strings.LastIndex(fullPath, "/")+1:]
}

func workspaceUploadObjectKey(workspaceId, fileId uuid.UUID) string {
	return workspaceId.String() + "/" + fileId.String()
}

func isUniqueWorkspaceFileViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "unique_workspace_file"
}

func (w *workspaceFileService) getWorkspaceUploadByID(ctx context.Context, workspaceId uuid.UUID, uploadId string) (sqlc.FileUpload, error) {
	id, err := uuid.Parse(uploadId)
	if err != nil {
		return sqlc.FileUpload{}, fmt.Errorf("invalid_upload_id")
	}

	upload, err := w.queries.GetFileUploadById(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sqlc.FileUpload{}, fmt.Errorf("invalid_upload_id")
		}
		return sqlc.FileUpload{}, err
	}
	if !upload.WorkspaceID.Valid || upload.WorkspaceID.UUID != workspaceId {
		return sqlc.FileUpload{}, fmt.Errorf("invalid_upload_id")
	}

	return upload, nil
}

func (w *workspaceFileService) CreateWorkspaceUpload(ctx context.Context, params *filedata.CreateWorkspaceUploadParams) (*filedata.CreateUploadIdResponse, error) {
	if params == nil {
		return nil, fmt.Errorf("missing upload params")
	}

	bucket := w.storage.GetBucketBaseName()
	storageMapping := uuid.New()
	objectKey := workspaceUploadObjectKey(params.WorkspaceID, storageMapping)

	uploadId, err := w.storage.CreateMultipartUpload(ctx, bucket, objectKey, params.ContentType)
	if err != nil {
		logger.FromContext(ctx).Errorw("error creating workspace file upload ID", "workspace_id", params.WorkspaceID, "error", err)
		return nil, err
	}
	if uploadId == nil || strings.TrimSpace(*uploadId) == "" {
		return nil, fmt.Errorf("%w: storage backend returned empty upload id", storagetypes.ErrUploadFailed)
	}

	storageBackend := strings.TrimSpace(strings.ToLower(params.StorageBackend))
	if storageBackend == "" {
		storageBackend = "s3"
	}

	result, err := w.queries.CreateFileUpload(ctx, sqlc.CreateFileUploadParams{
		OwnerID:         params.UploaderUserID,
		WorkspaceID:     uuid.NullUUID{UUID: params.WorkspaceID, Valid: true},
		Path:            wsNormPath(params.Folder),
		StorageBackend:  storageBackend,
		StorageUploadID: sql.NullString{Valid: true, String: *uploadId},
		StorageMapping:  storageMapping,
		FileName:        params.FileName,
		FileType:        sql.NullString{Valid: strings.TrimSpace(params.ContentType) != "", String: params.ContentType},
		ExpectedSize:    params.Size,
		Status:          "uploading",
	})
	if err != nil {
		logger.FromContext(ctx).Errorw("error creating workspace multipart upload", "workspace_id", params.WorkspaceID, "error", err)

		// Best-effort cleanup: avoid leaking a multipart upload when DB insert fails.
		if storageBackend == "s3" {
			if abortErr := w.storage.AbortMultipartUpload(ctx, bucket, objectKey, strings.TrimSpace(*uploadId)); abortErr != nil {
				logger.FromContext(ctx).Warnw("error aborting workspace multipart upload after DB failure", "workspace_id", params.WorkspaceID, "error", abortErr)
			}
		}

		return nil, err
	}

	return &filedata.CreateUploadIdResponse{
		UploadId:  result.ID.String(),
		ChunkSize: pkg.OptimalChunkSize(params.Size),
	}, nil
}

func (w *workspaceFileService) UploadWorkspacePart(ctx context.Context, workspaceId uuid.UUID, uploadId string, partNumber int32, body io.ReadCloser, size int64) (*filedata.UploadPartResult, error) {
	if body != nil {
		defer body.Close()
	}
	if partNumber <= 0 {
		return nil, fmt.Errorf("%w: invalid part number", storagetypes.ErrTypeConversion)
	}
	if size <= 0 {
		return nil, fmt.Errorf("%w: invalid part size", storagetypes.ErrTypeConversion)
	}

	upload, err := w.getWorkspaceUploadByID(ctx, workspaceId, uploadId)
	if err != nil {
		return nil, err
	}
	if upload.Status != "uploading" {
		return nil, fmt.Errorf("upload_closed")
	}
	if !upload.StorageUploadID.Valid || strings.TrimSpace(upload.StorageUploadID.String) == "" {
		return nil, fmt.Errorf("%w: missing storage upload id", storagetypes.ErrUploadFailed)
	}

	bucket := w.storage.GetBucketBaseName()
	uploadPartResult, err := w.storage.UploadPart(
		ctx,
		bucket,
		workspaceUploadObjectKey(workspaceId, upload.StorageMapping),
		upload.StorageUploadID.String,
		partNumber,
		body,
		size,
	)
	if err != nil {
		w.log.Errorw("error uploading workspace chunk", "upload_id", upload.ID, "err", err)
		return nil, err
	}
	if uploadPartResult == nil {
		return nil, fmt.Errorf("%w: storage backend returned empty part result", storagetypes.ErrUploadFailed)
	}

	metadata, err := json.Marshal(uploadPartResult.StorageMetadata)
	if err != nil {
		return nil, err
	}

	_, err = w.queries.SaveFileUploadPart(ctx, sqlc.SaveFileUploadPartParams{
		ID:              upload.ID,
		PartNumber:      partNumber,
		StorageMetadata: metadata,
		Size:            uploadPartResult.Size,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMultipartUploadClosed
		}
		logger.FromContext(ctx).Errorw("error updating workspace upload parts DB", "chunk_id", partNumber, "error", err)
		return nil, err
	}

	return &filedata.UploadPartResult{
		PartNumber:      uploadPartResult.PartNumber,
		Size:            uploadPartResult.Size,
		StorageMetadata: uploadPartResult.StorageMetadata,
	}, nil
}

func (w *workspaceFileService) AbortWorkspaceUpload(ctx context.Context, workspaceId uuid.UUID, uploadId string) (*filedata.AbortUploadResult, error) {
	upload, err := w.getWorkspaceUploadByID(ctx, workspaceId, uploadId)
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

	abortedUpload, err := w.queries.AbortFileUpload(ctx, upload.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			refreshed, refreshErr := w.queries.GetFileUploadById(ctx, upload.ID)
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
		if err := w.queries.DeleteFileUploadPartsByUploadID(ctx, abortedUpload.ID); err != nil {
			return nil, err
		}
		return uploadAbortResult(abortedUpload), nil
	}

	bucket := w.storage.GetBucketBaseName()
	if err := w.storage.AbortMultipartUpload(ctx, bucket, workspaceUploadObjectKey(workspaceId, abortedUpload.StorageMapping), abortedUpload.StorageUploadID.String); err != nil {
		_, _ = w.queries.FailFileUpload(ctx, abortedUpload.ID)
		return nil, err
	}

	if err := w.queries.DeleteFileUploadPartsByUploadID(ctx, abortedUpload.ID); err != nil {
		return nil, err
	}

	return uploadAbortResult(abortedUpload), nil
}

func (w *workspaceFileService) CompleteWorkspaceUpload(ctx context.Context, workspaceId uuid.UUID, uploadId string) (*filedata.CompleteUploadResult, error) {
	upload, err := w.getWorkspaceUploadByID(ctx, workspaceId, uploadId)
	if err != nil {
		return nil, err
	}

	if upload.Status == "completed" {
		file, getErr := w.queries.GetWorkspaceFileById(ctx, sqlc.GetWorkspaceFileByIdParams{
			ID:          upload.StorageMapping,
			WorkspaceID: workspaceId,
		})
		if getErr != nil {
			return nil, getErr
		}

		return &filedata.CompleteUploadResult{
			UploadId:    upload.ID.String(),
			FileName:    file.FileName,
			Md5Checksum: file.Md5Checksum.String,
			Size:        file.Size,
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

	_, err = w.queries.GetWorkspaceFileByName(ctx, sqlc.GetWorkspaceFileByNameParams{
		WorkspaceID: workspaceId,
		Path:        wsNormPath(upload.Path),
		FileName:    upload.FileName,
	})
	if err == nil {
		return nil, storagetypes.ErrFileAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	partRows, err := w.queries.ListFileUploadPartsByUploadID(ctx, upload.ID)
	if err != nil {
		return nil, err
	}

	parts, uploadedSize, err := buildCompletedParts(partRows, upload.ExpectedSize)
	if err != nil {
		return nil, err
	}

	bucket := w.storage.GetBucketBaseName()
	if w.repository == nil {
		return nil, fmt.Errorf("multipart completion requires repository transaction support")
	}

	completed, err := w.storage.CompleteMultipartUpload(
		ctx,
		bucket,
		workspaceUploadObjectKey(workspaceId, upload.StorageMapping),
		upload.StorageUploadID.String,
		parts,
	)
	if err != nil {
		w.log.Errorw("error completing workspace multipart upload", "upload_id", upload.ID, "err", err)
		return nil, err
	}
	if completed == nil {
		return nil, fmt.Errorf("%w: storage backend returned empty completion result", storagetypes.ErrUploadFailed)
	}

	checksum := strings.TrimSpace(completed.ETag)
	if checksum == "" {
		checksum = workspaceUploadObjectKey(workspaceId, upload.StorageMapping)
	}

	tx, err := w.repository.BeginTx(ctx, nil)
	if err != nil {
		w.handleCompletedWorkspaceUploadFailure(ctx, workspaceId, upload, bucket, "begin tx failure")
		return nil, err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txq := w.repository.Queries().WithTx(tx)
	file, err := txq.CreateWorkspaceFile(ctx, sqlc.CreateWorkspaceFileParams{
		ID:          upload.StorageMapping,
		WorkspaceID: workspaceId,
		UploadedBy:  upload.OwnerID,
		FileName:    upload.FileName,
		FileType:    upload.FileType,
		Size:        uploadedSize,
		Md5Checksum: sql.NullString{Valid: checksum != "", String: checksum},
		Path:        wsNormPath(upload.Path),
	})
	if err != nil {
		w.handleCompletedWorkspaceUploadFailure(ctx, workspaceId, upload, bucket, "file insert failure")
		if isUniqueWorkspaceFileViolation(err) {
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

			refreshed, refreshErr := w.queries.GetFileUploadById(ctx, upload.ID)
			if refreshErr == nil && refreshed.WorkspaceID.Valid && refreshed.WorkspaceID.UUID == workspaceId {
				switch refreshed.Status {
				case "completed":
					fileRow, fileErr := w.queries.GetWorkspaceFileById(ctx, sqlc.GetWorkspaceFileByIdParams{
						ID:          refreshed.StorageMapping,
						WorkspaceID: workspaceId,
					})
					if fileErr == nil {
						return &filedata.CompleteUploadResult{
							UploadId:    refreshed.ID.String(),
							FileName:    fileRow.FileName,
							Md5Checksum: fileRow.Md5Checksum.String,
							Size:        fileRow.Size,
						}, nil
					}
				case "aborted", "failed":
					w.cleanupCompletedWorkspaceMultipartObject(ctx, workspaceId, upload, bucket, "upload completion closed")
					return nil, ErrMultipartUploadClosed
				}
			}

			w.handleCompletedWorkspaceUploadFailure(ctx, workspaceId, upload, bucket, "upload completion closed")
			return nil, ErrMultipartUploadClosed
		}

		w.handleCompletedWorkspaceUploadFailure(ctx, workspaceId, upload, bucket, "upload completion update failure")
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		w.handleCompletedWorkspaceUploadFailure(ctx, workspaceId, upload, bucket, "commit failure")
		return nil, err
	}
	committed = true

	return &filedata.CompleteUploadResult{
		UploadId:    upload.ID.String(),
		FileName:    file.FileName,
		Md5Checksum: file.Md5Checksum.String,
		Size:        uploadedSize,
	}, nil
}

func (w *workspaceFileService) handleCompletedWorkspaceUploadFailure(ctx context.Context, workspaceId uuid.UUID, upload sqlc.FileUpload, bucket, reason string) {
	if _, err := w.queries.FailFileUpload(ctx, upload.ID); err != nil {
		logger.FromContext(ctx).Errorw("error marking workspace upload failed", "upload_id", upload.ID, "reason", reason, "error", err)
	}
	w.cleanupCompletedWorkspaceMultipartObject(ctx, workspaceId, upload, bucket, reason)
}

func (w *workspaceFileService) cleanupCompletedWorkspaceMultipartObject(ctx context.Context, workspaceId uuid.UUID, upload sqlc.FileUpload, bucket, reason string) {
	if delErr := w.storage.DeleteObjectFromBucket(ctx, workspaceUploadObjectKey(workspaceId, upload.StorageMapping), bucket); delErr != nil {
		logger.FromContext(ctx).Errorw("error deleting completed workspace multipart object after failure", "upload_id", upload.ID, "reason", reason, "object", workspaceUploadObjectKey(workspaceId, upload.StorageMapping), "error", delErr)
	}
}

// wsChildFolder returns the immediate child folder name at targetPath that
// contains filePath. Returns ("", false) when filePath is not a descendant.
func wsChildFolder(filePath, targetPath string) (string, bool) {
	var rel string
	if targetPath == "/" {
		if !strings.HasPrefix(filePath, "/") || filePath == "/" {
			return "", false
		}
		rel = strings.TrimPrefix(filePath, "/")
	} else {
		prefix := targetPath + "/"
		if !strings.HasPrefix(filePath, prefix) {
			return "", false
		}
		rel = strings.TrimPrefix(filePath, prefix)
	}
	if rel == "" {
		return "", false
	}
	if idx := strings.Index(rel, "/"); idx >= 0 {
		return rel[:idx], true
	}
	return rel, true
}

// ── Read methods ──────────────────────────────────────────────────────────────

func (w *workspaceFileService) GetWorkspaceFiles(ctx context.Context, workspaceId uuid.UUID) ([]WorkspaceFileResult, error) {
	files, err := w.queries.GetWorkspaceFiles(ctx, workspaceId)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceFileResult, 0, len(files))
	for _, f := range files {
		if f.FileType.String == "inode/directory" {
			continue
		}
		result = append(result, wsFileToResult(f))
	}
	return result, nil
}

func (w *workspaceFileService) GetWorkspaceFilesTree(ctx context.Context, workspaceId uuid.UUID, path string) (WorkspaceFilesTree, error) {
	path = wsNormPath(path)

	// Load all entries (files+folders) for this workspace to find virtual folders
	// contributed by deeper paths, and explicitly-created folder placeholders.
	all, err := w.queries.GetWorkspaceFiles(ctx, workspaceId)
	if err != nil {
		return WorkspaceFilesTree{}, err
	}

	foldersSet := map[string]struct{}{}
	for _, entry := range all {
		if entry.FileType.String == "inode/directory" {
			if entry.Path == path {
				foldersSet[entry.FileName] = struct{}{}
			}
			continue
		}
		if child, ok := wsChildFolder(entry.Path, path); ok {
			foldersSet[child] = struct{}{}
		}
	}

	// Fetch explicit folder metadata (created_by email, created_at) at this path.
	explicitFolderRows, err := w.queries.GetWorkspaceFoldersAtPathWithCreators(ctx, sqlc.GetWorkspaceFoldersAtPathWithCreatorsParams{
		WorkspaceID: workspaceId,
		Path:        path,
	})
	if err != nil {
		return WorkspaceFilesTree{}, err
	}
	folderMeta := make(map[string]sqlc.GetWorkspaceFoldersAtPathWithCreatorsRow, len(explicitFolderRows))
	for _, ef := range explicitFolderRows {
		folderMeta[ef.FileName] = ef
	}

	// Fetch files at this path level with uploader emails via a single JOIN query.
	rows, err := w.queries.GetWorkspaceFilesAtPathWithUploaders(ctx, sqlc.GetWorkspaceFilesAtPathWithUploadersParams{
		WorkspaceID: workspaceId,
		Path:        path,
	})
	if err != nil {
		return WorkspaceFilesTree{}, err
	}

	files := make([]WorkspaceFileTreeEntry, 0, len(rows))
	for _, entry := range rows {
		if entry.Path != path {
			continue
		}
		files = append(files, WorkspaceFileTreeEntry{
			ID:              entry.ID,
			Name:            entry.FileName,
			FileType:        entry.FileType.String,
			Size:            entry.Size,
			MD5Checksum:     entry.Md5Checksum.String,
			UploadedBy:      entry.UploadedBy,
			UploadedByEmail: entry.UploaderEmail,
			CreatedAt:       entry.CreatedAt.String(),
		})
	}

	// Compute recursive size for each folder by summing real-file sizes from `all`.
	folderSizes := make(map[string]int64, len(foldersSet))
	for _, entry := range all {
		if entry.FileType.String == "inode/directory" {
			continue
		}
		for name := range foldersSet {
			folderPath := path
			if path == "/" {
				folderPath = "/" + name
			} else {
				folderPath = path + "/" + name
			}
			if entry.Path == folderPath || strings.HasPrefix(entry.Path, folderPath+"/") {
				folderSizes[name] += entry.Size
			}
		}
	}

	folders := make([]WorkspaceFolderTreeEntry, 0, len(foldersSet))
	for name := range foldersSet {
		entry := WorkspaceFolderTreeEntry{Name: name, Size: folderSizes[name]}
		if meta, ok := folderMeta[name]; ok {
			entry.CreatedByEmail = meta.CreatorEmail
			entry.CreatedAt = meta.CreatedAt.String()
		}
		folders = append(folders, entry)
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Name < folders[j].Name })
	sort.SliceStable(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	return WorkspaceFilesTree{Path: path, Files: files, Folders: folders}, nil
}

func (w *workspaceFileService) GetWorkspaceFileDownloadInfo(ctx context.Context, workspaceId, fileId uuid.UUID) (*WorkspaceFileDownloadInfo, error) {
	row, err := w.queries.GetWorkspaceFileById(ctx, sqlc.GetWorkspaceFileByIdParams{
		ID:          fileId,
		WorkspaceID: workspaceId,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWsFileNotFound
		}
		return nil, err
	}
	return &WorkspaceFileDownloadInfo{
		ObjectKey: workspaceId.String() + "/" + fileId.String(),
		FileName:  row.FileName,
		Bucket:    w.storage.GetBucketBaseName(),
	}, nil
}

// ── Write methods ─────────────────────────────────────────────────────────────

func (w *workspaceFileService) CreateWorkspaceFiles(ctx context.Context, workspaceId uuid.UUID, fd []filedata.WorkspaceFileData) ([]WorkspaceFileResult, error) {
	results := make([]WorkspaceFileResult, 0, len(fd))
	bucket := w.storage.GetBucketBaseName()

	for _, f := range fd {
		uploaderID, err := uuid.Parse(f.UploaderUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid uploader id: %w", err)
		}

		path := wsNormPath(f.Folder)
		fileName := f.RequestHeaders.Filename

		// Detect content type before upload.
		contentType := f.RequestHeaders.Header.Get("Content-Type")
		if contentType == "" {
			buf := make([]byte, 512)
			if _, readErr := f.MultipartFile.Read(buf); readErr != nil && readErr != io.EOF {
				return nil, readErr
			}
			f.MultipartFile.Seek(0, io.SeekStart)
			contentType = http.DetectContentType(buf)
		}

		// Generate the file ID here; it becomes the S3 object key.
		fileID := uuid.New()
		objectKey := workspaceUploadObjectKey(workspaceId, fileID)

		putResult, err := w.storage.PutObject(ctx, bucket, objectKey, f.MultipartFile, f.RequestHeaders.Size, contentType)
		if err != nil {
			logger.FromContext(ctx).Errorw("workspace upload: PutObject failed", "file", fileName, "error", err)
			return nil, err
		}

		row, err := w.queries.CreateWorkspaceFile(ctx, sqlc.CreateWorkspaceFileParams{
			ID:          fileID,
			WorkspaceID: workspaceId,
			UploadedBy:  uploaderID,
			FileName:    fileName,
			FileType:    sql.NullString{Valid: true, String: putResult.ContentType},
			Size:        putResult.Size,
			Md5Checksum: sql.NullString{Valid: true, String: putResult.MD5},
			Path:        path,
		})
		if err != nil {
			logger.FromContext(ctx).Errorw("workspace upload: DB insert failed, removing object", "file", fileName, "error", err)
			if delErr := w.storage.DeleteObjectFromBucket(ctx, objectKey, bucket); delErr != nil {
				logger.FromContext(ctx).Warnw("workspace upload: cleanup of storage object failed", "object_key", objectKey, "error", delErr)
			}
			return nil, err
		}

		results = append(results, wsFileToResult(row))
	}

	return results, nil
}

func (w *workspaceFileService) CreateWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, creatorID uuid.UUID, folderName, parentPath string) (*WorkspaceFolderResult, error) {
	parentPath = wsNormPath(parentPath)

	row, err := w.queries.CreateWorkspaceFolder(ctx, sqlc.CreateWorkspaceFolderParams{
		ID:          uuid.New(),
		WorkspaceID: workspaceId,
		UploadedBy:  creatorID,
		FileName:    folderName,
		Path:        parentPath,
	})
	if err != nil {
		return nil, err
	}

	return &WorkspaceFolderResult{
		ID:          row.ID,
		WorkspaceId: row.WorkspaceID,
		Name:        row.FileName,
		Path:        row.Path,
		CreatedBy:   row.UploadedBy,
		CreatedAt:   row.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (w *workspaceFileService) RemoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, callerID uuid.UUID, callerRole string) error {
	row, err := w.queries.GetWorkspaceFileById(ctx, sqlc.GetWorkspaceFileByIdParams{
		ID: fileId, WorkspaceID: workspaceId,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWsFileNotFound
		}
		return err
	}

	if err := checkWsOwnership(callerRole, row.UploadedBy, callerID); err != nil {
		return err
	}

	bucket := w.storage.GetBucketBaseName()
	objectKey := workspaceUploadObjectKey(workspaceId, fileId)
	if delErr := w.storage.DeleteObjectFromBucket(ctx, objectKey, bucket); delErr != nil {
		logger.FromContext(ctx).Warnw("workspace delete: storage delete failed (non-fatal)", "object_key", objectKey, "error", delErr)
	}

	return w.queries.DeleteWorkspaceFileById(ctx, sqlc.DeleteWorkspaceFileByIdParams{
		ID: fileId, WorkspaceID: workspaceId,
	})
}

func (w *workspaceFileService) RemoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, folderPath string, callerID uuid.UUID, callerRole string) error {
	folderPath = wsNormPath(folderPath)
	parentPath := wsParentPath(folderPath)
	folderName := wsFolderName(folderPath)

	placeholder, err := w.queries.GetWorkspaceFolderByPathAndName(ctx, sqlc.GetWorkspaceFolderByPathAndNameParams{
		WorkspaceID: workspaceId,
		Path:        parentPath,
		FileName:    folderName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWsFolderNotFound
		}
		return err
	}

	if err := checkWsOwnership(callerRole, placeholder.UploadedBy, callerID); err != nil {
		return err
	}

	// Collect all non-directory files within the folder tree.
	all, err := w.queries.GetWorkspaceFiles(ctx, workspaceId)
	if err != nil {
		return err
	}

	bucket := w.storage.GetBucketBaseName()
	for _, f := range all {
		if f.FileType.String == "inode/directory" {
			continue
		}
		if f.Path == folderPath || strings.HasPrefix(f.Path, folderPath+"/") {
			// Editors can only bulk-delete a folder if every file inside is theirs.
			if callerRole == "editor" && f.UploadedBy != callerID {
				return ErrWsForbidden
			}
			objectKey := workspaceId.String() + "/" + f.ID.String()
			if delErr := w.storage.DeleteObjectFromBucket(ctx, objectKey, bucket); delErr != nil {
				logger.FromContext(ctx).Warnw("workspace folder delete: storage delete failed (non-fatal)", "object_key", objectKey, "error", delErr)
			}
		}
	}

	// Delete DB records: direct children, nested children, then the placeholder.
	if err := w.queries.DeleteWorkspaceFilesByPath(ctx, sqlc.DeleteWorkspaceFilesByPathParams{
		WorkspaceID: workspaceId, Path: folderPath,
	}); err != nil {
		return err
	}
	if err := w.queries.DeleteWorkspaceFilesByPathPrefix(ctx, sqlc.DeleteWorkspaceFilesByPathPrefixParams{
		WorkspaceID: workspaceId, Path: folderPath + "/%",
	}); err != nil {
		return err
	}
	return w.queries.DeleteWorkspaceFileById(ctx, sqlc.DeleteWorkspaceFileByIdParams{
		ID: placeholder.ID, WorkspaceID: workspaceId,
	})
}

func (w *workspaceFileService) MoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, destination string, callerID uuid.UUID, callerRole string) error {
	destination = wsNormPath(destination)

	row, err := w.queries.GetWorkspaceFileById(ctx, sqlc.GetWorkspaceFileByIdParams{
		ID: fileId, WorkspaceID: workspaceId,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWsFileNotFound
		}
		return err
	}

	if err := checkWsOwnership(callerRole, row.UploadedBy, callerID); err != nil {
		return err
	}

	return w.queries.MoveWorkspaceFile(ctx, sqlc.MoveWorkspaceFileParams{
		Path: destination, ID: fileId, WorkspaceID: workspaceId,
	})
}

func (w *workspaceFileService) MoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, sourcePath, destPath string, callerID uuid.UUID, callerRole string) (int, error) {
	sourcePath = wsNormPath(sourcePath)
	destPath = wsNormPath(destPath)

	srcParent := wsParentPath(sourcePath)
	srcName := wsFolderName(sourcePath)
	dstParent := wsParentPath(destPath)
	dstName := wsFolderName(destPath)

	placeholder, err := w.queries.GetWorkspaceFolderByPathAndName(ctx, sqlc.GetWorkspaceFolderByPathAndNameParams{
		WorkspaceID: workspaceId,
		Path:        srcParent,
		FileName:    srcName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrWsFolderNotFound
		}
		return 0, err
	}

	if err := checkWsOwnership(callerRole, placeholder.UploadedBy, callerID); err != nil {
		return 0, err
	}

	// Editors must own all files within the folder to move it.
	if callerRole == "editor" {
		all, err := w.queries.GetWorkspaceFiles(ctx, workspaceId)
		if err != nil {
			return 0, err
		}
		for _, f := range all {
			if f.FileType.String == "inode/directory" {
				continue
			}
			if f.Path == sourcePath || strings.HasPrefix(f.Path, sourcePath+"/") {
				if f.UploadedBy != callerID {
					return 0, ErrWsForbidden
				}
			}
		}
	}

	// Use regexp_replace to update all file paths in one query.
	// Pattern requires a path separator after sourcePath so "/docs" doesn't match "/docs2".
	var pattern string
	if sourcePath == "/" {
		pattern = "^/"
	} else {
		pattern = "^" + regexp.QuoteMeta(sourcePath) + "(/|$)"
	}

	if err := w.queries.MoveWorkspaceFilesByPathPrefix(ctx, sqlc.MoveWorkspaceFilesByPathPrefixParams{
		RegexpReplace:   pattern,
		RegexpReplace_2: destPath + `\1`,
		WorkspaceID:     workspaceId,
		Path:            sourcePath,
	}); err != nil {
		return 0, err
	}

	// Update the folder placeholder itself.
	if err := w.queries.UpdateWorkspaceFolderLocation(ctx, sqlc.UpdateWorkspaceFolderLocationParams{
		Path:        dstParent,
		FileName:    dstName,
		ID:          placeholder.ID,
		WorkspaceID: workspaceId,
	}); err != nil {
		return 0, err
	}

	return 1, nil
}

// ── Private helpers ───────────────────────────────────────────────────────────

func wsFileToResult(f sqlc.WorkspaceFile) WorkspaceFileResult {
	return WorkspaceFileResult{
		ID:          f.ID,
		WorkspaceId: f.WorkspaceID,
		Name:        f.FileName,
		Type:        f.FileType.String,
		Size:        f.Size,
		Path:        f.Path,
		Md5Checksum: f.Md5Checksum.String,
		CreatedAt:   f.CreatedAt.Format(time.RFC3339),
		UploadedBy:  f.UploadedBy,
	}
}
