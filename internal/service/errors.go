package service

import "errors"

var (
	ErrRecursiveRequired = errors.New("recursive deletion required for non-empty folder")
	ErrNoteTooLong       = errors.New("note exceeds 500 character limit")
	ErrTokenExpired      = errors.New("sharing token has expired")
	ErrAccessDenied      = errors.New("access denied")
)
