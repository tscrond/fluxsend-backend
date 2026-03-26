package types

import "errors"

var (
	ErrFileAlreadyExists = errors.New("file already exists")
	ErrStorageUnavailable = errors.New("storage unavailable")
	ErrUploadFailed = errors.New("upload failed")
)