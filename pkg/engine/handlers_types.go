package engine

import (
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// ExtendedBuildRequest represents a request to build a function
type ExtendedBuildRequest struct {
	// Embed the build request from types package
	types.BuildRequest

	// Add the manifest field
	Manifest manifest.FunctionManifest `json:"manifest"`
}
