package filedata

import "mime/multipart"

type FileData struct {
	MultipartFile  multipart.File
	RequestHeaders *multipart.FileHeader
	Folder         string
}

func NewFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, folder string) *FileData {

	return &FileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
	}
}
