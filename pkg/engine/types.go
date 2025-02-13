package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
)

// Custom errors
var (
	ErrFunctionNotLoaded = NewError("function not loaded")
	ErrInvalidConfig     = NewError("invalid configuration")
	ErrPluginCreation    = NewError("failed to create plugin")
)

// Error represents a custom engine error
type Error struct {
	msg string
}

func (e *Error) Error() string {
	return e.msg
}

func NewError(msg string) *Error {
	return &Error{msg: msg}
}

type BuildRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Tag       string `json:"tag"`

	Manifest manifest.FunctionManifest `json:"manifest"`
}

type BuildResponse struct {
	Digest string `json:"digest"`
	Tag    string `json:"tag"`
	Status string `json:"status"`
}

type BuildResult struct {
	Name      string
	Namespace string
	Digest    string
	BuildTime time.Duration
	Tag       string
	Reused    bool
}

type LoadResult struct {
	Namespace string
	Name      string
	Digest    string
	LoadTime  time.Duration
}

type FunctionRequest struct {
	Namespace string `json:"namespace" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

type LoadRequest struct {
	FunctionRequest
	Digest string `json:"digest" validate:"required"`
}

type OneOffCallRequest struct {
	FunctionRequest
	Reference  string `json:"reference" validate:"required"`
	Entrypoint string `json:"entrypoint" validate:"required"`
	Payload    string `json:"payload"`
}

type ReassignTagRequest struct {
	FunctionRequest
	Tag    string `json:"tag" validate:"required"`
	Digest string `json:"digest" validate:"required"`
}
