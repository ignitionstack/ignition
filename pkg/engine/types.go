package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// ExtendedBuildRequest extends types.BuildRequest with the manifest
type ExtendedBuildRequest struct {
	types.BuildRequest
	Manifest manifest.FunctionManifest `json:"manifest" validate:"required"`
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
