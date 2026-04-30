package filedata

import (
	"mime/multipart"
)

const WorkspaceFileDataContextKey ContextKey = "workspacedata"

type WorkspaceFileData struct {
	MultipartFile  multipart.File
	RequestHeaders *multipart.FileHeader
	Folder         string
	WorkspaceId    string
	OwnerId        string
	FileName       string
}

func NewWorkspaceFileData(multipartFile multipart.File, requestHeaders *multipart.FileHeader, fileName, folder, workspaceId, ownerId string) *WorkspaceFileData {
	return &WorkspaceFileData{
		MultipartFile:  multipartFile,
		RequestHeaders: requestHeaders,
		Folder:         folder,
		WorkspaceId:    workspaceId,
		OwnerId:        ownerId,
		FileName:       fileName,
	}
}
