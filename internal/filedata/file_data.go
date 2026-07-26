package filedata

import (
	"io"
	"mime/multipart"

	"github.com/google/uuid"
)

type ContextKey string

const FileDataContextKey ContextKey = "filedata"

type FileData struct {
	MultipartFile  multipart.File
	RequestHeaders *multipart.FileHeader
	Folder         string
	OwnerUserID    uuid.UUID // files.owner_id / users.id for personal files
}

func NewFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, folder string) *FileData {
	return &FileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
	}
}

// upload initialization
type CreateUploadIdResponse struct {
	UploadId  string
	ChunkSize int64
}

type CreateUploadIdParams struct {
	OwnerUserID uuid.UUID

	FileName string
	Folder   string

	ContentType string
	Size        int64

	StorageBackend string
}

// chunk upload
type UploadPartParams struct {
	UploadID   uuid.UUID
	PartNumber int32
	Body       io.Reader
	Size       int64
}
type UploadPartResult struct {
	PartNumber int32
	Size       int64

	// Provider-specific data needed for CompleteMultipartUpload
	StorageMetadata map[string]any
}

type CompleteUploadResult struct {
	UploadId    string `json:"upload_id"`
	FileName    string `json:"file_name"`
	Md5Checksum string `json:"md5_checksum"`
	Size        int64  `json:"size"`
}
