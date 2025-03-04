package models

import (
	"time"
)

// Function represents a WebAssembly function in the engine
type Function struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Digest    string            `json:"digest,omitempty"`
	Status    string            `json:"status,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
	LoadTime  time.Time         `json:"load_time,omitempty"`
}

// FunctionReference is a lightweight reference to a function
type FunctionReference struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Service   string `json:"service,omitempty"`
}

// BuildResult contains information about a successful build
type BuildResult struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Digest    string `json:"digest"`
	BuildTime string `json:"build_time,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Reused    bool   `json:"reused,omitempty"`
}

// LoadResult contains information about a successful load operation
type LoadResult struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Digest    string        `json:"digest"`
	LoadTime  time.Duration `json:"load_time"`
}
