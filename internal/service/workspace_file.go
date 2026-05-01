package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
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

type WorkspaceFilesTree struct {
	Path    string                   `json:"path"`
	Files   []WorkspaceFileTreeEntry `json:"files"`
	Folders []string                 `json:"folders"`
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
	CreateWorkspaceFiles(ctx context.Context, workspaceId uuid.UUID, fd []filedata.WorkspaceFileData) ([]WorkspaceFileResult, error)
	CreateWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, creatorID uuid.UUID, folderName, parentPath string) (*WorkspaceFolderResult, error)
	RemoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, callerID uuid.UUID, callerRole string) error
	RemoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, folderPath string, callerID uuid.UUID, callerRole string) error
	MoveWorkspaceFile(ctx context.Context, workspaceId, fileId uuid.UUID, destination string, callerID uuid.UUID, callerRole string) error
	MoveWorkspaceFolder(ctx context.Context, workspaceId uuid.UUID, sourcePath, destPath string, callerID uuid.UUID, callerRole string) (int, error)
}

// ── Constructor ───────────────────────────────────────────────────────────────

func NewWorkspaceFileService(queries *sqlc.Queries, storage storagetypes.ObjectStorage) WorkspaceFileService {
	return &workspaceFileService{queries: queries, storage: storage}
}

type workspaceFileService struct {
	queries *sqlc.Queries
	storage storagetypes.ObjectStorage
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

	folders := make([]string, 0, len(foldersSet))
	for f := range foldersSet {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

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
		uploaderID, err := uuid.Parse(f.OwnerId)
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
		objectKey := workspaceId.String() + "/" + fileID.String()

		putResult, err := w.storage.PutObject(ctx, bucket, objectKey, f.MultipartFile, f.RequestHeaders.Size, contentType)
		if err != nil {
			log.Printf("workspace upload: PutObject failed for %q: %v", fileName, err)
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
			log.Printf("workspace upload: DB insert failed for %q, removing object: %v", fileName, err)
			if delErr := w.storage.DeleteObjectFromBucket(ctx, objectKey, bucket); delErr != nil {
				log.Printf("workspace upload: cleanup failed for %q: %v", objectKey, delErr)
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
	objectKey := workspaceId.String() + "/" + fileId.String()
	if delErr := w.storage.DeleteObjectFromBucket(ctx, objectKey, bucket); delErr != nil {
		log.Printf("workspace delete: storage delete failed for %s (non-fatal): %v", objectKey, delErr)
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
				log.Printf("workspace folder delete: storage delete failed for %s (non-fatal): %v", objectKey, delErr)
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
