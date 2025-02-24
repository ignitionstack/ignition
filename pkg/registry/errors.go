package registry

import "errors"

var (
	ErrFunctionNotFound = errors.New("function not found")
	ErrTagNotFound      = errors.New("tag not found")
	ErrDigestNotFound   = errors.New("digest not found")
	ErrInvalidReference = errors.New("invalid reference format")
	ErrVersionNotFound  = errors.New("version not found")
)
