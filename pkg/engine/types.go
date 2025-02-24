package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
)

// BuildRequest represents a request to build a function
type BuildRequest struct {
	Namespace string                    `json:"namespace" validate:"required"`
	Name      string                    `json:"name" validate:"required"`
	Path      string                    `json:"path" validate:"required"`
	Tag       string                    `json:"tag"`
	Manifest  manifest.FunctionManifest `json:"manifest" validate:"required"`
}

// BuildResponse represents the response from a build operation
type BuildResponse struct {
	Digest    string `json:"digest"`
	Tag       string `json:"tag"`
	Status    string `json:"status"`
	BuildTime string `json:"build_time"`
}

// BuildResult contains information about a successful build
type BuildResult struct {
	Name      string
	Namespace string
	Digest    string
	BuildTime time.Duration
	Tag       string
	Reused    bool
}

// LoadResult contains information about a successful load operation
type LoadResult struct {
	Namespace string
	Name      string
	Digest    string
	LoadTime  time.Duration
}

// FunctionRequest is a base request with function identification
type FunctionRequest struct {
	Namespace string `json:"namespace" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

// LoadRequest represents a request to load a function
type LoadRequest struct {
	FunctionRequest
	Digest string `json:"digest" validate:"required"`
}

// OneOffCallRequest represents a request to call a function once
type OneOffCallRequest struct {
	FunctionRequest
	Reference  string `json:"reference" validate:"required"`
	Entrypoint string `json:"entrypoint" validate:"required"`
	Payload    string `json:"payload"`
}

// ReassignTagRequest represents a request to reassign a tag
type ReassignTagRequest struct {
	FunctionRequest
	Tag    string `json:"tag" validate:"required"`
	Digest string `json:"digest" validate:"required"`
}

// ListResponse represents the response from a list operation
type ListResponse struct {
	Functions []FunctionInfo `json:"functions"`
}

// FunctionInfo provides summary information about a function
type FunctionInfo struct {
	Namespace    string   `json:"namespace"`
	Name         string   `json:"name"`
	LatestDigest string   `json:"latest_digest"`
	Tags         []string `json:"tags"`
}

// PluginOptions defines configuration options for plugins
type PluginOptions struct {
	EnableWasi    bool     `json:"enable_wasi"`
	AllowedHosts  []string `json:"allowed_hosts"`
	EnableCache   bool     `json:"enable_cache"`
	MaxMemoryMB   int      `json:"max_memory_mb"`
	TimeoutMillis int      `json:"timeout_millis"`
}

// EngineConfig contains configuration options for the engine
type EngineConfig struct {
	SocketPath      string `json:"socket_path"`
	HTTPAddress     string `json:"http_address"`
	RegistryPath    string `json:"registry_path"`
	LogLevel        string `json:"log_level"`
	LogFile         string `json:"log_file"`
	EnableMetrics   bool   `json:"enable_metrics"`
	MetricsEndpoint string `json:"metrics_endpoint"`
	MaxConcurrency  int    `json:"max_concurrency"`
}

// EngineStatus contains runtime status information about the engine
type EngineStatus struct {
	StartTime       time.Time `json:"start_time"`
	Uptime          string    `json:"uptime"`
	LoadedFunctions int       `json:"loaded_functions"`
	ActiveRequests  int       `json:"active_requests"`
	TotalRequests   int64     `json:"total_requests"`
	FailedRequests  int64     `json:"failed_requests"`
}
