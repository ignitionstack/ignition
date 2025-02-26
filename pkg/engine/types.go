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
	SocketPath  string
	HttpAddr    string
	RegistryDir string
}

type EngineStatus struct {
	Version       string
	LoadedPlugins int
	Uptime        int64
}
