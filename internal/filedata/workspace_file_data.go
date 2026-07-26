package filedata

import (
	"mime/multipart"
)

const WorkspaceFileDataContextKey ContextKey = "workspacedata"

type WorkspaceFileData struct {
	MultipartFile  multipart.File
	RequestHeaders *multipart.FileHeader
	Folder         string
	WorkspaceID    string
	UploaderUserID string
	FileName       string
}

func NewWorkspaceFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, fileName, folder, workspaceID, uploaderUserID string) *WorkspaceFileData {
	return &WorkspaceFileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
		WorkspaceID:    workspaceID,
		UploaderUserID: uploaderUserID,
		FileName:       fileName,
	}
}
