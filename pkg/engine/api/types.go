package api

import (
	"github.com/ignitionstack/ignition/pkg/engine/models"
	"github.com/ignitionstack/ignition/pkg/manifest"
)

// BaseRequest contains common fields for all requests
type BaseRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// LoadRequest represents a request to load a function into the engine
type LoadRequest struct {
	BaseRequest
	Digest    string            `json:"digest"`
	Config    map[string]string `json:"config,omitempty"`
	ForceLoad bool              `json:"force_load,omitempty"`
}

// UnloadRequest represents a request to unload a function from the engine
type UnloadRequest struct {
	BaseRequest
}

// StopRequest represents a request to stop a function in the engine
type StopRequest struct {
	BaseRequest
}

// CallRequest represents a request to call a function
type CallRequest struct {
	BaseRequest
	Entrypoint string            `json:"entrypoint"`
	Payload    string            `json:"payload,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
}

// OneOffCallRequest represents a request to call a function by loading it temporarily
type OneOffCallRequest struct {
	BaseRequest
	Reference  string            `json:"reference"`
	Entrypoint string            `json:"entrypoint"`
	Payload    string            `json:"payload,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
}

// BuildRequest represents a request to build a function
type BuildRequest struct {
	BaseRequest
	Path     string                    `json:"path"`
	Tag      string                    `json:"tag,omitempty"`
	Manifest manifest.FunctionManifest `json:"manifest"`
}

// StatusResponse represents the response from a status check
type StatusResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

// CallResponse represents the response from a function call
type CallResponse struct {
	Result  interface{} `json:"result"`
	Elapsed string      `json:"elapsed"`
}

// OneOffCallResponse represents the response from a one-off function call
type OneOffCallResponse = []byte

// BuildResponse represents the response from a build operation
type BuildResponse struct {
	models.BuildResult
}

// LoadResponse represents the response from a load operation
type LoadResponse struct {
	models.LoadResult
}

// ListFunctionsResponse represents the response from listing functions
type ListFunctionsResponse struct {
	Functions []models.Function `json:"functions"`
}

// LogsResponse represents the response from a logs request
type LogsResponse []string

// ResponseError represents an error response from the engine API
type ResponseError struct {
	ErrorType string `json:"error"`
	Message   string `json:"message"`
	Code      int    `json:"code"`
}

func (e ResponseError) Error() string {
	return e.Message
}

// APIResponseError is deprecated, use ResponseError instead
//
//nolint:revive // maintained for backwards compatibility
type APIResponseError = ResponseError

// ErrorResponse is deprecated, use ResponseError instead
//
//nolint:errname // maintained for backwards compatibility
type ErrorResponse = ResponseError
