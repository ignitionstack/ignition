package engine

import (
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

type ExtendedBuildRequest struct {
	types.BuildRequest
	Manifest manifest.FunctionManifest
}

type EngineConfig struct {
	// SocketPath is the path to the Unix socket used by the engine for local
	// communication with CLI tools.
	SocketPath string

	// HttpAddr is the address the HTTP API will listen on. This is used for
	// HTTP requests to stored functions.
	HttpAddr string

	// RegistryDir is the directory that will store function data.
	RegistryDir string
}

type EngineStatus struct {
	// Version is the semantic version of the engine.
	Version string

	// LoadedPlugins is the number of WebAssembly plugins loaded.
	LoadedPlugins int

	// Uptime is the uptime of the engine in seconds.
	Uptime int64
}