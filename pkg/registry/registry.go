package registry

import "github.com/ignitionstack/ignition/pkg/manifest"

type Registry interface {
	Get(namespace, name string) (*FunctionMetadata, error)
	Push(namespace, name string, payload []byte, digest, tag string, config manifest.FunctionVersionSettings) error
	Pull(namespace, name string, version string) ([]byte, *VersionInfo, error)
	ReassignTag(namespace, name, tag, newDigest string) error
	DigestExists(namespace, name, digest string) (bool, error)
	ListAll() ([]FunctionMetadata, error)
}
