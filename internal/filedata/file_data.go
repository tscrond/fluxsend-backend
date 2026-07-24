package filedata

import (
	"mime/multipart"

	"github.com/google/uuid"
)

type ContextKey string

const FileDataContextKey ContextKey = "filedata"

type FileData struct {
	MultipartFile   multipart.File
	RequestHeaders  *multipart.FileHeader
	Folder          string
	OwnerID         uuid.UUID // owner's ID within application logic
	OwnerInternalID string    // user's ID based on his identity provider_user_id
}

func NewFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, folder string) *FileData {
	return &FileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
	}
}

type CreateUploadIdResponse struct {
	UploadId  string
	ChunkSize *int64
}

type CreateUploadIdParams struct {
	OwnerID uuid.UUID

	FileName string
	Folder   string

	ContentType string
	Size        int64

	StorageBackend string
}
