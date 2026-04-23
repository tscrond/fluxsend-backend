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
	OwnerID         uuid.UUID
	OwnerInternalID string
}

func NewFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, folder string) *FileData {
	return &FileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
	}
}
