package engine

import (
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

type ExtendedBuildRequest struct {
	types.BuildRequest
	Manifest manifest.FunctionManifest `json:"manifest"`
}
