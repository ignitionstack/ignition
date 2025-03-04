package engineclient

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
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
	Payload    string            `json:"payload"`
	Config     map[string]string `json:"config,omitempty"`
}

// OneOffCallRequest represents a request to call a function by loading it temporarily
type OneOffCallRequest struct {
	BaseRequest
	Reference  string            `json:"reference"`
	Entrypoint string            `json:"entrypoint"`
	Payload    string            `json:"payload"`
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
	types.BuildResult
}

// LoadResponse represents the response from a load operation
type LoadResponse struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Digest    string        `json:"digest"`
	LoadTime  time.Duration `json:"load_time"`
}

// ListFunctionsResponse represents the response from listing functions
type ListFunctionsResponse struct {
	Functions []types.LoadedFunction `json:"functions"`
}

// LogsResponse represents the response from a logs request
type LogsResponse []string

// EngineResponseError represents an error response from the engine
type EngineResponseError struct {
	ErrorType string `json:"error"`
	Message   string `json:"message"`
	Code      int    `json:"code"`
}

func (e EngineResponseError) Error() string {
	return e.Message
}

// ErrorResponse is maintained for backwards compatibility
//
//nolint:errname // maintained for backwards compatibility
type ErrorResponse = EngineResponseError
