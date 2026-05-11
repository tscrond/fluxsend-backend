package service

import (
	"context"
	"database/sql"
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
	Upload(ctx context.Context, fd *filedata.FileData) error
	GetFilesTree(ctx context.Context, userID uuid.UUID, path string) (*FilesTree, error)
	GetFolders(ctx context.Context, userID uuid.UUID, path string) ([]string, error)
	DeleteFile(ctx context.Context, userID uuid.UUID, userInternalID, fileName string) error
	DeleteFiles(ctx context.Context, userID uuid.UUID, userInternalID string, fileNames []string) (deleted []string, failed []string, err error)
	DeleteFolder(ctx context.Context, userID uuid.UUID, userInternalID, folderPath string, recursive bool) (int, error)
	MoveFile(ctx context.Context, userID uuid.UUID, source, destination string) error
	MoveFolder(ctx context.Context, userID uuid.UUID, source, destination string) (int, error)
	GetNote(ctx context.Context, userID uuid.UUID, checksum string) (string, error)
	UpsertNote(ctx context.Context, userID uuid.UUID, checksum, content string) (string, error)
}

type fileService struct {
	log       *zap.SugaredLogger
	queries   sqlc.Querier
	storage   storagetypes.ObjectStorage
	sanitizer *bluemonday.Policy
}

func NewFileService(log *zap.SugaredLogger, queries sqlc.Querier, storage storagetypes.ObjectStorage, sanitizer *bluemonday.Policy) FileService {
	return &fileService{
		log:       log,
		queries:   queries,
		storage:   storage,
		sanitizer: sanitizer,
	}
}

func (s *fileService) Upload(ctx context.Context, fd *filedata.FileData) error {
	fileName := fd.RequestHeaders.Filename
	if fd.Folder != "" {
		fileName = fd.Folder + "/" + fileName
	}

	_, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  fd.OwnerID,
		FileName: fileName,
	})
	if err == nil {
		return storagetypes.ErrFileAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		logger.FromContext(ctx).Errorw("error checking existing file", "error", err)
		return storagetypes.ErrUploadFailed
	}

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), fd.OwnerID, fd.OwnerInternalID)
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
		OwnerID:              fd.OwnerID,
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

func (s *fileService) DeleteFile(ctx context.Context, userID uuid.UUID, userInternalID, fileName string) error {
	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID, userInternalID)
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

func (s *fileService) DeleteFiles(ctx context.Context, userID uuid.UUID, userInternalID string, fileNames []string) (deleted []string, failed []string, err error) {
	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID, userInternalID)
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

func (s *fileService) DeleteFolder(ctx context.Context, userID uuid.UUID, userInternalID, folderPath string, recursive bool) (int, error) {
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

	bucket, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID, userInternalID)
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
	fileID, err := s.queries.GetFileFromChecksum(ctx, checksum)
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
	fileID, err := s.queries.GetFileFromChecksum(ctx, checksum)
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

// resolveUserBucketName resolves the stored bucket name for a user, falling back
// to constructing it from the base name and the user's internal ID.
func resolveUserBucketName(ctx context.Context, queries sqlc.Querier, baseBucketName string, userID uuid.UUID, internalID string) (string, error) {
	stored, err := queries.GetUserBucketById(ctx, userID)
	if err != nil {
		return "", err
	}
	if stored.Valid && strings.TrimSpace(stored.String) != "" {
		return strings.TrimSpace(stored.String), nil
	}
	return fmt.Sprintf("%s-%s", baseBucketName, internalID), nil
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
