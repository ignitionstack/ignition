package types

import (
	"time"
)

// BuildRequest represents a request to build a function
type BuildRequest struct {
	Namespace string `json:"namespace" validate:"required"`
	Name      string `json:"name" validate:"required"`
	Path      string `json:"path" validate:"required"`
	Tag       string `json:"tag"`
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
	Digest string            `json:"digest" validate:"required"`
	Config map[string]string `json:"config,omitempty"`
}

// OneOffCallRequest represents a request to call a function once
type OneOffCallRequest struct {
	FunctionRequest
	Reference  string            `json:"reference" validate:"required"`
	Entrypoint string            `json:"entrypoint" validate:"required"`
	Payload    string            `json:"payload"`
	Config     map[string]string `json:"config,omitempty"`
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
	Tags         []string `json:"tags,omitempty"`
}

// PluginOptions defines configuration options for plugins
type PluginOptions struct {
	EnableWasi    bool     `json:"enable_wasi"`
	AllowedHosts  []string `json:"allowed_hosts"`
	EnableCache   bool     `json:"enable_cache"`
	MaxMemoryMB   int      `json:"max_memory_mb"`
	TimeoutMillis int      `json:"timeout_millis"`
}

// LoadedFunction represents a function that is currently loaded in memory
type LoadedFunction struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}
