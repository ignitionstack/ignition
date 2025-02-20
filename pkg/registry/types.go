package registry

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
)

type FunctionMetadata struct {
	Namespace string                 `json:"namespace"`
	Name      string                 `json:"name"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Versions  []VersionInfo          `json:"versions"`
	Config    map[string]interface{} `json:"config"`
}

type VersionInfo struct {
	Hash       string                           `json:"hash"`
	FullDigest string                           `json:"full_digest"`
	CreatedAt  time.Time                        `json:"created_at"`
	Size       int64                            `json:"size"`
	Tags       []string                         `json:"tags"`
	Settings   manifest.FunctionVersionSettings `json:"settings"` // Add settings field
}
