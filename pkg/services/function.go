package services

import (
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

type FunctionService interface {
	BuildFunction(path string, config manifest.FunctionManifest) (*types.BuildResult, error)
}
