package lerrors

import "errors"

var (
	ErrAvatarNotFound   = errors.New("avatar not found")
	ErrAvatarForbidden  = errors.New("forbidden")
	ErrInvalidSize      = errors.New("invalid size parameter")
	ErrInvalidFormat    = errors.New("invalid format parameter")
	ErrUploadFailed     = errors.New("upload failed")
	ErrProcessingFailed = errors.New("processing failed")
	ErrDeletingFailed   = errors.New("deleting failed")
)
